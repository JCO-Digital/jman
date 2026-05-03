package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/fetch/wpvuln"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

var pluginsMu sync.Mutex

// GetCachedPlugins retrieves all installed plugins across all cached sites.
func GetCachedPlugins(ttl ...time.Duration) ([]models.WPPlugin, error) {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	plugins := []models.WPPlugin{}

	force := t == 0
	if !force {
		pluginsMu.Lock()
		_ = ReadJSONCache("plugins", &plugins, t)
		pluginsMu.Unlock()
	}

	sites, err := GetSiteList()
	if err != nil {
		return plugins, fmt.Errorf("failed to get site list: %w", err)
	}

	updated := false
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12)

	for _, site := range sites {
		pluginsMu.Lock()
		// Skip if we already have plugins for this site
		siteHasPlugins := false
		for _, p := range plugins {
			if p.SiteID == site.ID {
				siteHasPlugins = true
				break
			}
		}
		pluginsMu.Unlock()

		if siteHasPlugins && !force {
			continue
		}

		site := site
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			sitePlugins, err := wpcli.GetPlugins(site, true)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Warning: failed to fetch plugins for site %s:\n%v\n", verb.Blue(site.Name), verb.Red(err))
				return
			}
			verb.Printf(verb.Verbose, "Fetched %d plugins for site %s\n", len(sitePlugins), verb.Blue(site.Name))

			pluginsMu.Lock()
			plugins = append(plugins, sitePlugins...)
			updated = true
			pluginsMu.Unlock()
		})
	}

	wg.Wait()

	for _, p := range plugins {
		bestVer := p.Version
		if p.Update != "" {
			bestVer = p.Update
		}
		UpdatePluginInfo(p.Name, "", bestVer)
	}

	if updated {
		pluginsMu.Lock()
		if err := WriteJSONCache("plugins", plugins); err != nil {
			verb.PrintErrorf(verb.Normal, "Warning: failed to write plugins cache: %v\n", err)
		}
		pluginsMu.Unlock()
	}

	return plugins, nil
}

// UpdateSitePluginCache fetches current plugins for a specific site and updates the cache.
func UpdateSitePluginCache(site models.CliSite) error {
	sitePlugins, err := wpcli.GetPlugins(site, true)
	if err != nil {
		return fmt.Errorf("failed to fetch plugins for site %s: %w", site.Name, err)
	}

	verb.Printf(verb.Verbose, "Fetched %d plugins for site %s to update cache\n", len(sitePlugins), verb.Blue(site.Name))

	pluginsMu.Lock()
	defer pluginsMu.Unlock()

	plugins := []models.WPPlugin{}
	_ = ReadJSONCache("plugins", &plugins, -1) // Load existing even if expired

	// Filter out old entries for this site and replace with new ones
	var updatedPlugins []models.WPPlugin
	for _, p := range plugins {
		if p.SiteID != site.ID {
			updatedPlugins = append(updatedPlugins, p)
		}
	}
	updatedPlugins = append(updatedPlugins, sitePlugins...)

	for _, p := range sitePlugins {
		bestVer := p.Version
		if p.Update != "" {
			bestVer = p.Update
		}
		UpdatePluginInfo(p.Name, "", bestVer)
	}

	if err := WriteJSONCache("plugins", updatedPlugins); err != nil {
		return fmt.Errorf("failed to write plugins cache: %w", err)
	}

	verb.Printf(verb.Verbose, "Cache updated for site %s\n", verb.Blue(site.Name))
	return nil
}

// GetCachedPluginData groups all active plugins into WPPluginData structures for easier scanning.
func GetCachedPluginData() ([]models.WPPluginData, error) {
	plugins, err := GetCachedPlugins(DefaultTTL)
	if err != nil {
		return []models.WPPluginData{}, err
	}

	sites, err := GetSiteList()
	if err != nil {
		return []models.WPPluginData{}, err
	}
	siteNames := make(map[int]string)
	for _, s := range sites {
		siteNames[s.ID] = s.Name
	}

	pluginData := []models.WPPluginData{}
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

	for _, data := range pluginMap {
		pluginData = append(pluginData, *data)
	}

	return pluginData, nil
}

// GetFastCachedPluginData retrieves plugin data from cache without checking expiry.
func GetFastCachedPluginData() ([]models.WPPluginData, error) {
	plugins, err := GetFastCachedPlugins()
	if err != nil {
		return []models.WPPluginData{}, err
	}

	sites, err := GetFastSiteList()
	if err != nil {
		return []models.WPPluginData{}, err
	}

	siteNames := make(map[int]string)
	for _, s := range sites {
		siteNames[s.ID] = s.Name
	}

	pluginData := []models.WPPluginData{}
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

	for _, data := range pluginMap {
		pluginData = append(pluginData, *data)
	}

	return pluginData, nil
}

// GetFastCachedPlugins retrieves plugins from the cache without checking expiry.
func GetFastCachedPlugins() ([]models.WPPlugin, error) {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()

	plugins := []models.WPPlugin{}
	if err := ReadJSONCache("plugins", &plugins, -1); err != nil {
		return plugins, err
	}
	return plugins, nil
}

// GetCachedVulnerabilities fetches vulnerability data for a specific plugin from the cache or the WPVulnerability API.
func GetCachedVulnerabilities(plugin string, ttl ...time.Duration) (*models.VulnResponse, error) {
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
