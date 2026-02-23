package wpvuln

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/JCO-Digital/jman/internal/models"
)

const WPVulnerabilityAPIURL = "https://www.wpvulnerability.net/plugin/"

// GetVulnerabilities fetches vulnerability data for a specific WordPress plugin.
func GetVulnerabilities(pluginName string) (*models.VulnResponse, error) {
	endpoint := fmt.Sprintf("%s%s", WPVulnerabilityAPIURL, pluginName)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch vulnerabilities for %s: %w", pluginName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch vulnerabilities for %s: API returned status %d", pluginName, resp.StatusCode)
	}

	var vulnResponse models.VulnResponse
	if err := json.NewDecoder(resp.Body).Decode(&vulnResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response for %s: %w", pluginName, err)
	}

	return &vulnResponse, nil
}
