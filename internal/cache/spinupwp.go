package cache

import (
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/fetch/spinupwp"
	"github.com/JCO-Digital/jman/internal/models"
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

// GetSiteList retrieves all sites and maps them to CLI-friendly Site models
func GetSiteList() ([]models.CliSite, error) {
	cliSites := []models.CliSite{}
	serverMap, err := GetServerMap()
	if err != nil {
		return nil, err
	}

	sites, err := GetCachedSites()
	if err != nil {
		return nil, err
	}

	for _, site := range sites {
		if !site.IsWordpress {
			continue
		}

		if serverNameFull, ok := serverMap[site.ServerID]; ok {
			serverNameParts := strings.Split(serverNameFull, ".")
			serverName := serverNameParts[0]

			cliSite := models.CliSite{
				ID:         site.ID,
				Name:       site.Domain,
				ServerID:   site.ServerID,
				ServerName: serverName,
				SSH:        fmt.Sprintf("%s@%s", site.SiteUser, serverNameFull),
				Path:       "files",
			}

			cliSites = append(cliSites, cliSite)
		}
	}

	return cliSites, nil
}

// GetFastSiteList retrieves sites from cache without checking expiry.
func GetFastSiteList() ([]models.CliSite, error) {
	cliSites := []models.CliSite{}
	serverMap, err := GetFastServerMap()
	if err != nil {
		return nil, err
	}

	sites, err := GetFastCachedSites()
	if err != nil {
		return nil, err
	}

	for _, site := range sites {
		if !site.IsWordpress {
			continue
		}

		if serverNameFull, ok := serverMap[site.ServerID]; ok {
			serverNameParts := strings.Split(serverNameFull, ".")
			serverName := serverNameParts[0]

			cliSite := models.CliSite{
				ID:         site.ID,
				Name:       site.Domain,
				ServerID:   site.ServerID,
				ServerName: serverName,
				SSH:        fmt.Sprintf("%s@%s", site.SiteUser, serverNameFull),
				Path:       "files",
			}

			cliSites = append(cliSites, cliSite)
		}
	}

	return cliSites, nil
}
