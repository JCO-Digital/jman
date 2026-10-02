package cache

import (
	"fmt"
	"sort"
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

	// Keep inventory.db, which WP-CLI targets are built from, in step with
	// the fresh site list. Servers are always refreshed before sites.
	if servers, err := GetFastCachedServers(); err == nil && len(servers) > 0 {
		if err := db.SyncSpinupWPIntoInventory(servers, sites); err != nil {
			verb.PrintErrorf(verb.Normal, "Warning: failed to sync SpinupWP inventory into database: %v\n", err)
		}
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
	servers, err := GetCachedServers()
	if err != nil {
		return nil, err
	}

	sites, err := GetCachedSites()
	if err != nil {
		return nil, err
	}

	return buildCliSites(sites, servers)
}

// GetFastSiteList retrieves sites from cache without checking expiry.
func GetFastSiteList() ([]models.CliSite, error) {
	servers, err := GetFastCachedServers()
	if err != nil {
		return nil, err
	}

	sites, err := GetFastCachedSites()
	if err != nil {
		return nil, err
	}

	return buildCliSites(sites, servers)
}

// buildCliSites returns every WP-CLI-reachable site, built from inventory.db
// through ManagedSite.ToCliSite so SpinupWP and external sites share one
// connection path.
//
// The cached SpinupWP site list stays the source of truth for which
// SpinupWP sites exist and their order: SyncSpinupWPIntoInventory only
// upserts, so inventory can still hold sites since deleted in SpinupWP.
// SpinupWP sites missing from inventory (an inventory that hasn't synced
// since the cache was written) trigger one sync first.
func buildCliSites(sites []models.Site, servers []models.Server) ([]models.CliSite, error) {
	managedSites, err := loadManagedSitesByID()
	if err != nil {
		return nil, err
	}

	if hasUnsyncedSpinupWPSites(sites, managedSites) && len(servers) > 0 {
		if err := db.SyncSpinupWPIntoInventory(servers, sites); err != nil {
			verb.PrintErrorf(verb.Normal, "Warning: failed to sync SpinupWP inventory into database: %v\n", err)
		} else if managedSites, err = loadManagedSitesByID(); err != nil {
			return nil, err
		}
	}

	cliSites := []models.CliSite{}
	spinupIDs := make(map[string]bool, len(sites))
	for _, site := range sites {
		if !site.IsWordpress {
			continue
		}
		id := utils.SpinupWPSiteUUID(site.ID)
		spinupIDs[id] = true
		ms, ok := managedSites[id]
		// No SSH host means the site's server isn't in the cached server
		// list, so there is nothing to connect to.
		if !ok || !ms.IsWordpress || !ms.CanWPCLI || ms.SSHHost == "" {
			continue
		}
		cliSites = append(cliSites, ms.ToCliSite())
	}

	external := make([]models.ManagedSite, 0, len(managedSites))
	for _, ms := range managedSites {
		if spinupIDs[ms.ID] || ms.Provider == "spinupwp" || !ms.IsWordpress || !ms.CanWPCLI {
			continue
		}
		external = append(external, ms)
	}
	sort.Slice(external, func(i, j int) bool { return external[i].Domain < external[j].Domain })
	for _, ms := range external {
		cliSites = append(cliSites, ms.ToCliSite())
	}

	return cliSites, nil
}

func loadManagedSitesByID() (map[string]models.ManagedSite, error) {
	list, err := db.ListManagedSites()
	if err != nil {
		return nil, fmt.Errorf("failed to load managed sites: %w", err)
	}
	byID := make(map[string]models.ManagedSite, len(list))
	for _, ms := range list {
		byID[ms.ID] = ms
	}
	return byID, nil
}

func hasUnsyncedSpinupWPSites(sites []models.Site, managed map[string]models.ManagedSite) bool {
	for _, site := range sites {
		if _, ok := managed[utils.SpinupWPSiteUUID(site.ID)]; !ok {
			return true
		}
	}
	return false
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
