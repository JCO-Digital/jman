package commands

import (
	"fmt"
	"strings"

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
		wpArgs := strings.Join(args[1:], " ")

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
			res, err := wpcli.RunWP(site.SSH, site.Path, wpArgs, false)

			if res.Output != "" {
				fmt.Println(res.Output)
			}
			if res.Error != "" {
				fmt.Println(res.Error)
			}
			if err != nil {
				verbosity.Printf(verbosity.Verbose, "Error executing command: %v\n", err)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(wpCmd)
}
