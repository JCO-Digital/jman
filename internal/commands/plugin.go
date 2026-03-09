package commands

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin <target> [list|install|update|remove] <plugin-name>",
	Short: "Install a plugin on target sites.",
	Long:  "Install a plugin on target sites. Supports WordPress.org slugs or custom repo URLs.",
	Args:  cobra.MinimumNArgs(2),
	RunE:  pluginCommand,
}

func init() {
	rootCmd.AddCommand(pluginCmd)
}

func pluginCommand(cmd *cobra.Command, args []string) error {
	target := args[0]
	operation := args[1]
	if operation != "list" && operation != "install" && operation != "update" && operation != "remove" {
		return fmt.Errorf("invalid operation: %s. Use 'list', 'install', 'update', or 'remove'", operation)
	}

	pluginName := ""
	if len(args) > 2 {
		pluginName = args[2]
	}

	sites, err := search.PromptSearch(target)
	if err != nil {
		return err
	}

	if len(sites) == 0 {
		fmt.Println("Operation cancelled or no sites matched.")
		return nil
	}

	for _, site := range sites {
		switch operation {
		case "list":
			err := listPlugins(site)
			if err != nil {
				verbosity.Printf(verbosity.Verbose, "Error listing plugins on %s: %v\n", site.Name, err)
				continue
			}
		case "update":
			updateErr := updatePlugin(site, pluginName)
			if updateErr != nil {
				verbosity.Printf(verbosity.Verbose, "Error updating plugin on %s: %v\n", site.Name, updateErr)
				continue
			}
		case "remove":
			removePlugin(site, pluginName)
			if err != nil {
				verbosity.Printf(verbosity.Verbose, "Error removing plugin from %s: %v\n", site.Name, err)
				continue
			}
		case "install":
			verbosity.Printf(verbosity.Verbose, "Installing '%s' on %s (%s)...\n", pluginName, site.Name, site.ServerName)
			success, err := wpcli.AddPlugin(site.SSH, site.Path, pluginName, true)

			if err != nil {
				verbosity.Printf(verbosity.Verbose, "Error installing plugin on %s: %v\n", site.Name, err)
				continue
			}

			if success {
				verbosity.Printf(verbosity.Normal, "Successfully installed and activated '%s' on %s.\n", pluginName, site.Name)
			} else {
				verbosity.Printf(verbosity.Normal, "Failed to install '%s' on %s. (It might already be installed)\n", pluginName, site.Name)
			}
		}
	}

	return nil
}

func listPlugins(site models.CliSite) error {
	verbosity.Printf(verbosity.Verbose, "Listing plugins on %s (%s)...\n", site.Name, site.ServerName)
	plugins, err := wpcli.GetPlugins(site)
	if err != nil {
		return fmt.Errorf("failed to get plugins: %w", err)
	}

	if len(plugins) == 0 {
		verbosity.Printf(verbosity.Normal, "No plugins found on %s.\n", site.Name)
		return nil
	}

	verbosity.Printf(verbosity.Normal, "Plugins on %s:\n", site.Name)
	for _, plugin := range plugins {
		status := ""
		if plugin.Status != "active" {
			status += fmt.Sprintf("(%s) ", plugin.Status)
		}
		update := ""
		if plugin.Update != "" {
			update += fmt.Sprintf("-> %s available", plugin.Update)
		}
		verbosity.Printf(verbosity.Normal, "- %s %s%s %s\n", plugin.Name, status, plugin.Version, update)
	}
	//verbosity.Printf(verbosity.Normal, "Plugins on %s:\n%s\n", site.Name, plugins)
	return nil
}

func updatePlugin(site models.CliSite, pluginName string) error {
	if pluginName == "" {
		pluginName = "all"
	}
	verbosity.Printf(verbosity.Verbose, "Updating '%s' on %s (%s)...\n", pluginName, site.Name, site.ServerName)
	plugins, err := wpcli.GetPlugins(site)
	if err != nil {
		return fmt.Errorf("failed to get plugins: %w", err)
	}

	var updated = 0
	for _, plugin := range plugins {
		if pluginName == "all" || plugin.Name == pluginName {
			if plugin.Update != "" {
				verbosity.Printf(verbosity.Normal, "Updating plugin '%s' from %s to %s on %s...\n", plugin.Name, plugin.Version, plugin.Update, site.Name)
				err := wpcli.UpdatePlugin(site.SSH, site.Path, plugin.Name)

				if err != nil {
					verbosity.Printf(verbosity.Normal, "Error updating plugin '%s' on %s: %v\n", plugin.Name, site.Name, err)
				} else {
					updated++
				}
			}
		}
	}
	if updated == 0 {
		verbosity.Printf(verbosity.Normal, "No plugins to update on %s.\n", site.Name)
	}

	return nil
}

func removePlugin(site models.CliSite, pluginName string) error {
	verbosity.Printf(verbosity.Verbose, "Removing '%s' from %s (%s)...\n", pluginName, site.Name, site.ServerName)
	return nil
}
