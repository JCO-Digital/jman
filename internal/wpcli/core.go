package wpcli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/JCO-Digital/jman/internal/verbosity"
)

// Check WordPress core for updates and display the current version and available updates if any.
func CheckCore(ssh, path string) (bool, error) {
	res, err := RunWP(ssh, path, true, "core", "check-update")
	if err != nil {
		return false, fmt.Errorf("failed to check core updates: %w (stderr: %s)", err, res.Error)
	}
	if strings.TrimSpace(res.Output) == "Success: WordPress is at the latest version." {
		verbosity.Println(verbosity.Verbose, "WordPress core is up to date.")
		return false, nil
	}

	verbosity.Print(verbosity.Normal, res.Output)
	return true, nil
}

var updateRegex = regexp.MustCompile(`^Updating to version [0-9.-]+ \([^)]+\)...`)

// Update WordPress core to the latest minor version. This will also update the database if necessary.
func UpdateCore(ssh, path string) (bool, error) {
	res, err := RunWP(ssh, path, true, "core", "update", "--minor")
	if err != nil {
		return false, fmt.Errorf("failed to update core: %w (stderr: %s)", err, res.Error)
	}

	// return early if already up to date to avoid printing unnecessary output
	if strings.Contains(res.Output, "Success: WordPress is at the latest") {
		return false, nil
	}

	verbosity.Print(verbosity.Debug, res.Output)

	updateLines := updateRegex.FindString(res.Output)
	if updateLines != "" {
		verbosity.Println(verbosity.Normal, updateLines)
	}
	if !strings.Contains(res.Output, "Success: WordPress updated successfully.") {
		return false, fmt.Errorf("core update did not complete successfully (stderr: %s)", res.Error)
	}

	res, err = RunWP(ssh, path, true, "core", "update-db")
	if err != nil {
		return false, fmt.Errorf("failed to update core database: %w (stderr: %s)", err, res.Error)
	}
	fmt.Print(res.Output)

	return true, nil
}

// CoreVersion returns the current WordPress core version on the target site.
func CoreVersion(ssh, path string) (string, error) {
	res, err := RunWP(ssh, path, true, "core", "version")
	if err != nil {
		return "", fmt.Errorf("failed to show core version: %w (stderr: %s)", err, res.Error)
	}
	return strings.TrimSpace(res.Output), nil
}
