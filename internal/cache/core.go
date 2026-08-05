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

// GetCachedCoreVersions retrieves the installed WordPress core version for every cached site.
// It uses the database as the primary cache and fetches from sites if data is missing or stale.
func GetCachedCoreVersions(ttl ...time.Duration) ([]models.SiteCore, error) {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	force := t == 0

	existing, err := db.GetAllSiteCore()
	if err != nil {
		return nil, fmt.Errorf("failed to get existing core versions from database: %w", err)
	}

	lastUpdates, err := db.GetSiteCoreLastUpdates()
	if err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to get core version last updates from database: %v\n", err)
		lastUpdates = make(map[int]string)
	}

	sites, err := GetSiteList()
	if err != nil {
		return existing, fmt.Errorf("failed to get site list: %w", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 12)
	updated := false
	var mu sync.Mutex

	for _, site := range sites {
		if !force {
			if lastUpdate, ok := lastUpdates[site.ID]; ok && lastUpdate != "" {
				if t == -1 {
					continue
				}
				lu, err := parseCacheTimestamp(lastUpdate)
				if err == nil && time.Now().UTC().Sub(lu) < t {
					continue
				}
			}
		}

		site := site
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			version, err := wpcli.CoreVersion(site)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Warning: failed to fetch core version for site %s:\n%v\n", verb.Blue(site.Name), verb.Red(err))
				return
			}
			verb.Printf(verb.Verbose, "Fetched core version %s for site %s\n", version, verb.Blue(site.Name))

			if err := db.SaveSiteCore(site.ID, version); err != nil {
				verb.PrintErrorf(verb.Normal, "Warning: failed to save core version for site %s: %v\n", verb.Blue(site.Name), err)
				return
			}

			mu.Lock()
			updated = true
			mu.Unlock()
		}()
	}

	wg.Wait()

	if updated {
		return db.GetAllSiteCore()
	}
	return existing, nil
}

// GetCachedCoreVersionData groups all known sites by their installed WordPress core version.
func GetCachedCoreVersionData() ([]models.CoreVersionData, error) {
	versions, err := GetCachedCoreVersions(DefaultTTL)
	if err != nil {
		return []models.CoreVersionData{}, err
	}

	return groupCoreVersions(versions)
}

// groupCoreVersions is an internal helper to aggregate flat site-core rows into sites grouped by version.
func groupCoreVersions(versions []models.SiteCore) ([]models.CoreVersionData, error) {
	sites, err := GetSiteList()
	if err != nil {
		return []models.CoreVersionData{}, fmt.Errorf("failed to get site list for grouping: %w", err)
	}

	siteNames := make(map[int]string)
	for _, s := range sites {
		siteNames[s.ID] = s.Name
	}

	versionMap := make(map[string]*models.CoreVersionData)

	for _, v := range versions {
		data, exists := versionMap[v.Version]
		if !exists {
			newData := models.CoreVersionData{
				Version: v.Version,
				Sites:   []models.PluginSite{},
			}
			versionMap[v.Version] = &newData
			data = &newData
		}

		data.Sites = append(data.Sites, models.PluginSite{
			SiteID:   v.SiteID,
			SiteName: siteNames[v.SiteID],
			Version:  v.Version,
		})
	}

	versionData := make([]models.CoreVersionData, 0, len(versionMap))
	for _, data := range versionMap {
		versionData = append(versionData, *data)
	}

	return versionData, nil
}

// GetCachedCoreVulnerabilities fetches vulnerability data for a specific WordPress core version
// from the cache or the WPVulnerability API.
func GetCachedCoreVulnerabilities(coreVersion string, ttl ...time.Duration) (*models.CoreVulnResponse, error) {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}

	filename := fmt.Sprintf("vulnerabilities/core/%s", coreVersion)

	var vulnData models.CoreVulnResponse
	if t > 0 {
		err := ReadJSONCache(filename, &vulnData, t)
		if err == nil && vulnData.Error == 0 {
			return &vulnData, nil
		}
	}

	verb.Printf(verb.Verbose, "Fetching core vulnerabilities for %s\n", coreVersion)
	newVulnData, err := wpvuln.GetCoreVulnerabilities(coreVersion)
	if err != nil {
		return nil, err
	}

	if err := WriteJSONCache(filename, newVulnData); err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to write core vulnerability cache for %s: %v\n", coreVersion, err)
	}

	return newVulnData, nil
}
