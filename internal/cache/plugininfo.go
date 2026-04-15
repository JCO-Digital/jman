package cache

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/fetch/wporg"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/hashicorp/go-version"
)

var (
	migrationOnce sync.Once
)

// migrateIfNecessary checks if the legacy JSON cache exists and migrates it to SQLite.
func migrateIfNecessary() {
	migrationOnce.Do(func() {
		filename := "plugin_info"
		legacyPath := getCacheFilePath(filename)

		if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
			return
		}

		verb.Printf(verb.Verbose, "Migrating legacy plugin info cache to SQLite...\n")

		cache := &PluginInfoCache{
			Plugins: make(map[string]PluginInfoEntry),
		}

		// Read the old JSON file
		if err := ReadJSONCache(filename, cache, -1); err != nil {
			verb.PrintErrorf(verb.Verbose, "Warning: failed to read legacy cache for migration: %v\n", err)
			return
		}

		// Insert into SQLite
		count := 0
		for _, entry := range cache.Plugins {
			// Note: We can't easily preserve the exact 'FetchedAt' via the simple SavePluginInfo
			// because it defaults to CURRENT_TIMESTAMP, but for a one-time migration
			// this is acceptable as it just resets the 24h TTL.
			info := entry.Info
			sanitizePluginInfo(&info)
			if err := db.SavePluginInfo(info); err == nil {
				count++
			}
		}

		verb.Printf(verb.Verbose, "Migrated %d entries. Removing legacy file: %s\n", count, legacyPath)

		// Backup/Remove the old file
		_ = os.Rename(legacyPath, legacyPath+".bak")
	})
}

// PluginInfoCache and PluginInfoEntry are kept for legacy migration support.
type PluginInfoCache struct {
	Plugins   map[string]PluginInfoEntry `json:"plugins"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

type PluginInfoEntry struct {
	Info      models.PluginInfo `json:"info"`
	FetchedAt time.Time         `json:"fetched_at"`
}

// GetPluginName returns the human-readable name for a plugin slug.
func GetPluginName(slug string) string {
	info := GetPluginInfo(slug, DefaultTTL)
	if info != nil && info.Name != "" {
		return info.Name
	}
	return slug
}

// sanitizePluginInfo normalizes fields before writing to DB.
func sanitizePluginInfo(info *models.PluginInfo) {
	if info == nil {
		return
	}

	info.Name = utils.CleanHTML(info.Name)
	info.Author = utils.CleanHTML(info.Author)
}

// UpdatePluginInfo updates or creates a plugin info entry with new data.
func UpdatePluginInfo(slug, name, ver string, fullFetch ...bool) bool {
	migrateIfNecessary()

	isFull := false
	if len(fullFetch) > 0 && fullFetch[0] {
		isFull = true
	}

	existing, _, err := db.GetPluginInfo(slug)
	if err != nil {
		verb.PrintErrorf(verb.Debug, "DB Error: %v\n", err)
	}

	info := &models.PluginInfo{
		Slug:    slug,
		Name:    name,
		Version: ver,
	}

	if existing == nil {
		if info.Name == "" {
			info.Name = slug
		}
		sanitizePluginInfo(info)
		_ = db.SavePluginInfo(*info)
		return true
	}

	updated := false
	if isFull {
		sanitizePluginInfo(info)
		_ = db.SavePluginInfo(*info)
		return true
	}

	// Partial update logic
	if info.Name != "" && (existing.Name == "" || existing.Name == slug) {
		existing.Name = info.Name
		updated = true
	}

	if info.Version != "" && info.Version != existing.Version {
		vNew, errNew := version.NewVersion(info.Version)
		vOld, errOld := version.NewVersion(existing.Version)

		if errNew == nil && (errOld != nil || vNew.GreaterThan(vOld)) {
			existing.Version = info.Version
			updated = true
		}
	}

	// Always sanitize to ensure any legacy uncleaned data is fixed.
	oldName, oldAuthor := existing.Name, existing.Author
	sanitizePluginInfo(existing)

	if updated || existing.Name != oldName || existing.Author != oldAuthor {
		_ = db.SavePluginInfo(*existing)
		updated = true
	}

	return updated
}

// GetPluginInfo returns the full cached PluginInfo for a slug,
// fetching from the API if stale or missing.
func GetPluginInfo(slug string, ttl ...time.Duration) *models.PluginInfo {
	migrateIfNecessary()

	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	existing, fetchedAt, err := db.GetPluginInfo(slug)
	if err == nil && existing != nil && t > 0 && time.Since(fetchedAt) < t {
		// Ensure data is sanitized even if it's already in the cache.
		oldName, oldAuthor := existing.Name, existing.Author
		sanitizePluginInfo(existing)
		if existing.Name != oldName || existing.Author != oldAuthor {
			_ = db.SavePluginInfo(*existing)
		}
		return existing
	}

	// Fetch from API
	info, err := wporg.GetPluginInfo(slug)
	if err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to fetch plugin info for %s: %v\n", slug, err)
		return existing // Fallback to stale
	}

	if info == nil {
		return existing
	}

	sanitizePluginInfo(info)
	if err := db.SavePluginInfo(*info); err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to save plugin info for %s: %v\n", slug, err)
	}

	return info
}

// RefreshPluginInfoCache fetches info for all given slugs concurrently.
func RefreshPluginInfoCache(slugs []string, ttl ...time.Duration) error {
	migrateIfNecessary()

	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 12)

	for _, slug := range slugs {
		existing, fetchedAt, err := db.GetPluginInfo(slug)
		if err == nil && !fetchedAt.IsZero() && t > 0 && time.Since(fetchedAt) < t {
			if existing != nil {
				oldName, oldAuthor := existing.Name, existing.Author
				sanitizePluginInfo(existing)
				if existing.Name != oldName || existing.Author != oldAuthor {
					_ = db.SavePluginInfo(*existing)
				}
			}
			continue
		}

		slug := slug
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := wporg.GetPluginInfo(slug)
			if err != nil || info == nil {
				return
			}

			sanitizePluginInfo(info)
			_ = db.SavePluginInfo(*info)
		})
	}

	wg.Wait()
	return nil
}

func DisplayPluginName(slug string, truncate, color bool) string {
	name := GetPluginName(slug)
	if name != slug {
		name = utils.CleanHTML(name)

		if truncate {
			name = utils.ShowFirstPart(name)
		}

		if color {
			name = verb.Yellow(name)
			slug = verb.Cyan(slug)
		}

		return fmt.Sprintf("%s (%s)", name, slug)
	}
	if color {
		slug = verb.Yellow(slug)
	}

	return slug
}
