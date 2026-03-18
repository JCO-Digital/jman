package wpcli

import (
	"fmt"
	"strings"
)

// Check WordPress core for updates and display the current version and available updates if any.
func CheckCore(ssh, path string) error {
	res, err := RunWP(ssh, path, true, "core", "version")
	if err != nil {
		return fmt.Errorf("failed to get core version: %w (stderr: %s)", err, res.Error)
	}
	fmt.Printf("Current version: %s", res.Output)

	res, err = RunWP(ssh, path, true, "core", "check-update")
	if err != nil {
		return fmt.Errorf("failed to check core updates: %w (stderr: %s)", err, res.Error)
	}
	fmt.Print(res.Output)
	return nil
}

// Update WordPress core to the latest minor version. This will also update the database if necessary.
func UpdateCore(ssh, path string) error {
	fmt.Println("Updating WordPress core...")
	res, err := RunWP(ssh, path, true, "core", "update", "--minor")
	if err != nil {
		return fmt.Errorf("failed to update core: %w (stderr: %s)", err, res.Error)
	}
	fmt.Print(res.Output)

	fmt.Println("Checking for database updates...")
	res, err = RunWP(ssh, path, true, "core", "update-db")
	if err != nil {
		return fmt.Errorf("failed to update core database: %w (stderr: %s)", err, res.Error)
	}
	fmt.Print(res.Output)

	return nil
}

// ShowCoreVersion displays the current WordPress core version on the target site.
func ShowCoreVersion(ssh, path string) error {
	res, err := RunWP(ssh, path, true, "core", "version")
	if err != nil {
		return fmt.Errorf("failed to show core version: %w (stderr: %s)", err, res.Error)
	}
	fmt.Printf("WordPress version: %s", strings.TrimSpace(res.Output))
	return nil
}
