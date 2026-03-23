package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var modsCmd = &cobra.Command{
	Use:   "mods <target> [enable|disable|allow|disallow]",
	Short: "Set DISALLOW_FILE_MODS to true on target sites.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		disallow := true
		if len(args) > 1 {
			switch args[1] {
			case "enable", "allow":
				disallow = false
			case "disable", "disallow":
				disallow = true
			default:
				return fmt.Errorf("invalid argument: %s. Use 'enable' or 'disable'.", args[1])
			}
		}

		sites, err := search.PromptSearch(target)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		enableText := "enable"
		if disallow {
			enableText = "disable"
		}

		for _, site := range sites {
			verb.Printf(verb.Verbose, "Setting DISALLOW_FILE_MODS on %s...\n", site.Name)
			err := wpcli.SetDisallowFileMods(site.SSH, site.Path, disallow)
			if err != nil {
				verb.Printf(verb.Normal, "Error setting DISALLOW_FILE_MODS for %s: %v\n", site.Name, err)
			} else {
				verb.Printf(verb.Normal, "Successfully set file mods to %s for %s.\n", enableText, site.Name)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(modsCmd)
}
