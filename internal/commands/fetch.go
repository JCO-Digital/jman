package commands

import (
	"fmt"
	"slices"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var (
	forceFetch bool
	fetchCmd   = &cobra.Command{
		Use:           "fetch [servers|sites|plugins|vulns|info|basic|all]",
		Short:         "Fetch latest data from SpinupWP and update local cache.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return []string{"servers", "sites", "plugins", "vulns", "info", "basic", "all"}, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			operation := "basic"
			if len(args) > 0 {
				operation = args[0]
			}

			ttl := cache.FetchTTL
			if forceFetch {
				ttl = 0
			}

			if slices.Contains([]string{"servers", "sites", "basic", "all"}, operation) {
				verb.PrintErrorln(verb.Normal, "Fetching latest data from SpinupWP...")
			}

			if slices.Contains([]string{"servers", "basic", "all"}, operation) {
				servers, err := cache.RefreshCachedServers(ttl)
				if err != nil {
					return fmt.Errorf("error fetching servers: %w", err)
				}
				verb.PrintErrorf(verb.Verbose, "Successfully fetched and cached %d servers.\n", len(servers))
			}

			if slices.Contains([]string{"sites", "basic", "all"}, operation) {
				sites, err := cache.RefreshCachedSites(ttl)
				if err != nil {
					return fmt.Errorf("error fetching sites: %w", err)
				}
				verb.Printf(verb.Verbose, "Successfully fetched and cached %d sites.\n", len(sites))

				classified, err := db.AutoClassifySiteEnvironments(sites)
				if err != nil {
					verb.PrintErrorf(verb.Normal, "Warning: failed to auto-classify site environments: %v\n", err)
				} else if classified > 0 {
					verb.Printf(verb.Verbose, "Auto-classified environment for %d sites.\n", classified)
				}
			}

			fetchPlugins := slices.Contains([]string{"plugins", "all"}, operation)
			fetchVulns := slices.Contains([]string{"vulns", "all"}, operation)
			fetchInfo := slices.Contains([]string{"info", "plugins", "all"}, operation)

			if fetchPlugins || fetchVulns || fetchInfo {
				pTTL := ttl
				if !fetchPlugins && !forceFetch {
					pTTL = cache.DefaultTTL
				}

				if fetchPlugins {
					verb.PrintErrorln(verb.Normal, "Fetching plugins.")
				}

				plugins, err := cache.GetCachedPlugins(pTTL)
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
					if err := cache.RefreshPluginInfoCache(slugs, ttl); err != nil {
						verb.PrintErrorf(verb.Normal, "Warning: failed to refresh plugin info: %v\n", err)
					}
				}

				if fetchVulns {
					verb.Printf(verb.Normal, "Fetching vulnerabilities for %d plugins.\n", len(pluginList))
					for plugin := range pluginList {
						response, err := cache.GetCachedVulnerabilities(plugin, ttl)
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

						verb.Printf(verb.Verbose, "Successfully fetched and cached %d vulnerabilities for %s (%s).\n", len(response.Data.Vulnerability), cache.GetPluginName(plugin), plugin)
					}
				}
			}

			if fetchVulns {
				verb.PrintErrorln(verb.Normal, "Fetching WordPress core vulnerabilities.")

				coreVersions, err := cache.GetCachedCoreVersions(ttl)
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
					response, err := cache.GetCachedCoreVulnerabilities(version, ttl)
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
			}

			verb.PrintErrorln(verb.Normal, "Cache update complete.")
			return nil
		},
	}
)

func init() {
	fetchCmd.Flags().BoolVarP(&forceFetch, "force", "f", false, "Force refresh all caches")
	rootCmd.AddCommand(fetchCmd)
}
