//go:build !noupdate

/*
 * Remove this command from the external build to avoid including the update logic and its dependencies when building with -tags noupdate.
 */

package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

// confirmPrompt reads a single [Y/n]-style confirmation from stdin. An empty
// response (the user just pressing Enter) defaults to "yes", but a true EOF
// — no input available at all, e.g. stdin closed or redirected from
// /dev/null in a cron job or script — is treated as "no" rather than
// silently defaulting to yes, since nothing was actually confirmed.
func confirmPrompt() (bool, error) {
	var response string
	n, err := fmt.Scanln(&response)
	if n == 0 && errors.Is(err, io.EOF) {
		return false, fmt.Errorf("no input available to confirm the update; aborting (run this command interactively to confirm)")
	}
	return response == "y" || response == "Y" || response == "", nil
}

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

		latestVersion, releaseURL, sigURL, available, err := update.CheckForUpdate(checkVersion, component)
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
			verb.Printf(verb.Quiet, "\n%s? [Y/n]: ", verb.Bold("Would you like to download and install it"))

			confirmed, err := confirmPrompt()
			if err != nil {
				return err
			}
			if !confirmed {
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
			fmt.Printf("%s? [Y/n]: ", verb.Bold("Proceed"))

			confirmed, err := confirmPrompt()
			if err != nil {
				return err
			}
			if !confirmed {
				return nil
			}
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
		targetPath := jmanPath
		if component != "jman" {
			targetPath = filepath.Join(filepath.Dir(jmanPath), component)
		}

		fmt.Printf("\n%s...\n", verb.Cyan("Downloading"))
		if err := update.DownloadAndReplace(releaseURL, sigURL, targetPath, component); err != nil {
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
