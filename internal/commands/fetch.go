package commands

import (
	"fmt"
	"slices"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/verbosity"
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
			verbosity.PrintErrorln(verbosity.Normal, "Fetching latest data from SpinupWP...")
		}

		if slices.Contains([]string{"servers", "basic", "all"}, target) {
			servers, err := cache.RefreshCachedServers()
			if err != nil {
				return fmt.Errorf("error fetching servers: %w", err)
			}
			verbosity.PrintErrorf(verbosity.Verbose, "Successfully fetched and cached %d servers.\n", len(servers))
		}

		if slices.Contains([]string{"sites", "basic", "all"}, target) {
			sites, err := cache.RefreshCachedSites()
			if err != nil {
				return fmt.Errorf("error fetching sites: %w", err)
			}
			verbosity.Printf(verbosity.Verbose, "Successfully fetched and cached %d sites.\n", len(sites))
		}

		fetchPlugins := slices.Contains([]string{"plugins", "all"}, target)
		fetchVulns := slices.Contains([]string{"vulns", "all"}, target)

		if fetchPlugins || fetchVulns {
			if fetchPlugins {
				verbosity.Print(verbosity.Verbose, "Fetching plugins.")
			}
			plugins, err := cache.GetCachedPlugins(fetchPlugins)
			if err != nil {
				return fmt.Errorf("error fetching plugins: %w", err)
			}
			if fetchPlugins {
				verbosity.Printf(verbosity.Verbose, "Successfully fetched and cached %d plugins.\n", len(plugins))
			}

			if fetchVulns {
				// Make a de-duplicated list.
				pluginList := make(map[string]bool)
				for _, plugin := range plugins {
					pluginList[plugin.Name] = true
				}
				verbosity.Printf(verbosity.Normal, "Fetching vulnerabilities for %d plugins.\n", len(pluginList))
				for plugin, _ := range pluginList {
					response, err := cache.GetCachedVulnerabilities(plugin, true)
					if err != nil {
						return fmt.Errorf("error fetching vulnerabilities: %w", err)
					}
					verbosity.Printf(verbosity.Verbose, "Successfully fetched and cached %d vulnerabilities.\n", len(response.Data.Vulnerability))
				}
			}
		}

		verbosity.PrintErrorln(verbosity.Normal, "Cache update complete.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
