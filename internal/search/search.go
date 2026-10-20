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
// user to pick from them, or, for a single match, to confirm it. With
// confirmSingle false (for read-only commands), a single match is used
// without asking.
func PromptSearch(all []models.CliSite, query string, confirmSingle bool) ([]models.CliSite, error) {
	if query == "" {
		return []models.CliSite{}, fmt.Errorf("no query provided")
	}

	sites := SearchSites(all, query)

	if len(sites) == 0 {
		return []models.CliSite{}, fmt.Errorf("no sites found")
	}

	return PromptSelect(sites, "site", func(site models.CliSite) string {
		return verb.Blue(site.Name) + " " + verb.Gray("("+site.ServerName+")")
	}, confirmSingle)
}

// PromptSelect asks the user to pick from items, shown with label, or, for
// a single item, to confirm it. noun names an item ("site"). With
// confirmSingle false, a single item is used without asking. Picking
// nothing returns an empty slice.
func PromptSelect[T any](items []T, noun string, label func(T) string, confirmSingle bool) ([]T, error) {
	if len(items) == 0 {
		return []T{}, nil
	}

	reader := bufio.NewReader(os.Stdin)

	if len(items) == 1 {
		if !confirmSingle {
			verb.PrintErrorf(verb.Verbose, "Found %s: %s\n", noun, label(items[0]))
			return items, nil
		}
		verb.PrintErrorf(verb.Quiet, "Found %s: %s\n", noun, label(items[0]))
		verb.PrintErrorf(verb.Quiet, "Do you want to run the command on this %s? [Y/n]: ", noun)
		response, err := reader.ReadString('\n')
		if err != nil {
			return []T{}, err
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response == "n" || response == "no" {
			return []T{}, nil
		}
		return items, nil
	}

	verb.PrintErrorf(verb.Normal, "Found %ss:\n", noun)
	for i, item := range items {
		verb.PrintErrorf(verb.Quiet, "[%d] %s\n", i+1, label(item))
	}

	verb.PrintErrorf(verb.Quiet, "%s (empty for all, 'n' to cancel): ", verb.Bold("Enter numbers separated by space"))
	response, err := reader.ReadString('\n')
	if err != nil {
		return []T{}, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" {
		return items, nil
	}

	if response == "n" || response == "no" {
		return []T{}, nil
	}

	parts := strings.Fields(response)
	var selected []T
	for _, part := range parts {
		idx, err := strconv.Atoi(part)
		if err != nil {
			return []T{}, fmt.Errorf("invalid selection: %s", part)
		}
		if idx < 1 || idx > len(items) {
			return []T{}, fmt.Errorf("selection out of range: %d", idx)
		}
		selected = append(selected, items[idx-1])
	}

	return selected, nil
}
