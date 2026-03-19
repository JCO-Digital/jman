package wpcli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/JCO-Digital/jman/internal/verb"
)

// CoreUpdate represents a WordPress core update notification from wp-cli.
type CoreUpdate struct {
	Version    string `json:"version"`
	UpdateType string `json:"update_type"`
	PackageURL string `json:"package_url"`
}

// Check WordPress core for updates and return the available updates if any.
func CheckCore(ssh, path string) ([]CoreUpdate, error) {
	res, err := RunWP(ssh, path, true, "core", "check-update", "--format=json")
	if err != nil {
		return nil, fmt.Errorf("failed to check core updates: %w (stderr: %s)", err, res.Error)
	}

	output := strings.TrimSpace(res.Output)
	if output == "" || output == "[]" {
		return nil, nil
	}

	var updates []CoreUpdate
	if err := json.Unmarshal([]byte(output), &updates); err != nil {
		return nil, fmt.Errorf("failed to parse core update JSON: %w", err)
	}

	return updates, nil
}

var updateRegex = regexp.MustCompile(`(?m)^Updating to version ([0-9.-]+) \(([^)]+)\)...`)

type CoreUpdateResult struct {
	Success  bool
	Version  string
	Language string
}

// UpdateCore updates WordPress core to the latest minor version. It returns the new version and language if an update was performed.
func UpdateCore(ssh, path string) (CoreUpdateResult, error) {
	result := CoreUpdateResult{
		Success:  false,
		Version:  "unknown",
		Language: "",
	}

	res, err := RunWP(ssh, path, true, "core", "update", "--minor")
	if err != nil {
		return result, fmt.Errorf("failed to update core: %w (stderr: %s)", err, res.Error)
	}

	// return early if already up to date to avoid printing unnecessary output
	if strings.Contains(res.Output, "Success: WordPress is at the latest") {
		return result, nil
	}

	verb.Print(verb.Debug, res.Output)

	matches := updateRegex.FindStringSubmatch(res.Output)
	if len(matches) == 3 {
		result.Version = matches[1]
		result.Language = matches[2]
	}

	if !strings.Contains(res.Output, "Success: WordPress updated successfully.") {
		return result, fmt.Errorf("core update did not complete successfully (stderr: %s)", res.Error)
	}
	result.Success = true

	res, err = RunWP(ssh, path, true, "core", "update-db")
	if err != nil {
		return result, fmt.Errorf("failed to update core database: %w (stderr: %s)", err, res.Error)
	}
	verb.Print(verb.Verbose, res.Output)

	return result, nil
}

// CoreVersion returns the current WordPress core version on the target site.
func CoreVersion(ssh, path string) (string, error) {
	res, err := RunWP(ssh, path, true, "core", "version")
	if err != nil {
		return "", fmt.Errorf("failed to show core version: %w (stderr: %s)", err, res.Error)
	}
	return strings.TrimSpace(res.Output), nil
}
