package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/hashicorp/go-version"
)

// maxDownloadSize caps how much data we'll read from a release asset, to
// bound memory/disk use if an upstream host is compromised or misbehaves.
const maxDownloadSize = 200 * 1024 * 1024 // 200 MiB

// maxSignatureSize caps how much data we'll read for a detached signature.
const maxSignatureSize = 4 * 1024 // 4 KiB

// allowedDownloadHosts restricts release asset downloads to GitHub's own
// hosts, so a compromised release API response can't redirect us elsewhere.
var allowedDownloadHosts = []string{"github.com", "objects.githubusercontent.com"}

func validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("URL must use https, got %q", u.Scheme)
	}
	for _, host := range allowedDownloadHosts {
		if u.Host == host || strings.HasSuffix(u.Host, "."+host) {
			return nil
		}
	}
	return fmt.Errorf("URL host %q is not an allowed download host", u.Host)
}

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
			verb.Printf(verb.Verbose, "\r  Downloading: %3d%%  (%.1f / %.1f MB)", pct, readMB, totalMB)
		}
	} else {
		verb.Printf(verb.Verbose, "\r  Downloading: %.1f MB", readMB)
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

// GetLatestRelease fetches the latest release from a GitHub repository URL.
func GetLatestRelease(url string) (*Release, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch latest release: received status code %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release data: %w", err)
	}

	return &release, nil
}

// CheckForUpdate checks if a newer version of the CLI is available.
// It returns the latest version string, the download URL, the signature URL, and a boolean indicating if an update is available.
func CheckForUpdate(currentVersion string, component string) (string, string, string, bool, error) {
	release, err := GetLatestRelease(LatestReleaseURL)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to check for updates: %w", err)
	}

	downloadURL := ""
	sigURL := ""
	sigName := component + ".minisig"

	for _, asset := range release.Assets {
		if asset.Name == component {
			downloadURL = asset.BrowserDownloadURL
		} else if asset.Name == sigName {
			sigURL = asset.BrowserDownloadURL
		}
	}

	vCurrent, err := version.NewVersion(currentVersion)
	if err != nil {
		// If current version is not semver, we can't reliably compare.
		return release.TagName, downloadURL, sigURL, false, nil
	}

	vLatest, err := version.NewVersion(release.TagName)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to parse latest version %s: %w", release.TagName, err)
	}

	if vLatest.GreaterThan(vCurrent) && downloadURL != "" {
		return release.TagName, downloadURL, sigURL, true, nil
	}

	return release.TagName, downloadURL, sigURL, false, nil
}

// DownloadAndReplace downloads the binary from downloadURL, writes it to a
// temporary file, and then replaces the target component binary with it.
// If component is "jman", it replaces the currently running executable.
// Otherwise, it looks for the component in the same directory as the jman binary.
func DownloadAndReplace(downloadURL, sigURL, component string) error {
	if err := validateDownloadURL(downloadURL); err != nil {
		return fmt.Errorf("refusing to download update: %w", err)
	}
	if sigURL == "" {
		return fmt.Errorf("refusing to install update: no signature asset found for %s", component)
	}
	if err := validateDownloadURL(sigURL); err != nil {
		return fmt.Errorf("refusing to download update signature: %w", err)
	}

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
	limited := io.LimitReader(progress, maxDownloadSize+1)

	written, err := io.Copy(tmpFile, limited)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write update to temporary file: %w", err)
	}
	if written > maxDownloadSize {
		tmpFile.Close()
		return fmt.Errorf("update download exceeded maximum allowed size of %d bytes", maxDownloadSize)
	}
	progress.finish()

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	// Verify signature. A missing or invalid signature is a hard failure —
	// every release binary is signed in CI, so its absence indicates a
	// compromised or malformed release and must not be installed.
	if _, err := tmpFile.Seek(0, 0); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to seek temporary file: %w", err)
	}
	content, err := io.ReadAll(io.LimitReader(tmpFile, maxDownloadSize+1))
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to read temporary file for verification: %w", err)
	}
	if err := verifySignature(content, sigURL); err != nil {
		tmpFile.Close()
		return fmt.Errorf("signature verification failed: %w", err)
	}
	verb.Printf(verb.Verbose, "  Signature verified successfully\n")

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

func verifySignature(content []byte, sigURL string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sigURL)
	if err != nil {
		return fmt.Errorf("failed to download signature: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download signature: received status code %d", resp.StatusCode)
	}

	sigBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureSize+1))
	if err != nil {
		return fmt.Errorf("failed to read signature: %w", err)
	}
	if len(sigBytes) > maxSignatureSize {
		return fmt.Errorf("signature response exceeded maximum allowed size of %d bytes", maxSignatureSize)
	}

	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigBytes)))
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil {
		return fmt.Errorf("failed to decode public key: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), content, signature) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}
