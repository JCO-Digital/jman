package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verb"
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

		currentVersion := config.AppVersion
		verb.Printf(verb.Normal, "Current jman version: %s\n", verb.Blue(currentVersion))

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
				fmt.Println(verb.Yellow("You are running a development version of jman."))
				if latestVersion != "" {
					verb.Printf(verb.Verbose, "The latest stable version is: %s\n", verb.Blue(latestVersion))
				}
				return nil
			}

			if !available {
				verb.Println(verb.Normal, verb.Green("You are running the latest version of jman."))
				return nil
			}

			verb.Printf(verb.Normal, "\nA new version of jman is available: %s\n", verb.Green(latestVersion))
			verb.Printf(verb.Quiet, "\n%s? [y/N]: ", verb.Bold("Would you like to download and install it"))

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
			fmt.Printf("\nLatest version of %s is %s.\n", verb.Blue(component), verb.Green(latestVersion))
			fmt.Printf("This will download %s to %s and overwrite any existing version.\n", verb.Blue(component), verb.Gray(binDir))
			fmt.Printf("%s? [y/N]: ", verb.Bold("Proceed"))

			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				return nil
			}
		}

		fmt.Printf("\n%s...\n", verb.Cyan("Downloading"))
		if err := update.DownloadAndReplace(releaseURL, component); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}

		verb.Printf(verb.Verbose, "Successfully updated %s to %s\n", verb.Blue(component), verb.Green(latestVersion))
		if component == "jman" {
			os.Exit(0)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
