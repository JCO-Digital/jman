package cache

import (
	"html"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/fetch/wporg"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/hashicorp/go-version"
)

// PluginInfoCache is the on-disk structure for plugin_info.json.
type PluginInfoCache struct {
	Plugins   map[string]PluginInfoEntry `json:"plugins"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

// PluginInfoEntry wraps a single plugin's info with its own fetch timestamp.
type PluginInfoEntry struct {
	Info      models.PluginInfo `json:"info"`
	FetchedAt time.Time         `json:"fetched_at"`
}

var (
	pluginInfoCacheInstance *PluginInfoCache
	pluginInfoMutex         sync.RWMutex
	htmlTagRe               = regexp.MustCompile(`<[^>]*>`)
)

// loadPluginInfoCache ensures the cache is loaded from disk.
func loadPluginInfoCache() *PluginInfoCache {
	pluginInfoMutex.Lock()
	defer pluginInfoMutex.Unlock()

	if pluginInfoCacheInstance != nil {
		return pluginInfoCacheInstance
	}

	cache := &PluginInfoCache{
		Plugins: make(map[string]PluginInfoEntry),
	}

	// We use ReadJSONCache which handles TTL, but for this specific cache
	// we manage TTL per-entry. However, the helper is still useful for loading.
	_ = ReadJSONCache("plugin_info", cache)

	if cache.Plugins == nil {
		cache.Plugins = make(map[string]PluginInfoEntry)
	}

	pluginInfoCacheInstance = cache
	return pluginInfoCacheInstance
}

// SavePluginInfoCache writes the current cache instance to disk.
func SavePluginInfoCache() error {
	pluginInfoMutex.RLock()
	defer pluginInfoMutex.RUnlock()

	if pluginInfoCacheInstance == nil {
		return nil
	}

	pluginInfoCacheInstance.UpdatedAt = time.Now()
	return WriteJSONCache("plugin_info", pluginInfoCacheInstance)
}

// GetPluginName returns the human-readable name for a plugin slug.
// It checks the cache first, fetches from the API if stale/missing,
// and falls back to returning the slug itself on any failure.
func GetPluginName(slug string) string {
	info := GetPluginInfo(slug)
	if info != nil && info.Name != "" {
		return info.Name
	}
	return slug
}

// sanitizePluginInfo normalizes fields before writing to cache.
func sanitizePluginInfo(info *models.PluginInfo) {
	if info == nil {
		return
	}

	// Decode numeric/named HTML entities in plugin name.
	// Example: "My Plugin &#8211; Lite" -> "My Plugin – Lite"
	info.Name = strings.TrimSpace(html.UnescapeString(info.Name))

	// Strip HTML tags from author field and decode entities.
	// Example: "<a href='...'>Jane Doe</a>" -> "Jane Doe"
	info.Author = strings.TrimSpace(
		html.UnescapeString(
			htmlTagRe.ReplaceAllString(info.Author, ""),
		),
	)
}

// updatePluginInfoInternal handles the actual cache update logic.
func updatePluginInfoInternal(info *models.PluginInfo, isFull bool) bool {
	if info == nil || info.Slug == "" {
		return false
	}

	sanitizePluginInfo(info)

	cache := loadPluginInfoCache()
	pluginInfoMutex.Lock()
	defer pluginInfoMutex.Unlock()

	entry, exists := cache.Plugins[info.Slug]
	updated := false

	if !exists {
		entry = PluginInfoEntry{
			Info: *info,
		}
		if isFull {
			entry.FetchedAt = time.Now()
		}
		if entry.Info.Name == "" {
			entry.Info.Name = info.Slug
		}
		updated = true
	} else {
		if isFull {
			entry.FetchedAt = time.Now()
			entry.Info = *info
			updated = true
		} else {
			// Update name if we have a better one
			if info.Name != "" && (entry.Info.Name == "" || entry.Info.Name == info.Slug) {
				entry.Info.Name = info.Name
				updated = true
			}

			// Update version if the new one is higher
			if info.Version != "" && info.Version != entry.Info.Version {
				vNew, errNew := version.NewVersion(info.Version)
				vOld, errOld := version.NewVersion(entry.Info.Version)

				if errNew == nil && (errOld != nil || vNew.GreaterThan(vOld)) {
					entry.Info.Version = info.Version
					updated = true
				}
			}
		}
	}

	if updated {
		cache.Plugins[info.Slug] = entry
	}

	return updated
}

// UpdatePluginInfo updates or creates a plugin info entry with new data.
// If fullFetch is true, it also updates the FetchAt timestamp.
// It returns true if the cache was modified.
func UpdatePluginInfo(slug, name, ver string, fullFetch ...bool) bool {
	isFull := false
	if len(fullFetch) > 0 && fullFetch[0] {
		isFull = true
	}
	return updatePluginInfoInternal(&models.PluginInfo{
		Slug:    slug,
		Name:    name,
		Version: ver,
	}, isFull)
}

// GetPluginInfo returns the full cached PluginInfo for a slug,
// fetching from the API if stale or missing.
func GetPluginInfo(slug string) *models.PluginInfo {
	cache := loadPluginInfoCache()

	pluginInfoMutex.RLock()
	entry, exists := cache.Plugins[slug]
	pluginInfoMutex.RUnlock()

	// If found and fresh (less than 24h old)
	if exists && time.Since(entry.FetchedAt) < 24*time.Hour {
		return &entry.Info
	}

	// If not found or stale, try to fetch
	info, err := wporg.GetPluginInfo(slug)
	if err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to fetch plugin info for %s: %v\n", slug, err)
		if exists {
			return &entry.Info // Return stale info on failure
		}
		return nil
	}

	// If API returned nothing (not in repo)
	if info == nil {
		if exists {
			return &entry.Info
		}
		return nil
	}

	// Update cache using the version-aware helper, marking as full fetch
	if updatePluginInfoInternal(info, true) {
		_ = SavePluginInfoCache()
	}

	// Fetch again to get the merged result
	pluginInfoMutex.RLock()
	entry, _ = cache.Plugins[slug]
	pluginInfoMutex.RUnlock()

	return &entry.Info
}

// RefreshPluginInfoCache fetches info for all given slugs concurrently,
// updating the cache. Uses a semaphore to limit concurrency.
func RefreshPluginInfoCache(slugs []string) error {
	cache := loadPluginInfoCache()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 24)
	var mu sync.Mutex
	updated := false

	for _, slug := range slugs {
		pluginInfoMutex.RLock()
		entry, exists := cache.Plugins[slug]
		pluginInfoMutex.RUnlock()

		// Skip if fresh
		if exists && time.Since(entry.FetchedAt) < 24*time.Hour {
			continue
		}

		slug := slug
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := wporg.GetPluginInfo(slug)
			if err != nil {
				verb.PrintErrorf(verb.Verbose, "Warning: failed to refresh plugin info for %s: %v\n", slug, err)
				return
			}

			if info != nil {
				mu.Lock()
				if updatePluginInfoInternal(info, true) {
					updated = true
				}
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	if updated {
		return SavePluginInfoCache()
	}

	return nil
}
