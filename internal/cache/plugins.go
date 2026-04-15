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

// GetCachedPlugins retrieves all installed plugins across all cached sites.
func GetCachedPlugins(ttl ...time.Duration) ([]models.WPPlugin, error) {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	var plugins []models.WPPlugin

	force := t == 0
	if !force {
		_ = ReadJSONCache("plugins", &plugins, t)
	}

	sites, err := GetSiteList()
	if err != nil {
		return nil, fmt.Errorf("failed to get site list: %w", err)
	}

	updated := false
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 24)

	for _, site := range sites {
		mu.Lock()
		// Skip if we already have plugins for this site
		siteHasPlugins := false
		for _, p := range plugins {
			if p.SiteID == site.ID {
				siteHasPlugins = true
				break
			}
		}
		mu.Unlock()

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
			verb.PrintErrorf(verb.Verbose, "Fetched %d plugins for site %s\n", len(sitePlugins), verb.Blue(site.Name))

			mu.Lock()
			plugins = append(plugins, sitePlugins...)
			updated = true
			mu.Unlock()
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
		if err := WriteJSONCache("plugins", plugins); err != nil {
			verb.PrintErrorf(verb.Normal, "Warning: failed to write plugins cache: %v\n", err)
		}
	}

	return plugins, nil
}

// GetCachedPluginData groups all active plugins into WPPluginData structures for easier scanning.
func GetCachedPluginData() ([]models.WPPluginData, error) {
	plugins, err := GetCachedPlugins(DefaultTTL)
	if err != nil {
		return nil, err
	}

	sites, err := GetSiteList()
	if err != nil {
		return nil, err
	}
	siteNames := make(map[int]string)
	for _, s := range sites {
		siteNames[s.ID] = s.Name
	}

	var pluginData []models.WPPluginData
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
