package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/spf13/cobra"
)

func getSiteCompletions() ([]string, cobra.ShellCompDirective) {
	m := completionMirror()

	var completions []string
	seen := map[string]bool{}
	for _, site := range m.CliSites() {
		// Servers complete by their short name, as site search matches it.
		if server := site.ServerName; server != "" && !seen[server] {
			seen[server] = true
			completions = append(completions, fmt.Sprintf("%s\tserver", server))
		}
	}

	for _, site := range m.CliSites() {
		name := site.Name
		server := site.ServerName
		if len(name) >= 4 && strings.HasPrefix(strings.ToLower(name), "www.") {
			completions = append(completions, fmt.Sprintf("%s\t%s", name[4:], server))
		}
		completions = append(completions, fmt.Sprintf("%s\t%s", name, server))
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func getPluginCompletions(toComplete string, allowFiles bool) ([]string, cobra.ShellCompDirective) {
	var names []string
	for _, p := range search.SearchPlugins(completionMirror().PluginData(), toComplete) {
		names = append(names, p.Name)
	}

	if allowFiles {
		return names, cobra.ShellCompDirectiveDefault
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
