package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var wpCmd = &cobra.Command{
	Use:   "wp <target> [args...]",
	Short: "Run a wp-cli command on a target site.",
	Args:  cobra.MinimumNArgs(2),
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
			verbosity.Printf(verbosity.Verbose, "\n=== %s (%s) ===\n", site.Name, site.ServerName)
			res, err := wpcli.RunWP(site.SSH, site.Path, false, args[1:]...)

			if res.Output != "" {
				verbosity.Println(verbosity.Quiet, res.Output)
			}
			if res.Error != "" {
				verbosity.PrintErrorln(verbosity.Verbose, res.Error)
			}
			if err != nil {
				verbosity.PrintErrorf(verbosity.Verbose, "Error executing command: %v\n", err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(wpCmd)
}
