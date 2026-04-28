package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var coreCmd = &cobra.Command{
	Use:           "core [check|update|version] <target>",
	Short:         "Manage WordPress core.",
	Long:          "Check core version, update core to latest version, or display current core version on target sites.",
	Args:          cobra.MinimumNArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"check", "update", "version"}, cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: coreCommand,
}

func init() {
	rootCmd.AddCommand(coreCmd)
}

func coreCommand(cmd *cobra.Command, args []string) error {
	operation := args[0]
	target := args[1]
	if operation != "check" && operation != "update" && operation != "version" {
		return fmt.Errorf("invalid operation: %s. Use 'check', 'update', or 'version'", operation)
	}

	sites, err := search.PromptSearch(target)
	if err != nil {
		return err
	}

	if len(sites) == 0 {
		verb.Println(verb.Normal, "Operation cancelled or no sites matched.")
		return nil
	}

	switch operation {
	case "check":
		updates := 0
		updated := 0
		for _, site := range sites {
			verb.Printf(verb.Verbose, "Checking WordPress core on %s...\n", site.Name)
			coreUpdates, err := wpcli.CheckCore(site.SSH, site.Path)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Error checking core on %s: %v\n", site.Name, err)
				continue
			}
			if len(coreUpdates) > 0 {
				currentVersion, err := wpcli.CoreVersion(site.SSH, site.Path)
				if err != nil {
					verb.PrintErrorf(verb.Normal, "Error getting current core version on %s: %v\n", site.Name, err)
					continue
				}
				for _, update := range coreUpdates {
					verb.Printf(verb.Normal, "Update available for %s: %s -> %s (%s)\n",
						verb.Blue(site.Name),
						verb.Yellow(currentVersion),
						verb.Green(update.Version),
						update.UpdateType,
					)
				}
				updates++
			} else {
				verb.Printf(verb.Verbose, "WordPress core is up to date on %s.\n", site.Name)
				updated++
			}
		}
		if updated > 0 {
			verb.Printf(verb.Normal, "%d site(s) are up to date.\n", updated)
		}
		if updates > 0 {
			verb.Printf(verb.Normal, "%d site(s) have updates available.\n", updates)
		}
	case "update":
		updated := 0
		for _, site := range sites {
			verb.Printf(verb.Verbose, "Updating WordPress core on %s...\n", site.Name)
			result, err := wpcli.UpdateCore(site.SSH, site.Path)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Error updating core on %s: %v\n", site.Name, err)
				continue
			}
			if result.Success {
				verb.Printf(verb.Normal, "Successfully updated WordPress core on %s to %s (%s).\n", verb.Blue(site.Name), verb.Green(result.Version), verb.Cyan(result.Language))
				updated++
			}
		}
		if updated > 0 {
			verb.Printf(verb.Normal, "Successfully updated core on %d site(s).\n", updated)
		}
	case "version":
		for _, site := range sites {
			verb.Printf(verb.Verbose, "Showing WordPress core version on %s...\n", site.Name)
			version, err := wpcli.CoreVersion(site.SSH, site.Path)
			if err != nil {
				verb.PrintErrorf(verb.Normal, "Error showing core version on %s: %v\n", site.Name, err)
				continue
			}
			verb.Printf(verb.Normal, "WordPress core version on %s: %s\n", verb.Blue(site.Name), verb.Yellow(version))
		}
	}

	return nil
}
