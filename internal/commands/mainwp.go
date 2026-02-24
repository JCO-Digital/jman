package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/api/mainwp"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var mainWPCmd = &cobra.Command{
	Use:   "mainwp <target>",
	Short: "Install and configure MainWP on sites.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.Cfg.TokenMainWP == "" {
			return fmt.Errorf("MainWP token not found in configuration")
		}

		target := args[0]
		sites, err := search.PromptSearch(target)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		for _, site := range sites {
			verbosity.Printf(verbosity.Verbose, "\n=== %s (%s) ===\n", site.Name, site.ServerName)

			active := wpcli.IsActiveMainwp(site.SSH, site.Path)
			if active {
				verbosity.Printf(verbosity.Verbose, "MainWP is already active for %s\n", site.Name)
				continue
			}

			verbosity.Printf(verbosity.Verbose, "Installing MainWP for %s\n", site.Name)
			var password string

			fmt.Println("Installing MainWP user...")
			pwd, err := wpcli.AddUser(site.SSH, site.Path, "mainwp", "mainwp@jco.fi", "administrator")
			if err != nil || pwd == "" {
				verbosity.Printf(verbosity.Verbose, "MainWP user already exists for %s, resetting password.\n", site.Name)
				pwd, err = wpcli.ResetUserPassword(site.SSH, site.Path, "mainwp")
				if err != nil {
					verbosity.Printf(verbosity.Verbose, "Failed to reset password for %s: %v\n", site.Name, err)
					continue
				}
				password = strings.TrimSpace(pwd)
			} else {
				password = strings.TrimSpace(pwd)
			}

			fmt.Println("Installing MainWP Child Plugin...")
			success, err := wpcli.AddPlugin(site.SSH, site.Path, "mainwp-child", true)
			if err != nil || !success {
				verbosity.Printf(verbosity.Verbose, "MainWP Child Plugin failed to install for %s: %v\n", site.Name, err)
				continue
			}

			fmt.Println("Adding site to MainWP...")
			siteURL := fmt.Sprintf("https://%s", site.Name)
			if err := mainwp.AddSite(siteURL, "mainwp", password); err != nil {
				verbosity.Printf(verbosity.Verbose, "Error adding site to MainWP for %s: %v\n", site.Name, err)
				continue
			}

			verbosity.Printf(verbosity.Verbose, "Successfully installed and configured MainWP for %s\n", site.Name)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mainWPCmd)
}
