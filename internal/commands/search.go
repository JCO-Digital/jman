package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for a specific term across sites.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]

		sites, err := search.SearchSites(query)
		if err != nil {
			return fmt.Errorf("error searching sites: %w", err)
		}

		if len(sites) == 0 {
			verbosity.Printf(verbosity.Verbose, "No sites found matching '%s'.\n", query)
			return nil
		}

		verbosity.Printf(verbosity.Verbose, "Found %d sites matching '%s':\n", len(sites), query)
		for _, site := range sites {
			verbosity.Printf(verbosity.Verbose, "- %s (Server: %s)\n", site.Name, site.ServerName)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
