package cache

import (
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/fetch/spinupwp"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// GetCachedServers retrieves servers from the cache or fetches them from the API if expired/missing.
func GetCachedServers(ttl ...time.Duration) ([]models.Server, error) {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}
	return RefreshCachedServers(t)
}

// RefreshCachedServers fetches servers from the API and updates the cache.
// If a ttl is provided and the cache is still valid, it returns the cached data.
func RefreshCachedServers(ttl ...time.Duration) ([]models.Server, error) {
	servers := []models.Server{}
	if len(ttl) > 0 && ttl[0] > 0 {
		if err := ReadJSONCache("servers", &servers, ttl[0]); err == nil && len(servers) > 0 {
			return servers, nil
		}
	}

	if config.Cfg.TokenSpinup == "" {
		verb.Printf(verb.Verbose, "SpinupWP API token not configured; skipping SpinupWP server fetch\n")
		return servers, nil
	}

	verb.PrintErrorln(verb.Normal, "Fetching servers from SpinupWP API...")
	var err error
	servers, err = spinupwp.GetServers()
	if err != nil {
		return servers, fmt.Errorf("failed to fetch servers: %w", err)
	}

	if err := WriteJSONCache("servers", servers); err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: Failed to write servers cache: %v\n", err)
	}

	return servers, nil
}

// GetCachedSites retrieves sites from the cache or fetches them from the API if expired/missing.
func GetCachedSites(ttl ...time.Duration) ([]models.Site, error) {
	t := DefaultTTL
	if len(ttl) > 0 {
		t = ttl[0]
	}
	return RefreshCachedSites(t)
}

// RefreshCachedSites fetches sites from the API and updates the cache.
// If a ttl is provided and the cache is still valid, it returns the cached data.
func RefreshCachedSites(ttl ...time.Duration) ([]models.Site, error) {
	sites := []models.Site{}
	if len(ttl) > 0 && ttl[0] > 0 {
		if err := ReadJSONCache("sites", &sites, ttl[0]); err == nil && len(sites) > 0 {
			return sites, nil
		}
	}

	if config.Cfg.TokenSpinup == "" {
		verb.Printf(verb.Verbose, "SpinupWP API token not configured; skipping SpinupWP site fetch\n")
		return sites, nil
	}

	verb.PrintErrorln(verb.Normal, "Fetching sites from SpinupWP API...")
	var err error
	sites, err = spinupwp.GetSites()
	if err != nil {
		return sites, fmt.Errorf("failed to fetch sites: %w", err)
	}

	if err := WriteJSONCache("sites", sites); err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: Failed to write sites cache: %v\n", err)
	}

	return sites, nil
}

// GetFastCachedServers retrieves servers from the cache without checking expiry.
func GetFastCachedServers() ([]models.Server, error) {
	servers := []models.Server{}
	if err := ReadJSONCache("servers", &servers, -1); err != nil {
		return servers, err
	}
	return servers, nil
}

// GetFastCachedSites retrieves sites from the cache without checking expiry.
func GetFastCachedSites() ([]models.Site, error) {
	sites := []models.Site{}
	if err := ReadJSONCache("sites", &sites, -1); err != nil {
		return sites, err
	}
	return sites, nil
}

// GetServerMap returns a map of server IDs to server names
func GetServerMap() (map[int]string, error) {
	serverMap := make(map[int]string)
	servers, err := GetCachedServers()
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		serverMap[server.ID] = server.Name
	}
	return serverMap, nil
}

// GetFastServerMap returns a map of server IDs to server names from cache without checking expiry.
func GetFastServerMap() (map[int]string, error) {
	serverMap := make(map[int]string)
	servers, err := GetFastCachedServers()
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		serverMap[server.ID] = server.Name
	}
	return serverMap, nil
}

// GetSiteList retrieves all WP-CLI-reachable WordPress sites (SpinupWP sites
// refreshed through the cache, plus external sites from inventory.db).
func GetSiteList() ([]models.CliSite, error) {
	serverMap, err := GetServerMap()
	if err != nil {
		return nil, err
	}

	sites, err := GetCachedSites()
	if err != nil {
		return nil, err
	}

	return buildCliSites(sites, serverMap), nil
}

// GetFastSiteList retrieves sites from cache without checking expiry.
func GetFastSiteList() ([]models.CliSite, error) {
	serverMap, err := GetFastServerMap()
	if err != nil {
		return nil, err
	}

	sites, err := GetFastCachedSites()
	if err != nil {
		return nil, err
	}

	return buildCliSites(sites, serverMap), nil
}

// buildCliSites maps SpinupWP sites to CliSites and appends the external
// (non-SpinupWP) managed sites from inventory.db.
func buildCliSites(sites []models.Site, serverMap map[int]string) []models.CliSite {
	cliSites := []models.CliSite{}
	known := make(map[string]bool, len(sites))

	for _, site := range sites {
		if !site.IsWordpress {
			continue
		}

		serverNameFull, ok := serverMap[site.ServerID]
		if !ok {
			continue
		}

		siteUUID := utils.SpinupWPSiteUUID(site.ID)
		known[siteUUID] = true
		cliSites = append(cliSites, models.CliSite{
			ID:             siteUUID,
			ProviderSiteID: site.ID,
			Provider:       "spinupwp",
			Name:           site.Domain,
			ServerID:       utils.SpinupWPServerUUID(site.ServerID),
			ServerName:     strings.Split(serverNameFull, ".")[0],
			SSH:            fmt.Sprintf("%s@%s", site.SiteUser, serverNameFull),
			Path:           "files",
		})
	}

	managedSites, err := db.ListManagedSites()
	if err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to load managed sites: %v\n", err)
		return cliSites
	}
	for _, ms := range managedSites {
		if known[ms.ID] || ms.Provider == "spinupwp" || !ms.IsWordpress || !ms.CanWPCLI {
			continue
		}
		cliSites = append(cliSites, ms.ToCliSite())
	}

	return cliSites
}

// GetSitesForServer returns every managed site assigned to the given server
// (by UUID), for the agent manifest.
func GetSitesForServer(serverID string) ([]models.ManagedSite, error) {
	sites, err := db.ListManagedSites()
	if err != nil {
		return nil, err
	}

	result := []models.ManagedSite{}
	for _, site := range sites {
		if site.ServerID != nil && *site.ServerID == serverID {
			result = append(result, site)
		}
	}
	return result, nil
}

// MonitorTarget is a site the uptime monitor checks, with the site and server
// UUIDs used to evaluate ignore rules.
type MonitorTarget struct {
	SiteID   string
	ServerID string
	Domain   string
}

// GetMonitorTargets returns every site the uptime monitor should check: all
// SpinupWP sites (refreshed through the cache), plus active external sites
// from inventory.db that have monitoring enabled.
func GetMonitorTargets() ([]MonitorTarget, error) {
	sites, err := GetCachedSites()
	if err != nil {
		return nil, err
	}

	targets := make([]MonitorTarget, 0, len(sites))
	known := make(map[string]bool, len(sites))
	for _, site := range sites {
		siteUUID := utils.SpinupWPSiteUUID(site.ID)
		known[siteUUID] = true
		targets = append(targets, MonitorTarget{
			SiteID:   siteUUID,
			ServerID: utils.SpinupWPServerUUID(site.ServerID),
			Domain:   site.Domain,
		})
	}

	managedSites, err := db.ListManagedSites()
	if err != nil {
		verb.PrintErrorf(verb.Verbose, "Warning: failed to load managed sites for monitoring: %v\n", err)
		return targets, nil
	}
	for _, ms := range managedSites {
		if known[ms.ID] || ms.Provider == "spinupwp" || !ms.HasMonitoring || ms.Status != "active" {
			continue
		}
		serverID := ""
		if ms.ServerID != nil {
			serverID = *ms.ServerID
		}
		targets = append(targets, MonitorTarget{SiteID: ms.ID, ServerID: serverID, Domain: ms.Domain})
	}

	return targets, nil
}
