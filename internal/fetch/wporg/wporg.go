package wporg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

const PluginInfoAPIURL = "https://api.wordpress.org/plugins/info/1.0/"

// GetPluginInfo fetches metadata for a single plugin by slug from WordPress.org.
func GetPluginInfo(slug string) (*models.PluginInfo, error) {
	if !utils.IsValidSlug(slug) {
		return nil, nil
	}

	endpoint := fmt.Sprintf("%s%s.json", PluginInfoAPIURL, slug)

	client := utils.NewHTTPClient(15 * time.Second)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", slug, err)
	}
	utils.SetStandardHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plugin info for %s: %w", slug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch plugin info for %s: API returned status %d", slug, resp.StatusCode)
	}

	var info models.PluginInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response for %s: %w", slug, err)
	}

	// WordPress.org API returns a "null" or empty response for plugins not in the repo.
	// If the slug is empty in the decoded struct, we treat it as not found.
	if info.Slug == "" {
		return nil, nil
	}

	return &info, nil
}
