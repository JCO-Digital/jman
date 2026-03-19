package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var coreCmd = &cobra.Command{
	Use:   "core <target> [check|update|version]",
	Short: "Manage WordPress core.",
	Long:  "Check core version, update core to latest version, or display current core version on target sites.",
	Args:  cobra.MinimumNArgs(2),
	RunE:  coreCommand,
}

func init() {
	rootCmd.AddCommand(coreCmd)
}

func coreCommand(cmd *cobra.Command, args []string) error {
	target := args[0]
	operation := args[1]
	if operation != "check" && operation != "update" && operation != "version" {
		return fmt.Errorf("invalid operation: %s. Use 'check', 'update', or 'version'", operation)
	}

	sites, err := search.PromptSearch(target)
	if err != nil {
		return err
	}

	if len(sites) == 0 {
		fmt.Println("Operation cancelled or no sites matched.")
		return nil
	}

	switch operation {
	case "check":
		updates := 0
		updated := 0
		for _, site := range sites {
			verbosity.Printf(verbosity.Verbose, "Checking WordPress core on %s...\n", site.Name)
			coreUpdates, err := wpcli.CheckCore(site.SSH, site.Path)
			if err != nil {
				verbosity.PrintErrorf(verbosity.Normal, "Error checking core on %s: %v\n", site.Name, err)
				continue
			}
			if len(coreUpdates) > 0 {
				currentVersion, err := wpcli.CoreVersion(site.SSH, site.Path)
				if err != nil {
					verbosity.PrintErrorf(verbosity.Normal, "Error getting current core version on %s: %v\n", site.Name, err)
					continue
				}
				for _, update := range coreUpdates {
					verbosity.Printf(verbosity.Normal, "Update available for %s: %s -> %s (%s)\n", site.Name, currentVersion, update.Version, update.UpdateType)
				}
				updates++
			} else {
				verbosity.Printf(verbosity.Verbose, "WordPress core is up to date on %s.\n", site.Name)
				updated++
			}
		}
		if updated > 0 {
			verbosity.Printf(verbosity.Normal, "%d site(s) are up to date.\n", updated)
		}
		if updates > 0 {
			verbosity.Printf(verbosity.Normal, "%d site(s) have updates available.\n", updates)
		}
	case "update":
		updated := 0
		for _, site := range sites {
			verbosity.Printf(verbosity.Verbose, "Updating WordPress core on %s...\n", site.Name)
			success, err := wpcli.UpdateCore(site.SSH, site.Path)
			if err != nil {
				verbosity.PrintErrorf(verbosity.Normal, "Error updating core on %s: %v\n", site.Name, err)
				continue
			}
			if success {
				verbosity.Printf(verbosity.Normal, "Successfully updated WordPress core on %s.\n", site.Name)
				updated++
			}
		}
		if updated > 0 {
			verbosity.Printf(verbosity.Normal, "Successfully updated core on %d site(s).\n", updated)
		}
	case "version":
		for _, site := range sites {
			verbosity.Printf(verbosity.Verbose, "Showing WordPress core version on %s...\n", site.Name)
			version, err := wpcli.CoreVersion(site.SSH, site.Path)
			if err != nil {
				verbosity.PrintErrorf(verbosity.Normal, "Error showing core version on %s: %v\n", site.Name, err)
				continue
			}
			verbosity.Printf(verbosity.Normal, "WordPress core version on %s: %s\n", site.Name, version)
		}
	}

	return nil
}
