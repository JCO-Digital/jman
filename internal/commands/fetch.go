package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch latest data from SpinupWP and update local cache.",
	RunE: func(cmd *cobra.Command, args []string) error {
		verbosity.PrintErrorln(verbosity.Normal, "Fetching latest data from SpinupWP...")

		servers, err := cache.RefreshCachedServers()
		if err != nil {
			return fmt.Errorf("error fetching servers: %w", err)
		}
		verbosity.PrintErrorf(verbosity.Verbose, "Successfully fetched and cached %d servers.\n", len(servers))

		sites, err := cache.RefreshCachedSites()
		if err != nil {
			return fmt.Errorf("error fetching sites: %w", err)
		}
		verbosity.Printf(verbosity.Verbose, "Successfully fetched and cached %d sites.\n", len(sites))

		verbosity.PrintErrorln(verbosity.Normal, "Cache update complete.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
