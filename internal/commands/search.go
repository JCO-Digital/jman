package commands

import (
	"fmt"
	"sort"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for a specific term across sites.",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
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
			verb.Printf(verb.Normal, "No sites or plugins found matching '%s'.\n", query)
			return nil
		}

		if len(matchedSites) > 0 {
			verb.Printf(verb.Normal, "Found %d sites matching '%s':\n", len(matchedSites), query)
			for _, site := range matchedSites {
				verb.Printf(verb.Quiet, "- %s (Server: %s)\n", site.Name, site.ServerName)
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

			verb.Printf(verb.Normal, "Found %d plugins matching '%s':\n", len(matchedPlugins), query)
			for _, plugin := range matchedPlugins {
				verb.Printf(verb.Quiet, "- %s\n", plugin.Name)

				sort.Slice(plugin.Sites, func(i, j int) bool {
					return siteMap[plugin.Sites[i].SiteID] < siteMap[plugin.Sites[j].SiteID]
				})

				for _, site := range plugin.Sites {
					siteName := siteMap[site.SiteID]
					if siteName == "" {
						siteName = fmt.Sprintf("Unknown Site (ID: %d)", site.SiteID)
					}
					verb.Printf(verb.Quiet, "  * %s (%s)\n", siteName, site.Version)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
