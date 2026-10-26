package wporg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// CoreChecksumsAPIURL is wordpress.org's core checksums API; a variable so
// tests can point it elsewhere.
var CoreChecksumsAPIURL = "https://api.wordpress.org/core/checksums/1.0/"

// GetCoreChecksums fetches the MD5 checksums of a WordPress release's
// files, keyed by path. It returns nil when wordpress.org has none for the
// version and locale.
func GetCoreChecksums(version, locale string) (map[string]string, error) {
	q := url.Values{"version": {version}, "locale": {locale}}
	client := utils.NewHTTPClient(30 * time.Second)
	req, err := http.NewRequest(http.MethodGet, CoreChecksumsAPIURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create core checksums request: %w", err)
	}
	utils.SetStandardHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch core checksums for %s (%s): %w", version, locale, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch core checksums for %s (%s): API returned status %d", version, locale, resp.StatusCode)
	}

	// "checksums" is false for an unknown version or locale.
	var body struct {
		Checksums json.RawMessage `json:"checksums"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode core checksums for %s (%s): %w", version, locale, err)
	}
	var sums map[string]string
	if err := json.Unmarshal(body.Checksums, &sums); err != nil || len(sums) == 0 {
		return nil, nil
	}
	return sums, nil
}

// PluginChecksumsURL is where wordpress.org publishes plugin checksums; a
// variable so tests can point it elsewhere.
var PluginChecksumsURL = "https://downloads.wordpress.org/plugin-checksums/"

// GetPluginChecksums fetches the SHA-256 checksums of a plugin release's
// files, keyed by path within the plugin. A file can have several. It
// returns nil when wordpress.org has none for the release.
func GetPluginChecksums(slug, version string) (map[string][]string, error) {
	if !utils.IsValidSlug(slug) {
		return nil, nil
	}
	client := utils.NewHTTPClient(30 * time.Second)
	req, err := http.NewRequest(http.MethodGet, PluginChecksumsURL+slug+"/"+url.PathEscape(version)+".json", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin checksums request: %w", err)
	}
	utils.SetStandardHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch checksums for %s %s: %w", slug, version, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch checksums for %s %s: status %d", slug, version, resp.StatusCode)
	}

	// sha256 is a string, or a list when the file changed between builds.
	var body struct {
		Files map[string]struct {
			SHA256 json.RawMessage `json:"sha256"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode checksums for %s %s: %w", slug, version, err)
	}
	sums := make(map[string][]string, len(body.Files))
	for p, f := range body.Files {
		var one string
		var many []string
		switch {
		case json.Unmarshal(f.SHA256, &one) == nil && one != "":
			sums[p] = []string{one}
		case json.Unmarshal(f.SHA256, &many) == nil && len(many) > 0:
			sums[p] = many
		}
	}
	if len(sums) == 0 {
		return nil, nil
	}
	return sums, nil
}
