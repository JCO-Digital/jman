package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type SiteAlias struct {
	SSH  string `yaml:"ssh"`
	Path string `yaml:"path"`
}

var aliasCmd = &cobra.Command{
	Use:   "alias [target]",
	Short: "Create alias file for all sites, or a custom collection.",
	RunE: func(cmd *cobra.Command, args []string) error {
		searchQuery := ""
		if len(args) > 0 {
			searchQuery = args[0]
		}

		aliasRegistry := make(map[string]any)

		if searchQuery != "" {
			err := createSearchAliases(searchQuery, aliasRegistry)
			if err != nil {
				return err
			}
		} else {
			err := createAllAliases(aliasRegistry)
			if err != nil {
				return err
			}
		}

		yamlData, err := yaml.Marshal(&aliasRegistry)
		if err != nil {
			return fmt.Errorf("failed to generate yaml: %w", err)
		}

		// Print the generated aliases to stdout
		fmt.Println(string(yamlData))
		return nil
	},
}

func createSearchAliases(query string, registry map[string]any) error {
	sites, err := search.SearchSites(query)
	if err != nil {
		return err
	}

	var siteAliases []string
	groupAlias := fmt.Sprintf("@%s", query)

	for _, site := range sites {
		alias := fmt.Sprintf("@%s", site.Name)
		registry[alias] = SiteAlias{
			SSH:  site.SSH,
			Path: site.Path,
		}
		siteAliases = append(siteAliases, alias)
	}

	registry[groupAlias] = siteAliases
	return nil
}

func createAllAliases(registry map[string]any) error {
	servers, err := cache.GetCachedServers()
	if err != nil {
		return fmt.Errorf("failed to get cached servers: %w", err)
	}

	type ServerInfo struct {
		Alias    string
		Hostname string
	}
	serverInfoMap := make(map[int]ServerInfo)
	serverAliasLists := make(map[string][]string)

	for _, server := range servers {
		serverNameParts := strings.Split(server.Name, ".")
		serverAlias := fmt.Sprintf("@%s", serverNameParts[0])
		serverInfoMap[server.ID] = ServerInfo{
			Alias:    serverAlias,
			Hostname: server.Name,
		}
		serverAliasLists[serverAlias] = []string{}
	}

	sites, err := cache.GetCachedSites()
	if err != nil {
		return fmt.Errorf("failed to get cached sites: %w", err)
	}

	for _, site := range sites {
		serverInfo, ok := serverInfoMap[site.ServerID]
		if !ok {
			verbosity.Printf(verbosity.Verbose, "Warning: Server not found for site %s (server_id: %d)\n", site.Domain, site.ServerID)
			continue
		}

		siteAlias := fmt.Sprintf("@%s", site.Domain)
		registry[siteAlias] = SiteAlias{
			SSH:  fmt.Sprintf("%s@%s", site.SiteUser, serverInfo.Hostname),
			Path: "files",
		}

		serverAliasLists[serverInfo.Alias] = append(serverAliasLists[serverInfo.Alias], siteAlias)
	}

	for serverAlias, siteList := range serverAliasLists {
		registry[serverAlias] = siteList
	}

	return nil
}

func init() {
	rootCmd.AddCommand(aliasCmd)
}
