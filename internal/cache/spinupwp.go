package cache

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/api/spinupwp"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verbosity"
)

// GetCachedServers retrieves servers from the cache or fetches them from the API if expired/missing.
func GetCachedServers() ([]models.Server, error) {
	var servers []models.Server
	err := ReadJSONCache("servers", &servers)
	if err != nil || len(servers) == 0 {
		return RefreshCachedServers()
	}
	return servers, nil
}

// RefreshCachedServers fetches servers from the API and updates the cache.
func RefreshCachedServers() ([]models.Server, error) {
	fmt.Println("Fetching servers from SpinupWP API...")
	servers, err := spinupwp.GetServers()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}

	if err := WriteJSONCache("servers", servers); err != nil {
		verbosity.Printf(verbosity.Verbose, "Warning: Failed to write servers cache: %v\n", err)
	}

	return servers, nil
}

// GetCachedSites retrieves sites from the cache or fetches them from the API if expired/missing.
func GetCachedSites() ([]models.Site, error) {
	var sites []models.Site
	err := ReadJSONCache("sites", &sites)
	if err != nil || len(sites) == 0 {
		return RefreshCachedSites()
	}
	return sites, nil
}

// RefreshCachedSites fetches sites from the API and updates the cache.
func RefreshCachedSites() ([]models.Site, error) {
	fmt.Println("Fetching sites from SpinupWP API...")
	sites, err := spinupwp.GetSites()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sites: %w", err)
	}

	if err := WriteJSONCache("sites", sites); err != nil {
		verbosity.Printf(verbosity.Verbose, "Warning: Failed to write sites cache: %v\n", err)
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

// GetSiteList retrieves all sites and maps them to CLI-friendly Site models
func GetSiteList() ([]models.CliSite, error) {
	var cliSites []models.CliSite
	serverMap, err := GetServerMap()
	if err != nil {
		return nil, err
	}

	sites, err := GetCachedSites()
	if err != nil {
		return nil, err
	}

	for _, site := range sites {
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
