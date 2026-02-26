package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [api|monitor]",
	Short: "Check for a new version of jman or its components",
	Long:  `Checks the GitHub repository for a newer version of the jman CLI tool or its sidecar binaries (api, monitor).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		component := "jman"
		if len(args) > 0 {
			switch args[0] {
			case "api":
				component = "jman-api"
			case "monitor":
				component = "jman-monitor"
			default:
				return fmt.Errorf("unknown update target: %s (expected 'api' or 'monitor')", args[0])
			}
		}

		currentVersion := config.RunData.Version
		verbosity.Printf(verbosity.Normal, "Current jman version: %s\n", currentVersion)

		// Use a dummy version if running in dev to see what the latest version is
		checkVersion := currentVersion
		if currentVersion == "dev" {
			checkVersion = "v0.0.0"
		}

		latestVersion, releaseURL, available, err := update.CheckForUpdate(checkVersion, component)
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}

		if component == "jman" {
			if currentVersion == "dev" {
				fmt.Println("You are running a development version of jman.")
				if latestVersion != "" {
					verbosity.Printf(verbosity.Verbose, "The latest stable version is: %s\n", latestVersion)
				}
				return nil
			}

			if !available {
				verbosity.Println(verbosity.Normal, "You are running the latest version of jman.")
				return nil
			}

			verbosity.Printf(verbosity.Normal, "\nA new version of jman is available: %s\n", latestVersion)
			verbosity.Printf(verbosity.Quiet, "\nWould you like to download and install it? [y/N]: ")

			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				return nil
			}
		} else {
			if releaseURL == "" {
				return fmt.Errorf("could not find download URL for %s in latest release %s", component, latestVersion)
			}

			execPath, _ := os.Executable()
			binDir := filepath.Dir(execPath)
			fmt.Printf("\nLatest version of %s is %s.\n", component, latestVersion)
			fmt.Printf("This will download %s to %s and overwrite any existing version.\n", component, binDir)
			fmt.Print("Proceed? [y/N]: ")

			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				return nil
			}
		}

		fmt.Println("\nDownloading...")
		if err := update.DownloadAndReplace(releaseURL, component); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}

		verbosity.Printf(verbosity.Verbose, "Successfully updated %s to %s\n", component, latestVersion)
		if component == "jman" {
			os.Exit(0)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
