package cache

import (
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/fetch/wporg"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
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

// savePluginInfoCache writes the current cache instance to disk.
func savePluginInfoCache() error {
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

	// Update cache
	pluginInfoMutex.Lock()
	cache.Plugins[slug] = PluginInfoEntry{
		Info:      *info,
		FetchedAt: time.Now(),
	}
	pluginInfoMutex.Unlock()

	_ = savePluginInfoCache()

	return info
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
				pluginInfoMutex.Lock()
				cache.Plugins[slug] = PluginInfoEntry{
					Info:      *info,
					FetchedAt: time.Now(),
				}
				pluginInfoMutex.Unlock()
				updated = true
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	if updated {
		return savePluginInfoCache()
	}

	return nil
}
