package commands

import (
	"fmt"
	"slices"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch latest data from SpinupWP and update local cache.",
	RunE: func(cmd *cobra.Command, args []string) error {
		target := "basic"
		if len(args) > 0 {
			target = args[0]
		}

		if slices.Contains([]string{"servers", "sites", "basic", "all"}, target) {
			verb.PrintErrorln(verb.Normal, "Fetching latest data from SpinupWP...")
		}

		if slices.Contains([]string{"servers", "basic", "all"}, target) {
			servers, err := cache.RefreshCachedServers()
			if err != nil {
				return fmt.Errorf("error fetching servers: %w", err)
			}
			verb.PrintErrorf(verb.Verbose, "Successfully fetched and cached %d servers.\n", len(servers))
		}

		if slices.Contains([]string{"sites", "basic", "all"}, target) {
			sites, err := cache.RefreshCachedSites()
			if err != nil {
				return fmt.Errorf("error fetching sites: %w", err)
			}
			verb.Printf(verb.Verbose, "Successfully fetched and cached %d sites.\n", len(sites))
		}

		fetchPlugins := slices.Contains([]string{"plugins", "all"}, target)
		fetchVulns := slices.Contains([]string{"vulns", "all"}, target)
		fetchInfo := slices.Contains([]string{"info", "plugins", "all"}, target)

		if fetchPlugins || fetchVulns || fetchInfo {
			if fetchPlugins {
				verb.Print(verb.Verbose, "Fetching plugins.")
			}
			plugins, err := cache.GetCachedPlugins(fetchPlugins)
			if err != nil {
				return fmt.Errorf("error fetching plugins: %w", err)
			}
			if fetchPlugins {
				verb.Printf(verb.Verbose, "Successfully fetched and cached %d plugins.\n", len(plugins))
			}

			// Make a de-duplicated list of plugins.
			pluginList := make(map[string]bool)
			for _, plugin := range plugins {
				pluginList[plugin.Name] = true
			}

			if fetchInfo {
				slugs := make([]string, 0, len(pluginList))
				for slug := range pluginList {
					slugs = append(slugs, slug)
				}
				verb.Printf(verb.Normal, "Fetching plugin info for %d plugins.\n", len(slugs))
				if err := cache.RefreshPluginInfoCache(slugs); err != nil {
					verb.PrintErrorf(verb.Normal, "Warning: failed to refresh plugin info: %v\n", err)
				}
			}

			if fetchVulns {
				verb.Printf(verb.Normal, "Fetching vulnerabilities for %d plugins.\n", len(pluginList))
				for plugin := range pluginList {
					response, err := cache.GetCachedVulnerabilities(plugin, true)
					if err != nil {
						return fmt.Errorf("error fetching vulnerabilities: %w", err)
					}
					verb.Printf(verb.Verbose, "Successfully fetched and cached %d vulnerabilities for %s (%s).\n", len(response.Data.Vulnerability), cache.GetPluginName(plugin), plugin)
				}
			}
		}

		verb.PrintErrorln(verb.Normal, "Cache update complete.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
