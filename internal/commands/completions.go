package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/spf13/cobra"
)

func getSiteCompletions() ([]string, cobra.ShellCompDirective) {
	sites, err := cache.GetFastSiteList()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	servers, err := cache.GetFastCachedServers()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var completions []string

	for _, server := range servers {
		shortName := strings.Split(server.Name, ".")[0]
		completions = append(completions, fmt.Sprintf("%s\t%s", shortName, server.Name))
	}

	for _, site := range sites {
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
	plugins, err := search.SearchPluginsFast(toComplete)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	for _, p := range plugins {
		names = append(names, p.Name)
	}

	if allowFiles {
		return names, cobra.ShellCompDirectiveDefault
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
