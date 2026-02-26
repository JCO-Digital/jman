package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/hashicorp/go-version"
)

// progressReader wraps an io.Reader and prints download progress to stdout.
type progressReader struct {
	reader  io.Reader
	total   int64 // total expected bytes, or -1 if unknown
	read    int64
	lastPct int
	mu      sync.Mutex
}

func newProgressReader(r io.Reader, total int64) *progressReader {
	return &progressReader{
		reader:  r,
		total:   total,
		lastPct: -1,
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.mu.Lock()
		pr.read += int64(n)
		pr.print()
		pr.mu.Unlock()
	}
	return n, err
}

func (pr *progressReader) print() {
	readMB := float64(pr.read) / (1024 * 1024)
	if pr.total > 0 {
		pct := int(float64(pr.read) * 100 / float64(pr.total))
		// Only redraw when the percentage changes to avoid excessive writes.
		if pct != pr.lastPct {
			pr.lastPct = pct
			totalMB := float64(pr.total) / (1024 * 1024)
			verbosity.Printf(verbosity.Verbose, "\r  Downloading: %3d%%  (%.1f / %.1f MB)", pct, readMB, totalMB)
		}
	} else {
		verbosity.Printf(verbosity.Verbose, "\r  Downloading: %.1f MB", readMB)
	}
}

func (pr *progressReader) finish() {
	fmt.Println() // move past the \r line
}

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
// It returns the latest version string, the release URL for the specified component, and a boolean indicating if an update is available.
func CheckForUpdate(currentVersion string, component string) (string, string, bool, error) {

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

	downloadURL := ""
	for _, asset := range release.Assets {
		if asset.Name == component {
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

	if vLatest.GreaterThan(vCurrent) && downloadURL != "" {
		return release.TagName, downloadURL, true, nil
	}

	return release.TagName, downloadURL, false, nil
}

// DownloadAndReplace downloads the binary from downloadURL, writes it to a
// temporary file, and then replaces the target component binary with it.
// If component is "jman", it replaces the currently running executable.
// Otherwise, it looks for the component in the same directory as the jman binary.
func DownloadAndReplace(downloadURL string, component string) error {
	// Resolve the path of the currently running jman executable (follow symlinks).
	jmanPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}
	jmanPath, err = filepath.EvalSymlinks(jmanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	dir := filepath.Dir(jmanPath)
	targetPath := jmanPath
	if component != "jman" {
		targetPath = filepath.Join(dir, component)
	}

	// Default mode to 0755 if the target doesn't exist yet.
	mode := os.FileMode(0755)
	if info, err := os.Stat(targetPath); err == nil {
		mode = info.Mode().Perm()
	}

	// Download the new binary to a temporary file in the same directory as the
	// target binary. Using the same directory avoids cross-device rename issues.
	tmpFile, err := os.CreateTemp(dir, fmt.Sprintf("%s-update-*", component))
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

	progress := newProgressReader(resp.Body, resp.ContentLength)

	if _, err := io.Copy(tmpFile, progress); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write update to temporary file: %w", err)
	}
	progress.finish()
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Set the same permissions as the original binary (ensure executable).
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("failed to set permissions on new binary: %w", err)
	}

	// Replace the old binary. os.Rename is atomic on the same filesystem.
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to replace binary %s (do you have write permission?): %w", component, err)
	}

	return nil
}
