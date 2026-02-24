package commands

import (
	"fmt"
	"slices"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/spf13/cobra"
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch latest data from SpinupWP and update local cache.",
	RunE: func(cmd *cobra.Command, args []string) error {
		target := "basic"
		if len(args) > 0 {
			target = args[0]
		}

		if slices.Contains([]string{"servers", "sites", "basic", "all"}, target) {
			verbosity.PrintErrorln(verbosity.Normal, "Fetching latest data from SpinupWP...")
		}

		if slices.Contains([]string{"servers", "basic", "all"}, target) {
			servers, err := cache.RefreshCachedServers()
			if err != nil {
				return fmt.Errorf("error fetching servers: %w", err)
			}
			verbosity.PrintErrorf(verbosity.Verbose, "Successfully fetched and cached %d servers.\n", len(servers))
		}

		if slices.Contains([]string{"sites", "basic", "all"}, target) {
			sites, err := cache.RefreshCachedSites()
			if err != nil {
				return fmt.Errorf("error fetching sites: %w", err)
			}
			verbosity.Printf(verbosity.Verbose, "Successfully fetched and cached %d sites.\n", len(sites))
		}

		if slices.Contains([]string{"plugins", "all"}, target) {
			plugins, err := cache.GetCachedPlugins(true)
			if err != nil {
				return fmt.Errorf("error fetching plugins: %w", err)
			}
			verbosity.Printf(verbosity.Verbose, "Successfully fetched and cached %d plugins.\n", len(plugins))
		}

		verbosity.PrintErrorln(verbosity.Normal, "Cache update complete.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fetchCmd)
}
