package commands

import (
	"fmt"
	"os"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for a new version of jman",
	Long:  `Checks the GitHub repository for a newer version of the jman CLI tool.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		currentVersion := config.RunData.Version
		verbosity.Printf(verbosity.Normal, "Current version: %s\n", currentVersion)

		// Use a dummy version if running in dev to see what the latest version is
		checkVersion := currentVersion
		if currentVersion == "dev" {
			checkVersion = "v0.0.0"
		}

		latestVersion, releaseURL, available, err := update.CheckForUpdate(checkVersion)
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}

		if currentVersion == "dev" {
			fmt.Println("You are running a development version of jman.")
			if latestVersion != "" {
				fmt.Printf("The latest stable version is: %s\n", latestVersion)
			}
			return nil
		}

		if !available {
			fmt.Println("You are running the latest version of jman.")
			return nil
		}

		fmt.Printf("\nA new version of jman is available: %s\n", latestVersion)
		fmt.Print("\nWould you like to download and install it? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			return nil
		}

		fmt.Println("\nDownloading new version...")
		if err := update.DownloadAndReplace(releaseURL); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}

		fmt.Printf("Successfully updated jman to %s\n", latestVersion)
		os.Exit(0)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
