package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/search"
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
	Args:  usageArgs,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		searchQuery := ""
		if len(args) > 0 {
			searchQuery = args[0]
		}

		m, err := loadMirror(cmd)
		if err != nil {
			return err
		}

		aliasRegistry := make(map[string]any)
		if searchQuery != "" {
			createSearchAliases(m, searchQuery, aliasRegistry)
		} else {
			createAllAliases(m, aliasRegistry)
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

func createSearchAliases(m *mirror.Mirror, query string, registry map[string]any) {
	var siteAliases []string
	for _, site := range search.SearchSites(m.CliSites(), query) {
		alias := fmt.Sprintf("@%s", site.Name)
		registry[alias] = SiteAlias{
			SSH:  site.SSH,
			Path: site.Path,
		}
		siteAliases = append(siteAliases, alias)
	}
	registry[fmt.Sprintf("@%s", query)] = siteAliases
}

// createAllAliases adds an alias for every site jman can reach over SSH,
// and a group alias per server (by its short name) listing its sites.
// SpinupWP sites are included even when they aren't WordPress, so the
// aliases also work for plain SSH access.
func createAllAliases(m *mirror.Mirror, registry map[string]any) {
	serverAliasLists := make(map[string][]string)
	for _, server := range m.Servers() {
		if server.Provider == "spinupwp" {
			serverAliasLists[fmt.Sprintf("@%s", strings.Split(server.Name, ".")[0])] = []string{}
		}
	}

	for _, site := range m.Sites() {
		if site.SSHHost == "" || (site.Provider != "spinupwp" && (!site.IsWordpress || !site.CanWPCLI)) {
			continue
		}
		siteAlias := fmt.Sprintf("@%s", site.Domain)
		if _, exists := registry[siteAlias]; exists {
			continue
		}
		cli := mirror.ToCliSite(site)
		registry[siteAlias] = SiteAlias{SSH: cli.SSH, Path: cli.Path}

		if cli.ServerName != "" {
			srvAlias := fmt.Sprintf("@%s", cli.ServerName)
			serverAliasLists[srvAlias] = append(serverAliasLists[srvAlias], siteAlias)
		}
	}

	for serverAlias, siteList := range serverAliasLists {
		registry[serverAlias] = siteList
	}
}

func init() {
	rootCmd.AddCommand(aliasCmd)
}
