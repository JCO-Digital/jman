package commands

import (
	"fmt"
	"sort"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for a specific term across sites.",
	Args:  usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]

		m, err := loadMirror(cmd)
		if err != nil {
			return err
		}
		matchedSites := search.SearchSites(m.CliSites(), query)
		matchedPlugins := search.SearchPlugins(m.PluginData(), query)

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
			verb.Printf(verb.Normal, "Found %d plugins matching '%s':\n", len(matchedPlugins), query)
			for _, plugin := range matchedPlugins {
				verb.Printf(verb.Quiet, "- %s\n", plugin.Name)

				sort.Slice(plugin.Sites, func(i, j int) bool {
					return plugin.Sites[i].SiteName < plugin.Sites[j].SiteName
				})

				for _, site := range plugin.Sites {
					siteName := site.SiteName
					if siteName == "" {
						siteName = fmt.Sprintf("Unknown Site (ID: %s)", site.SiteID)
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
