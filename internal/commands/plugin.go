package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin <target> <plugin_slug>",
	Short: "Install a plugin on target sites.",
	Long:  "Install a plugin on target sites. Supports WordPress.org slugs or custom repo URLs.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		pluginName := args[1]

		sites, err := search.PromptSearch(target)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		for _, site := range sites {
			verbosity.Printf(verbosity.Verbose, "Installing '%s' on %s (%s)...\n", pluginName, site.Name, site.ServerName)
			success, err := wpcli.AddPlugin(site.SSH, site.Path, pluginName, true)

			if err != nil {
				verbosity.Printf(verbosity.Verbose, "Error installing plugin on %s: %v\n", site.Name, err)
				continue
			}

			if success {
				verbosity.Printf(verbosity.Verbose, "Successfully installed and activated '%s' on %s.\n", pluginName, site.Name)
			} else {
				verbosity.Printf(verbosity.Verbose, "Failed to install '%s' on %s. (It might already be installed)\n", pluginName, site.Name)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(pluginCmd)
}
