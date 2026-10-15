package search

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// SearchSites returns the sites whose domain or server name matches query
// (case-insensitively). Exact matches win: if any site or server name
// equals query, only those sites are returned.
func SearchSites(sites []models.CliSite, query string) []models.CliSite {
	return filterSites(sites, query)
}

func filterSites(sites []models.CliSite, query string) []models.CliSite {
	matched := []models.CliSite{}
	exact := []models.CliSite{}
	query = strings.ToLower(query)

	for _, site := range sites {
		name := strings.ToLower(site.Name)
		server := strings.ToLower(site.ServerName)
		if name == query || server == query {
			exact = append(exact, site)
		}
		if strings.Contains(name, query) || strings.Contains(server, query) {
			matched = append(matched, site)
		}
	}

	if len(exact) > 0 {
		return exact
	}

	return matched
}

// SearchPlugins returns the plugins whose slug contains query
// (case-insensitively), sorted by slug.
func SearchPlugins(plugins []models.WPPluginData, query string) []models.WPPluginData {
	return filterPlugins(plugins, query)
}

func filterPlugins(plugins []models.WPPluginData, query string) []models.WPPluginData {
	matched := []models.WPPluginData{}
	query = strings.ToLower(query)

	for _, p := range plugins {
		if strings.Contains(strings.ToLower(p.Name), query) {
			matched = append(matched, p)
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})

	return matched
}

// PromptSearch finds the sites matching query among sites and asks the
// user to confirm or pick from them.
func PromptSearch(all []models.CliSite, query string) ([]models.CliSite, error) {
	if query == "" {
		return []models.CliSite{}, fmt.Errorf("no query provided")
	}

	sites := SearchSites(all, query)

	if len(sites) == 0 {
		return []models.CliSite{}, fmt.Errorf("no sites found")
	}

	reader := bufio.NewReader(os.Stdin)

	if len(sites) == 1 {
		site := sites[0]
		verb.PrintErrorf(verb.Quiet, "Found site: %s %s\n", verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
		verb.PrintErrorf(verb.Quiet, "Do you want to run the command on this site? [Y/n]: ")
		response, err := reader.ReadString('\n')
		if err != nil {
			return []models.CliSite{}, err
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response == "n" || response == "no" {
			return []models.CliSite{}, nil
		}
		return sites, nil
	}

	verb.PrintErrorln(verb.Normal, "Found sites:")
	for i, site := range sites {
		verb.PrintErrorf(verb.Quiet, "[%d] %s %s\n", i+1, verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
	}

	verb.PrintErrorf(verb.Quiet, "%s (empty for all, 'n' to cancel): ", verb.Bold("Enter numbers separated by space"))
	response, err := reader.ReadString('\n')
	if err != nil {
		return []models.CliSite{}, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" {
		return sites, nil
	}

	if response == "n" || response == "no" {
		return []models.CliSite{}, nil
	}

	parts := strings.Fields(response)
	var selected []models.CliSite
	for _, part := range parts {
		idx, err := strconv.Atoi(part)
		if err != nil {
			return []models.CliSite{}, fmt.Errorf("invalid selection: %s", part)
		}
		if idx < 1 || idx > len(sites) {
			return []models.CliSite{}, fmt.Errorf("selection out of range: %d", idx)
		}
		selected = append(selected, sites[idx-1])
	}

	return selected, nil
}
