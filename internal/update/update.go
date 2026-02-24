package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-version"
)

const LatestReleaseURL = "https://api.github.com/repos/JCO-Digital/jman/releases/latest"

// Release represents the simplified structure of a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a GitHub release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckForUpdate checks if a newer version of the CLI is available.
// It returns the latest version string and a boolean indicating if an update is available.
// CheckForUpdate checks if a newer version of the CLI is available.
// It returns the latest version string, the release URL, and a boolean indicating if an update is available.
func CheckForUpdate(currentVersion string) (string, string, bool, error) {

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(LatestReleaseURL)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("failed to check for updates: received status code %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", false, fmt.Errorf("failed to decode release data: %w", err)
	}

	downloadURL := release.HTMLURL
	for _, asset := range release.Assets {
		if asset.Name == "jman" {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	vCurrent, err := version.NewVersion(currentVersion)
	if err != nil {
		// If current version is not semver (and not "dev"), we can't reliably compare.
		return "", "", false, nil
	}

	vLatest, err := version.NewVersion(release.TagName)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to parse latest version %s: %w", release.TagName, err)
	}

	if vLatest.GreaterThan(vCurrent) {
		return release.TagName, downloadURL, true, nil
	}

	return release.TagName, downloadURL, false, nil
}
