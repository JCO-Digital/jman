package cache

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/fetch/wporg"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/hashicorp/go-version"
)

// GetPluginName returns the human-readable name for a plugin slug.
func GetPluginName(slug string) string {
	info := GetPluginInfo(slug, DefaultTTL)
	if info != nil && info.Name != "" {
		return info.Name
	}
	return slug
}

// UpdatePluginInfo updates or creates a plugin info entry with new data.
func UpdatePluginInfo(slug, name, ver string, fullFetch ...bool) bool {
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
		models.SanitizePluginInfo(info)
		_ = db.SavePluginInfo(*info)
		return true
	}

	updated := false
	if isFull {
		models.SanitizePluginInfo(info)
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
	models.SanitizePluginInfo(existing)

	if updated || existing.Name != oldName || existing.Author != oldAuthor {
		_ = db.SavePluginInfo(*existing)
		updated = true
	}

	return updated
}

// GetPluginInfo returns the full cached PluginInfo for a slug,
// fetching from the API if stale or missing.
func GetPluginInfo(slug string, ttl ...time.Duration) *models.PluginInfo {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	existing, fetchedAt, err := db.GetPluginInfo(slug)
	if err == nil && existing != nil && t > 0 && time.Since(fetchedAt) < t {
		// Ensure data is sanitized even if it's already in the cache.
		oldName, oldAuthor := existing.Name, existing.Author
		models.SanitizePluginInfo(existing)
		if existing.Name != oldName || existing.Author != oldAuthor {
			_ = db.SavePluginInfo(*existing)
		}
		return existing
	}

	// Skip remote fetches for mu-plugins and dropins as they won't have metadata on WP.org or via 'plugin get'
	if isSpecialPlugin(slug, nil) {
		verb.Printf(verb.Verbose, "Skipping remote metadata fetch for special plugin (mu/dropin): %s\n", slug)
		return existing
	}

	// Fetch from API (WordPress.org)
	verb.Printf(verb.Verbose, "Fetching metadata for %s from WordPress.org...\n", slug)
	info, err := wporg.GetPluginInfo(slug)
	if err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to fetch plugin info for %s from WordPress.org: %v\n", slug, err)
	} else if info != nil {
		verb.Printf(verb.Verbose, "Successfully fetched metadata for %s from WordPress.org.\n", slug)
	}

	// Fallback to WP-CLI if not found on WP.org
	if info == nil {
		verb.Printf(verb.Verbose, "Plugin %s not found on WordPress.org, trying WP-CLI...\n", slug)

		// Pre-fetch site list for fallback
		sites, _ := GetSiteList()
		siteMap := make(map[int]models.CliSite)
		for _, s := range sites {
			siteMap[s.ID] = s
		}

		info, err = fetchPluginInfoFromSites(slug, siteMap, nil)
		if err != nil {
			verb.PrintErrorf(verb.Verbose, "WP-CLI fallback failed for %s: %v\n", slug, err)
		}
	}

	if info == nil {
		return existing
	}

	models.SanitizePluginInfo(info)
	if err := db.SavePluginInfo(*info); err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to save plugin info for %s: %v\n", slug, err)
	}

	return info
}

// RefreshPluginInfoCache fetches info for all given slugs concurrently.
func RefreshPluginInfoCache(slugs []string, ttl ...time.Duration) error {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	// Pre-fetch site list once to avoid redundant cache/DB hits in the fallback loop.
	sites, _ := GetSiteList()
	siteMap := make(map[int]models.CliSite)
	for _, s := range sites {
		siteMap[s.ID] = s
	}

	// Pre-fetch all site plugins to avoid thousands of DB queries.
	allPlugins, _ := db.GetAllSitePlugins()
	pluginToSites := make(map[string][]int)
	isSpecial := make(map[string]bool)
	isRegular := make(map[string]bool)

	for _, p := range allPlugins {
		if p.Status == "must-use" || p.Status == "dropin" {
			isSpecial[p.Name] = true
		} else {
			isRegular[p.Name] = true
			pluginToSites[p.Name] = append(pluginToSites[p.Name], p.SiteID)
		}
	}

	// A plugin is considered "special" (mu/dropin) only if it doesn't exist as a regular plugin anywhere.
	specialOnlyMap := make(map[string]bool)
	for slug := range isSpecial {
		if !isRegular[slug] {
			specialOnlyMap[slug] = true
		}
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 12)

	for _, slug := range slugs {
		existing, fetchedAt, err := db.GetPluginInfo(slug)
		if err == nil && !fetchedAt.IsZero() && t > 0 && time.Since(fetchedAt) < t {
			if existing != nil {
				oldName, oldAuthor := existing.Name, existing.Author
				models.SanitizePluginInfo(existing)
				if existing.Name != oldName || existing.Author != oldAuthor {
					_ = db.SavePluginInfo(*existing)
				}
			}
			continue
		}

		slug := slug
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if isSpecialPlugin(slug, specialOnlyMap) {
				return
			}

			verb.Printf(verb.Verbose, "Fetching metadata for %s from WordPress.org...\n", slug)
			info, err := wporg.GetPluginInfo(slug)
			if err != nil {
				verb.PrintErrorf(verb.Verbose, "Warning: failed to fetch plugin info for %s from WordPress.org: %v\n", slug, err)
			}

			if info == nil {
				verb.Printf(verb.Verbose, "Plugin %s not found on WordPress.org, trying WP-CLI fallback...\n", slug)
				info, err = fetchPluginInfoFromSites(slug, siteMap, pluginToSites)
				if err != nil {
					verb.PrintErrorf(verb.Verbose, "WP-CLI fallback failed for %s: %v\n", slug, err)
				}
			}

			if info != nil {
				verb.Printf(verb.Verbose, "Successfully fetched metadata for %s.\n", slug)
			}

			if info == nil {
				return
			}

			models.SanitizePluginInfo(info)
			_ = db.SavePluginInfo(*info)
		}()
	}

	wg.Wait()
	return nil
}

func isSpecialPlugin(slug string, specialOnlyMap map[string]bool) bool {
	if specialOnlyMap != nil {
		return specialOnlyMap[slug]
	}

	// If it's found as a regular plugin anywhere, we don't treat it as special for fetching purposes.
	siteIDs, _ := db.GetSitesWithPlugin(slug)
	if len(siteIDs) > 0 {
		return false
	}

	dbConn := db.GetDB()
	if dbConn == nil {
		return false
	}

	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM site_plugins WHERE slug = ? AND (status = 'must-use' OR status = 'dropin'))"
	_ = dbConn.QueryRow(query, slug).Scan(&exists)
	return exists
}

// fetchPluginInfoFromSites attempts to get plugin metadata from a site where it is installed.
func fetchPluginInfoFromSites(slug string, siteMap map[int]models.CliSite, pluginToSites map[string][]int) (*models.PluginInfo, error) {
	var siteIDs []int
	var err error

	if pluginToSites != nil {
		siteIDs = pluginToSites[slug]
	} else {
		siteIDs, err = db.GetSitesWithPlugin(slug)
	}

	if err != nil || len(siteIDs) == 0 {
		return nil, err
	}

	// Sort site IDs by failure count to prioritize healthier sites.
	sort.Slice(siteIDs, func(i, j int) bool {
		return wpcli.GetFailureCount(siteIDs[i]) < wpcli.GetFailureCount(siteIDs[j])
	})

	for _, id := range siteIDs {
		site, ok := siteMap[id]
		if !ok {
			continue
		}

		// Only attempt sites that haven't failed too many times recently.
		if wpcli.GetFailureCount(id) > 3 {
			continue
		}

		verb.Printf(verb.Verbose, "Attempting WP-CLI fetch for %s from site %s...\n", slug, site.Name)
		info, err := wpcli.GetPluginInfo(site, slug, 15*time.Second)
		if err == nil && info != nil {
			verb.Printf(verb.Verbose, "Successfully fetched metadata for %s from site %s.\n", slug, site.Name)
			return info, nil
		}
		verb.PrintErrorf(verb.Verbose, "Failed to fetch metadata for %s from site %s: %v\n", slug, site.Name, err)
	}

	return nil, fmt.Errorf("plugin info not found on any site")
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
