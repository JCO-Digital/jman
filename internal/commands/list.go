package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list <target>",
	Short: "List cached data from SpinupWP.",
	Long:  "List cached data from SpinupWP. Valid targets are: servers, sites, or all.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := strings.ToLower(args[0])

		if target != "servers" && target != "sites" && target != "all" {
			return fmt.Errorf("invalid target '%s'. Specify: servers, sites, or all", target)
		}

		if target == "all" || target == "servers" {
			servers, err := cache.GetCachedServers()
			if err != nil {
				verb.Printf(verb.Verbose, "Error fetching cached servers: %v\n", err)
			} else {
				verb.Printf(verb.Verbose, "\nCached servers: %d\n", len(servers))
				for _, server := range servers {
					fmt.Println(server.Name)
				}
			}
		}

		if target == "all" || target == "sites" {
			sites, err := cache.GetCachedSites()
			if err != nil {
				verb.Printf(verb.Verbose, "Error fetching cached sites: %v\n", err)
			} else {
				verb.Printf(verb.Verbose, "\nCached sites: %d\n", len(sites))
				for _, site := range sites {
					fmt.Println(site.Domain)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
