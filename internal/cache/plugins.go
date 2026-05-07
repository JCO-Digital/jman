package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/fetch/wpvuln"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// GetCachedPlugins retrieves all installed plugins across all cached sites.
// It uses the database as the primary cache and fetches from sites if data is missing or forced.
func GetCachedPlugins(ttl ...time.Duration) ([]models.WPPlugin, error) {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	force := t == 0

	existingPlugins, err := db.GetAllSitePlugins()
	if err != nil {
		return nil, fmt.Errorf("failed to get existing plugins from database: %w", err)
	}

	hasPlugins := make(map[int]bool)
	for _, p := range existingPlugins {
		hasPlugins[p.SiteID] = true
	}

	sites, err := GetSiteList()
	if err != nil {
		return existingPlugins, fmt.Errorf("failed to get site list: %w", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 12)
	updated := false
	var mu sync.Mutex

	for _, site := range sites {
		// Skip sites that already have cached plugins unless forcing a refresh.
		if hasPlugins[site.ID] && !force {
			continue
		}

		site := site
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sitePlugins, err := wpcli.GetPlugins(site, true)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Warning: failed to fetch plugins for site %s:\n%v\n", verb.Blue(site.Name), verb.Red(err))
				return
			}
			verb.Printf(verb.Verbose, "Fetched %d plugins for site %s\n", len(sitePlugins), verb.Blue(site.Name))

			// Clear old entries and save new ones for this site.
			_ = db.DeleteSitePlugins(site.ID)
			for _, p := range sitePlugins {
				_ = db.SaveSitePlugin(p)

				if p.Status == "must-use" || p.Status == "dropin" {
					continue
				}

				// Incrementally update plugin metadata cache (slug/name/version).
				bestVer := p.Version
				if p.Update != "" {
					bestVer = p.Update
				}
				UpdatePluginInfo(p.Name, "", bestVer)
			}

			mu.Lock()
			updated = true
			mu.Unlock()
		}()
	}

	wg.Wait()

	if updated {
		return db.GetAllSitePlugins()
	}
	return existingPlugins, nil
}

// UpdateSitePluginCache fetches current plugins for a specific site and updates the database.
func UpdateSitePluginCache(site models.CliSite) error {
	sitePlugins, err := wpcli.GetPlugins(site, true)
	if err != nil {
		return fmt.Errorf("failed to fetch plugins for site %s: %w", site.Name, err)
	}

	verb.Printf(verb.Verbose, "Fetched %d plugins for site %s to update cache\n", len(sitePlugins), verb.Blue(site.Name))

	// Refresh the database records for this site.
	if err := db.DeleteSitePlugins(site.ID); err != nil {
		return fmt.Errorf("failed to clear site plugins for %s: %w", site.Name, err)
	}

	for _, p := range sitePlugins {
		if err := db.SaveSitePlugin(p); err != nil {
			verb.PrintErrorf(verb.Verbose, "Warning: failed to save plugin %s for site %s: %v\n", p.Name, site.Name, err)
		}

		if p.Status == "must-use" || p.Status == "dropin" {
			continue
		}

		bestVer := p.Version
		if p.Update != "" {
			bestVer = p.Update
		}
		UpdatePluginInfo(p.Name, "", bestVer)
	}

	verb.Printf(verb.Verbose, "Cache updated in database for site %s\n", verb.Blue(site.Name))
	return nil
}

// GetCachedPluginData groups all active plugins into WPPluginData structures for easier scanning.
func GetCachedPluginData() ([]models.WPPluginData, error) {
	plugins, err := GetCachedPlugins(DefaultTTL)
	if err != nil {
		return []models.WPPluginData{}, err
	}

	return groupPlugins(plugins)
}

// GetFastCachedPluginData retrieves plugin data from the database without checking expiry or re-fetching.
func GetFastCachedPluginData() ([]models.WPPluginData, error) {
	plugins, err := db.GetAllSitePlugins()
	if err != nil {
		return []models.WPPluginData{}, err
	}

	return groupPlugins(plugins)
}

// groupPlugins is a internal helper to aggregate a flat plugin list into sites grouped by plugin.
func groupPlugins(plugins []models.WPPlugin) ([]models.WPPluginData, error) {
	sites, err := GetSiteList()
	if err != nil {
		return []models.WPPluginData{}, fmt.Errorf("failed to get site list for grouping: %w", err)
	}

	siteNames := make(map[int]string)
	for _, s := range sites {
		siteNames[s.ID] = s.Name
	}

	pluginMap := make(map[string]*models.WPPluginData)

	for _, plugin := range plugins {
		if plugin.Status != "active" {
			continue
		}

		data, exists := pluginMap[plugin.Name]
		if !exists {
			newData := models.WPPluginData{
				Name:  plugin.Name,
				Sites: []models.PluginSite{},
			}
			pluginMap[plugin.Name] = &newData
			data = &newData
		}

		data.Sites = append(data.Sites, models.PluginSite{
			SiteID:   plugin.SiteID,
			SiteName: siteNames[plugin.SiteID],
			Version:  plugin.Version,
		})
	}

	pluginData := make([]models.WPPluginData, 0, len(pluginMap))
	for _, data := range pluginMap {
		pluginData = append(pluginData, *data)
	}

	return pluginData, nil
}

// GetFastCachedPlugins retrieves plugins from the database without checking expiry.
func GetFastCachedPlugins() ([]models.WPPlugin, error) {
	return db.GetAllSitePlugins()
}

// GetCachedVulnerabilities fetches vulnerability data for a specific plugin from the cache or the WPVulnerability API.
// Note: This still uses JSON files for individual plugin vulnerabilities.
func GetCachedVulnerabilities(plugin string, ttl ...time.Duration) (*models.VulnResponse, error) {
	special, err := isSpecialPlugin(plugin, nil)
	if err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to check if plugin %s is special: %v\n", plugin, err)
	} else if special {
		return nil, nil
	}

	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	filename := fmt.Sprintf("vulnerabilities/%s", plugin)

	var vulnData models.VulnResponse
	if t > 0 {
		err := ReadJSONCache(filename, &vulnData, t)
		if err == nil && vulnData.Error == 0 {
			return &vulnData, nil
		}
	}

	verb.Printf(verb.Verbose, "Fetching vulnerabilities for %s\n", plugin)
	newVulnData, err := wpvuln.GetVulnerabilities(plugin)
	if err != nil {
		return nil, err
	}

	if newVulnData != nil && newVulnData.Data != nil {
		name := ""
		if newVulnData.Data.Name != nil {
			name = *newVulnData.Data.Name
		}
		latest := ""
		UpdatePluginInfo(plugin, name, latest)
	}

	if err := WriteJSONCache(filename, newVulnData); err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to write vulnerability cache for %s: %v\n", plugin, err)
	}

	return newVulnData, nil
}
