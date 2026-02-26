package commands

import (
	"fmt"
	"sort"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for a specific term across sites.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]

		matchedSites, err := search.SearchSites(query)
		if err != nil {
			return fmt.Errorf("error searching sites: %w", err)
		}

		matchedPlugins, err := search.SearchPlugins(query)
		if err != nil {
			return fmt.Errorf("error searching plugins: %w", err)
		}

		if len(matchedSites) == 0 && len(matchedPlugins) == 0 {
			verbosity.Printf(verbosity.Normal, "No sites or plugins found matching '%s'.\n", query)
			return nil
		}

		if len(matchedSites) > 0 {
			verbosity.Printf(verbosity.Normal, "Found %d sites matching '%s':\n", len(matchedSites), query)
			for _, site := range matchedSites {
				verbosity.Printf(verbosity.Quiet, "- %s (Server: %s)\n", site.Name, site.ServerName)
			}
			if len(matchedPlugins) > 0 {
				fmt.Println()
			}
		}

		if len(matchedPlugins) > 0 {
			allSites, err := cache.GetSiteList()
			if err != nil {
				return fmt.Errorf("error getting site list: %w", err)
			}
			siteMap := make(map[int]string)
			for _, s := range allSites {
				siteMap[s.ID] = s.Name
			}

			verbosity.Printf(verbosity.Normal, "Found %d plugins matching '%s':\n", len(matchedPlugins), query)
			for _, plugin := range matchedPlugins {
				verbosity.Printf(verbosity.Quiet, "- %s\n", plugin.Name)

				sort.Slice(plugin.Sites, func(i, j int) bool {
					return siteMap[plugin.Sites[i].SiteID] < siteMap[plugin.Sites[j].SiteID]
				})

				for _, site := range plugin.Sites {
					siteName := siteMap[site.SiteID]
					if siteName == "" {
						siteName = fmt.Sprintf("Unknown Site (ID: %d)", site.SiteID)
					}
					verbosity.Printf(verbosity.Quiet, "  * %s (%s)\n", siteName, site.Version)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
