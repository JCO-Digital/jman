package spinupwp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

const APIBaseURL = "https://api.spinupwp.app/v1"

// Pagination represents the pagination metadata returned by the SpinupWP API.
type Pagination struct {
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	PerPage  int     `json:"per_page"`
	Count    int     `json:"count"`
}

// spinupResponse is a generic struct for parsing API responses containing an array of resources.
type spinupResponse[T any] struct {
	Data       []T         `json:"data"`
	Pagination *Pagination `json:"pagination"`
}

// makeRequest handles the HTTP request and decoding for paginated resources.
func makeRequest[T any](endpoint string) ([]T, *Pagination, error) {
	fmt.Printf("Making a GET request to %s\n", endpoint)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if config.Cfg.TokenSpinup == "" {
		return nil, nil, fmt.Errorf("SpinupWP API token is not configured")
	}
	req.Header.Set("Authorization", "Bearer "+config.Cfg.TokenSpinup)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var payload spinupResponse[T]
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return payload.Data, payload.Pagination, nil
}

// GetServers fetches all servers from the SpinupWP API, handling pagination.
func GetServers() ([]models.Server, error) {
	endpoint := APIBaseURL + "/servers"
	var allServers []models.Server

	for endpoint != "" {
		servers, pagination, err := makeRequest[models.Server](endpoint)
		if err != nil {
			return nil, err
		}

		allServers = append(allServers, servers...)

		if pagination != nil && pagination.Next != nil {
			endpoint = *pagination.Next
		} else {
			endpoint = ""
		}
	}

	return allServers, nil
}

// GetSites fetches all sites from the SpinupWP API, handling pagination.
func GetSites() ([]models.Site, error) {
	endpoint := APIBaseURL + "/sites"
	var allSites []models.Site

	for endpoint != "" {
		sites, pagination, err := makeRequest[models.Site](endpoint)
		if err != nil {
			return nil, err
		}

		allSites = append(allSites, sites...)

		if pagination != nil && pagination.Next != nil {
			endpoint = *pagination.Next
		} else {
			endpoint = ""
		}
	}

	return allSites, nil
}
