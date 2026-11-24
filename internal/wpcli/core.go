package wpcli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// CoreUpdate represents a WordPress core update notification from wp-cli.
type CoreUpdate struct {
	Version    string `json:"version"`
	UpdateType string `json:"update_type"`
	PackageURL string `json:"package_url"`
}

// Check WordPress core for updates and return the available updates if any.
func CheckCore(site models.CliSite) ([]CoreUpdate, error) {
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path}, "core", "check-update", "--format=json")
	if err != nil {
		return nil, fmt.Errorf("failed to check core updates: %w (stderr: %s)", err, res.Error)
	}

	return ParseCoreUpdates(res.Output)
}

// ParseCoreUpdates parses `wp core check-update --format=json` output.
func ParseCoreUpdates(output string) ([]CoreUpdate, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "[]" {
		return nil, nil
	}

	var updates []CoreUpdate
	if err := json.Unmarshal([]byte(output), &updates); err != nil {
		return nil, fmt.Errorf("failed to parse core update JSON: %w", err)
	}

	return updates, nil
}

// SplitCoreUpdates returns the first minor and first major update version
// in updates, or "" for either kind if there is none.
func SplitCoreUpdates(updates []CoreUpdate) (minorUpdate, majorUpdate string) {
	for _, u := range updates {
		switch u.UpdateType {
		case "minor":
			if minorUpdate == "" {
				minorUpdate = u.Version
			}
		case "major":
			if majorUpdate == "" {
				majorUpdate = u.Version
			}
		}
	}
	return minorUpdate, majorUpdate
}

var updateRegex = regexp.MustCompile(`(?m)^Updating to version ([0-9.-]+) \(([^)]+)\)...`)

type CoreUpdateResult struct {
	Success  bool
	Version  string
	Language string
}

// UpdateCore updates WordPress core. If major is true, it updates to the latest version (major/minor), otherwise it only updates to the latest minor version. It returns the new version and language if an update was performed.
// rec, if not nil, gets the update and database update calls recorded.
func UpdateCore(site models.CliSite, major bool, rec *[]models.WPCLICall) (CoreUpdateResult, error) {
	result := CoreUpdateResult{
		Success:  false,
		Version:  "unknown",
		Language: "",
	}

	args := []string{"core", "update"}
	if major {
		// wp-cli core update defaults to major/latest if no flag is provided, but we can be explicit if we want.
		// Actually wp-cli documentation says: "If no version is specified, it updates to the latest version."
		// --minor only updates to the latest minor version.
	} else {
		args = append(args, "--minor")
	}

	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, Timeout: WriteTimeout, Record: rec}, args...)
	if err != nil {
		return result, fmt.Errorf("failed to update core: %s", shortError(res.Error, err))
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
		if msg := Messages(res.Error); msg != "" {
			return result, fmt.Errorf("core update did not complete successfully: %s", msg)
		}
		return result, fmt.Errorf("core update did not complete successfully")
	}
	result.Success = true

	res, err = RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, Timeout: WriteTimeout, Record: rec}, "core", "update-db")
	if err != nil {
		return result, fmt.Errorf("failed to update core database: %s", shortError(res.Error, err))
	}
	verb.Print(verb.Verbose, res.Output)

	return result, nil
}

// CoreVersion returns the current WordPress core version on the target site.
func CoreVersion(site models.CliSite) (string, error) {
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path}, "core", "version")
	if err != nil {
		return "", fmt.Errorf("failed to show core version: %w (stderr: %s)", err, res.Error)
	}
	return strings.TrimSpace(res.Output), nil
}

// CLIPHPVersion returns the version of the PHP that runs wp-cli for the
// site, which updates run on. It can differ from the site's own PHP-FPM
// version: wp-cli uses whatever `php` the site user's shell finds.
func CLIPHPVersion(site models.CliSite) (string, error) {
	res, err := RunWP(CliOptions{SSH: site.SSH, Path: site.Path}, "cli", "info", "--format=json")
	if err != nil {
		return "", fmt.Errorf("failed to get wp-cli info: %s", shortError(res.Error, err))
	}
	return parseCLIInfoPHPVersion(res.Output)
}

func parseCLIInfoPHPVersion(output string) (string, error) {
	idx := strings.Index(output, "{")
	if idx == -1 {
		return "", fmt.Errorf("no JSON object in wp-cli info output")
	}
	var info struct {
		PHPVersion string `json:"php_version"`
	}
	if err := json.NewDecoder(strings.NewReader(output[idx:])).Decode(&info); err != nil {
		return "", fmt.Errorf("failed to parse wp-cli info: %w", err)
	}
	return info.PHPVersion, nil
}
