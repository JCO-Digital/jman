package wpvuln

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

const WPVulnerabilityAPIURL = "https://www.wpvulnerability.net/plugin/"

// GetVulnerabilities fetches vulnerability data for a specific WordPress plugin.
func GetVulnerabilities(pluginName string) (*models.VulnResponse, error) {
	if !utils.IsValidSlug(pluginName) {
		msg := "Invalid WordPress plugin slug"
		return &models.VulnResponse{
			Error:   0,
			Message: &msg,
			Data: &models.VulnData{
				Plugin:        pluginName,
				Vulnerability: []models.Vulnerability{},
			},
		}, nil
	}

	endpoint := fmt.Sprintf("%s%s", WPVulnerabilityAPIURL, pluginName)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := utils.NewHTTPClient(15 * time.Second)
	utils.SetStandardHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch vulnerabilities for %s: %w", pluginName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fmt.Sprintf("API returned status %d", resp.StatusCode)
		return &models.VulnResponse{
			Error:   1,
			Message: &msg,
			Data: &models.VulnData{
				Plugin:        pluginName,
				Vulnerability: []models.Vulnerability{},
			},
		}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response for %s: %w", pluginName, err)
	}

	contentType := resp.Header.Get("Content-Type")
	isJSON := strings.Contains(contentType, "application/json") || bytes.HasPrefix(bytes.TrimSpace(body), []byte("{"))

	if !isJSON {
		msg := fmt.Sprintf("API returned non-JSON response (Content-Type: %s)", contentType)
		return &models.VulnResponse{
			Error:   1,
			Message: &msg,
			Data: &models.VulnData{
				Plugin:        pluginName,
				Vulnerability: []models.Vulnerability{},
			},
		}, nil
	}

	var vulnResponse models.VulnResponse
	if err := json.Unmarshal(body, &vulnResponse); err != nil {
		msg := fmt.Sprintf("failed to decode response for %s: %v", pluginName, err)
		return &models.VulnResponse{
			Error:   1,
			Message: &msg,
			Data: &models.VulnData{
				Plugin:        pluginName,
				Vulnerability: []models.Vulnerability{},
			},
		}, nil
	}

	return &vulnResponse, nil
}
