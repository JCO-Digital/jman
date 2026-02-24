package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// DownloadAndReplace downloads the binary from downloadURL, writes it to a
// temporary file, and then replaces the currently running executable with it.
// On Linux the running binary's inode stays valid even after unlinking, so
// the process can continue long enough to print a message and exit.
func DownloadAndReplace(downloadURL string) error {
	// Resolve the path of the currently running executable (follow symlinks).
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	// Grab the permissions of the current binary so we can preserve them.
	info, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("failed to stat current executable: %w", err)
	}
	mode := info.Mode().Perm()

	// Download the new binary to a temporary file in the same directory as the
	// current executable. Using the same directory avoids cross-device rename
	// issues.
	dir := filepath.Dir(execPath)
	tmpFile, err := os.CreateTemp(dir, "jman-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Make sure we clean up the temp file on any error path.
	defer func() {
		// If the temp file still exists at this point, remove it.
		os.Remove(tmpPath)
	}()

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return fmt.Errorf("failed to download update: received status code %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write update to temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Set the same permissions as the original binary (ensure executable).
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("failed to set permissions on new binary: %w", err)
	}

	// Replace the old binary. os.Rename is atomic on the same filesystem.
	if err := os.Rename(tmpPath, execPath); err != nil {
		return fmt.Errorf("failed to replace current binary (do you have write permission?): %w", err)
	}

	return nil
}
