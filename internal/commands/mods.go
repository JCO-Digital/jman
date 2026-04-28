package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var modsCmd = &cobra.Command{
	Use:           "mods [enable|disable|allow|disallow] <target>",
	Short:         "Set DISALLOW_FILE_MODS on target sites.",
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"enable", "disable", "allow", "disallow"}, cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		disallow := true
		if len(args) > 1 {
			switch args[0] {
			case "enable", "allow":
				disallow = false
			case "disable", "disallow":
				disallow = true
			default:
				return fmt.Errorf("invalid operation: %s. Use 'enable', 'disable', 'allow' or 'disallow'.", args[0])
			}
		}
		target := args[1]

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
