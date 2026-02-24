package search

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/models"
)

// SearchSites filters the site list based on the provided query string
func SearchSites(query string) ([]models.CliSite, error) {
	sites, err := cache.GetSiteList()
	if err != nil {
		return nil, err
	}

	var matched []models.CliSite
	for _, site := range sites {
		if strings.Contains(site.Name, query) || strings.Contains(site.ServerName, query) {
			matched = append(matched, site)
		}
	}

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

	fmt.Println("Found sites:")
	for _, site := range sites {
		fmt.Printf("%s (%s)\n", site.Name, site.ServerName)
	}

	fmt.Print("Do you want to continue? [Y/n]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" || response == "y" || response == "yes" {
		return sites, nil
	}

	return []models.CliSite{}, nil
}
