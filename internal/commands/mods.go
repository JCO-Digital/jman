package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var modsCmd = &cobra.Command{
	Use:   "mods <target>",
	Short: "Set DISALLOW_FILE_MODS to true on target sites.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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
			verb.Printf(verb.Verbose, "Setting DISALLOW_FILE_MODS on %s...\n", site.Name)
			err := wpcli.SetDisallowFileMods(site.SSH, site.Path, true)
			if err != nil {
				verb.Printf(verb.Normal, "Error setting DISALLOW_FILE_MODS for %s: %v\n", site.Name, err)
			} else {
				verb.Printf(verb.Normal, "Successfully set DISALLOW_FILE_MODS for %s.\n", site.Name)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(modsCmd)
}
