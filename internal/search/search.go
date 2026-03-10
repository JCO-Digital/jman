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
	"github.com/JCO-Digital/jman/internal/verbosity"
)

// SearchSites filters the site list based on the provided query string
func SearchSites(query string) ([]models.CliSite, error) {
	sites, err := cache.GetSiteList()
	if err != nil {
		return nil, err
	}

	var matched []models.CliSite
	query = strings.ToLower(query)
	for _, site := range sites {
		if strings.Contains(strings.ToLower(site.Name), query) || strings.Contains(strings.ToLower(site.ServerName), query) {
			matched = append(matched, site)
		}
	}

	return matched, nil
}

// SearchPlugins filters the plugin list based on the provided query string
func SearchPlugins(query string) ([]models.WPPluginData, error) {
	plugins, err := cache.GetCachedPlugins(false)
	if err != nil {
		return nil, err
	}

	pluginMap := make(map[string]*models.WPPluginData)
	query = strings.ToLower(query)

	for _, p := range plugins {
		if strings.Contains(strings.ToLower(p.Name), query) {
			data, exists := pluginMap[p.Name]
			if !exists {
				newData := &models.WPPluginData{
					Name:  p.Name,
					Sites: []models.PluginSite{},
				}
				pluginMap[p.Name] = newData
				data = newData
			}
			data.Sites = append(data.Sites, models.PluginSite{
				SiteID:  p.SiteID,
				Version: p.Version,
			})
		}
	}

	var matched []models.WPPluginData
	for _, data := range pluginMap {
		matched = append(matched, *data)
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

	verbosity.PrintErrorln(verbosity.Normal, "Found sites:")
	for i, site := range sites {
		verbosity.PrintErrorf(verbosity.Quiet, "[%d] %s (%s)\n", i+1, site.Name, site.ServerName)
	}

	verbosity.PrintErrorf(verbosity.Quiet, "Enter numbers separated by space (empty for all, 'n' to cancel): ")
	reader := bufio.NewReader(os.Stdin)
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
