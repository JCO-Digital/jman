package cache

import (
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// RefreshServersAndSites refreshes the cached SpinupWP server and site lists
// and auto-classifies any newly-seen site environments. This is the cheap
// half of a full refresh (a couple of HTTP calls, no per-site fan-out), used
// both by `jman fetch` and the in-process refresh scheduler's fast tick.
func RefreshServersAndSites(ttl time.Duration) ([]models.Server, []models.Site, error) {
	servers, err := RefreshCachedServers(ttl)
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching servers: %w", err)
	}
	verb.Printf(verb.Verbose, "Successfully fetched and cached %d servers.\n", len(servers))

	sites, err := RefreshCachedSites(ttl)
	if err != nil {
		return servers, nil, fmt.Errorf("error fetching sites: %w", err)
	}
	verb.Printf(verb.Verbose, "Successfully fetched and cached %d sites.\n", len(sites))

	classified, err := db.AutoClassifySiteEnvironments(sites)
	if err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to auto-classify site environments: %v\n", err)
	} else if classified > 0 {
		verb.Printf(verb.Verbose, "Auto-classified environment for %d sites.\n", classified)
	}

	return servers, sites, nil
}

// RunFullRefresh refreshes installed plugins, plugin metadata, per-plugin
// vulnerabilities, WordPress core versions, and per-core-version
// vulnerabilities for every cached site. This is the expensive half of a
// full refresh (SSH/wp-cli fan-out across every managed site, bounded by the
// existing per-call concurrency limits in this package), used both by
// `jman fetch` and the in-process refresh scheduler's slow tick.
func RunFullRefresh(ttl time.Duration) error {
	verb.PrintErrorln(verb.Normal, "Fetching plugins.")
	plugins, err := GetCachedPlugins(ttl)
	if err != nil {
		return fmt.Errorf("error fetching plugins: %w", err)
	}
	verb.Printf(verb.Verbose, "Successfully fetched and cached %d plugins.\n", len(plugins))

	// De-duplicated list of plugin slugs.
	pluginList := make(map[string]bool)
	for _, plugin := range plugins {
		pluginList[plugin.Name] = true
	}

	slugs := make([]string, 0, len(pluginList))
	for slug := range pluginList {
		slugs = append(slugs, slug)
	}
	verb.Printf(verb.Normal, "Fetching plugin info for %d plugins.\n", len(slugs))
	if err := RefreshPluginInfoCache(slugs, ttl); err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to refresh plugin info: %v\n", err)
	}

	verb.Printf(verb.Normal, "Fetching vulnerabilities for %d plugins.\n", len(pluginList))
	for plugin := range pluginList {
		response, err := GetCachedVulnerabilities(plugin, ttl)
		if err != nil {
			verb.PrintErrorf(verb.Normal, "Warning: failed to fetch vulnerabilities for %s: %v\n", plugin, err)
			continue
		}

		if response == nil {
			continue
		}

		if response.Error != 0 {
			msg := "unknown error"
			if response.Message != nil {
				msg = *response.Message
			}
			verb.PrintErrorf(verb.Verbose, "Warning: API returned error for %s: %s\n", plugin, msg)
			continue
		}

		verb.Printf(verb.Verbose, "Successfully fetched and cached %d vulnerabilities for %s (%s).\n", len(response.Data.Vulnerability), GetPluginName(plugin), plugin)
	}

	verb.PrintErrorln(verb.Normal, "Fetching WordPress core vulnerabilities.")
	coreVersions, err := GetCachedCoreVersions(ttl)
	if err != nil {
		return fmt.Errorf("error fetching core versions: %w", err)
	}
	verb.Printf(verb.Verbose, "Successfully fetched and cached core versions for %d sites.\n", len(coreVersions))

	coreVersionList := make(map[string]bool)
	for _, v := range coreVersions {
		coreVersionList[v.Version] = true
	}

	verb.Printf(verb.Normal, "Fetching vulnerabilities for %d WordPress core versions.\n", len(coreVersionList))
	for version := range coreVersionList {
		response, err := GetCachedCoreVulnerabilities(version, ttl)
		if err != nil {
			verb.PrintErrorf(verb.Normal, "Warning: failed to fetch vulnerabilities for WordPress core %s: %v\n", version, err)
			continue
		}

		if response == nil {
			continue
		}

		if response.Error != 0 {
			msg := "unknown error"
			if response.Message != nil {
				msg = *response.Message
			}
			verb.PrintErrorf(verb.Verbose, "Warning: API returned error for WordPress core %s: %s\n", version, msg)
			continue
		}

		verb.Printf(verb.Verbose, "Successfully fetched and cached %d vulnerabilities for WordPress core %s.\n", len(response.Data.Vulnerability), version)
	}

	return nil
}
