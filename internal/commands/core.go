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

	for _, site := range sites {
		switch operation {
		case "check":
			err := wpcli.CheckCore(site.SSH, site.Path)
			if err != nil {
				verbosity.PrintErrorf(verbosity.Normal, "Error checking core on %s: %v\n", site.Name, err)
				continue
			}
		case "update":
			updateErr := wpcli.UpdateCore(site.SSH, site.Path)
			if updateErr != nil {
				verbosity.PrintErrorf(verbosity.Normal, "Error updating core on %s: %v\n", site.Name, updateErr)
				continue
			}
		case "version":
			err := wpcli.ShowCoreVersion(site.SSH, site.Path)
			if err != nil {
				verbosity.PrintErrorf(verbosity.Normal, "Error showing core version on %s: %v\n", site.Name, err)
				continue
			}
		}
	}

	return nil
}
