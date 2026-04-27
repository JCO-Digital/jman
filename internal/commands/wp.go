package commands

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var wpCmd = &cobra.Command{
	Use:   "wp <target> [args...]",
	Short: "Run a wp-cli command on a target site.",
	Args:  cobra.MinimumNArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			sites, err := search.SearchSitesFast(toComplete)
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			var completions []string
			for _, site := range sites {
				completions = append(completions, fmt.Sprintf("%s\t%s", site.Name, site.ServerName))
				if site.Name != site.ServerName && site.ServerName != "" {
					completions = append(completions, fmt.Sprintf("%s\t%s", site.ServerName, site.Name))
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		}

		// WP-CLI completion using local wp-cli
		type wpCommand struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Subcommands []wpCommand `json:"subcommands"`
		}

		cmdDump := exec.Command("wp", "cli", "cmd-dump", "--format=json")
		output, err := cmdDump.Output()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		var dump wpCommand
		if err := json.Unmarshal(output, &dump); err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		current := dump
		// Traverse the command tree based on args[1:]
		for i := 1; i < len(args); i++ {
			found := false
			for _, sub := range current.Subcommands {
				if sub.Name == args[i] {
					current = sub
					found = true
					break
				}
			}
			if !found {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}

		var completions []string
		for _, sub := range current.Subcommands {
			completions = append(completions, fmt.Sprintf("%s\t%s", sub.Name, sub.Description))
		}

		return completions, cobra.ShellCompDirectiveNoFileComp
	},

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
