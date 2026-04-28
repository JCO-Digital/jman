package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/spf13/cobra"
)

func getSiteCompletions(toComplete string) ([]string, cobra.ShellCompDirective) {
	sites, err := search.SearchSitesFast(toComplete)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var completions []string
	for _, site := range sites {
		completions = append(completions, fmt.Sprintf("%s\t%s", site.Name, site.ServerName))
		if site.Name != site.ServerName && site.ServerName != "" {
			completions = append(completions, fmt.Sprintf("%s", site.ServerName))
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
