package search

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// SearchSites filters the site list based on the provided query string
func SearchSites(query string) ([]models.CliSite, error) {
	sites, err := cache.GetSiteList()
	if err != nil {
		return nil, err
	}
	return filterSites(sites, query)
}

// SearchSitesFast filters the site list from cache without checking expiry or refreshing
func SearchSitesFast(query string) ([]models.CliSite, error) {
	sites, err := cache.GetFastSiteList()
	if err != nil {
		return nil, nil
	}
	return filterSites(sites, query)
}

func filterSites(sites []models.CliSite, query string) ([]models.CliSite, error) {
	var matched []models.CliSite
	var exact []models.CliSite
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
		return exact, nil
	}

	return matched, nil
}

// SearchPlugins filters the plugin list based on the provided query string
func SearchPlugins(query string) ([]models.WPPluginData, error) {
	plugins, err := cache.GetCachedPluginData()
	if err != nil {
		return nil, err
	}
	return filterPlugins(plugins, query)
}

// SearchPluginsFast filters the plugin list from cache without checking expiry or refreshing
func SearchPluginsFast(query string) ([]models.WPPluginData, error) {
	plugins, err := cache.GetFastCachedPluginData()
	if err != nil {
		return nil, nil
	}
	return filterPlugins(plugins, query)
}

func filterPlugins(plugins []models.WPPluginData, query string) ([]models.WPPluginData, error) {
	var matched []models.WPPluginData
	query = strings.ToLower(query)

	for _, p := range plugins {
		if strings.Contains(strings.ToLower(p.Name), query) {
			matched = append(matched, p)
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})

	return matched, nil
}

// PromptSearch asks the user to confirm the list of filtered sites
func PromptSearch(query string) ([]models.CliSite, error) {
	if query == "" {
		return nil, fmt.Errorf("no query provided")
	}

	sites, err := SearchSites(query)
	if err != nil {
		return nil, err
	}

	if len(sites) == 0 {
		return nil, fmt.Errorf("no sites found")
	}

	reader := bufio.NewReader(os.Stdin)

	if len(sites) == 1 {
		site := sites[0]
		verb.PrintErrorf(verb.Quiet, "Found site: %s %s\n", verb.Blue(site.Name), verb.Gray("("+site.ServerName+")"))
		verb.PrintErrorf(verb.Quiet, "Do you want to run the command on this site? [Y/n]: ")
		response, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
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
		return nil, err
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
			return nil, fmt.Errorf("invalid selection: %s", part)
		}
		if idx < 1 || idx > len(sites) {
			return nil, fmt.Errorf("selection out of range: %d", idx)
		}
		selected = append(selected, sites[idx-1])
	}

	return selected, nil
}
