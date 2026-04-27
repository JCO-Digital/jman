package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:           "admin <target> <username> <email>",
	Short:         "Create a new administrator user on target sites.",
	Args:          cobra.ExactArgs(3),
	SilenceErrors: true,
	SilenceUsage:  true,
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
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		username := args[1]
		email := args[2]

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
			password, err := wpcli.AddUser(site.SSH, site.Path, username, email, "administrator")
			if err != nil {
				verb.Printf(verb.Verbose, "Error creating admin user: %v\n", err)
				continue
			}

			if password != "" {
				verb.Printf(verb.Normal, "Successfully created user '%s' with password: %s\n", username, password)
			} else {
				verb.Printf(verb.Normal, "User '%s' may already exist or creation failed without a returned password.\n", username)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(adminCmd)
}
