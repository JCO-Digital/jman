package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
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
			verb.Printf(verb.Verbose, "\n=== %s (%s) ===\n", site.Name, site.ServerName)
			res, err := wpcli.RunWP(wpcli.CliOptions{SSH: site.SSH, Path: site.Path}, args[1:]...)

			if res.Output != "" {
				verb.Println(verb.Quiet, res.Output)
			}
			if res.Error != "" {
				verb.PrintErrorln(verb.Verbose, res.Error)
			}
			if err != nil {
				verb.PrintErrorf(verb.Verbose, "Error executing command: %v\n", err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(wpCmd)
}
