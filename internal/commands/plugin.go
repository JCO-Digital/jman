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
	Short: "Plugin actions on target sites.",
	Long:  "List, install, update or remove plugins on target sites. Supports WordPress.org slugs or custom repo URLs.",
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

	switch operation {
	case "list":
		if pluginName != "" {
			verbosity.Printf(verbosity.Verbose, "Plugin name '%s' will be ignored for 'list' operation.\n", pluginName)
		}
	case "install", "remove":
		if pluginName == "" {
			return fmt.Errorf("plugin name is required for '%s' operation", operation)
		}
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
			err := removePlugin(site, pluginName)
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

	plugins, err := getPluginUpdates(site)
	if err != nil {
		return err
	}

	if len(plugins) == 0 {
		verbosity.Printf(verbosity.Normal, "No plugins to update on %s.\n", site.Name)
		return nil
	}
	verbosity.Printf(verbosity.Normal, "Found %d plugins with updates available:\n", len(plugins))

	var toUpdate []string
	for _, plugin := range plugins {
		selected := ""
		if pluginName != "all" && plugin.Name != pluginName {
			selected = " (skipped)"
		} else {
			toUpdate = append(toUpdate, plugin.Name)
		}
		verbosity.Printf(verbosity.Normal, "- %s (%s -> %s)%s\n", plugin.Name, plugin.Version, plugin.Update, selected)
	}
	if len(toUpdate) == 0 {
		verbosity.Printf(verbosity.Normal, "No matching plugins to update on %s.\n", site.Name)
		return nil
	}
	verbosity.Printf(verbosity.Normal, "Updating %d plugins on %s...\n", len(toUpdate), site.Name)

	updated, err := wpcli.UpdatePlugin(site.SSH, site.Path, toUpdate)

	verbosity.Printf(verbosity.Verbose, "Updated %d plugins on %s.\n", updated, site.Name)

	if err != nil {
		verbosity.Printf(verbosity.Normal, "Error updating plugins\n")
	}

	return nil
}

func getPluginUpdates(site models.CliSite) ([]models.WPPlugin, error) {
	var updateList []models.WPPlugin
	plugins, err := wpcli.GetPlugins(site)
	if err != nil {
		return updateList, fmt.Errorf("failed to get plugins: %w", err)
	}
	for _, plugin := range plugins {
		if plugin.Update != "" {
			updateList = append(updateList, plugin)
		}
	}
	return updateList, nil
}

func removePlugin(site models.CliSite, pluginName string) error {
	verbosity.Printf(verbosity.Verbose, "Removing '%s' from %s (%s)...\n", pluginName, site.Name, site.ServerName)
	success, err := wpcli.RemovePlugin(site.SSH, site.Path, pluginName)
	if err != nil {
		return fmt.Errorf("failed to remove plugin: %w", err)
	}

	if success {
		verbosity.Printf(verbosity.Normal, "Successfully removed '%s' from %s.\n", pluginName, site.Name)
	} else {
		verbosity.Printf(verbosity.Normal, "Failed to remove '%s' from %s. (It might not be installed)\n", pluginName, site.Name)
	}
	return nil
}
