package commands

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin <target> [list|install|update|remove] <plugin-name>",
	Short: "Plugin actions on target sites.",
	Long:  "List, install, update or remove plugins on target sites. Supports WordPress.org slugs, custom repo URLs, or aliases defined in config (install only).",
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
			verb.Printf(verb.Verbose, "Plugin name '%s' will be ignored for 'list' operation.\n", pluginName)
		}
	case "install", "remove":
		if pluginName == "" {
			return fmt.Errorf("plugin name is required for '%s' operation", operation)
		}
		if operation == "install" {
			if alias, ok := config.Cfg.PluginAliases[pluginName]; ok {
				verb.Printf(verb.Verbose, "Using alias for '%s': %s\n", pluginName, alias)
				pluginName = alias
			}
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
				verb.Printf(verb.Normal, "Error listing plugins on %s: %v\n", verb.Blue(site.Name), err)
				continue
			}
		case "update":
			updateErr := updatePlugin(site, pluginName)
			if updateErr != nil {
				verb.Printf(verb.Normal, "Error updating plugin on %s: %v\n", verb.Blue(site.Name), updateErr)
				continue
			}
		case "remove":
			err := removePlugin(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error removing plugin from %s: %v\n", verb.Blue(site.Name), err)
				continue
			}
		case "install":
			err := installPlugin(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error installing plugin on %s: %v\n", verb.Blue(site.Name), err)
				continue
			}
		}
	}

	return nil
}

func installPlugin(site models.CliSite, pluginName string) error {
	verb.Printf(verb.Verbose, "Installing '%s' on %s (%s)...\n", pluginName, verb.Blue(site.Name), site.ServerName)
	success, err := wpcli.AddPlugin(site.SSH, site.Path, pluginName, true)
	if err != nil {
		return err
	}

	if success {
		verb.Printf(verb.Normal, "Successfully installed and activated '%s' on %s.\n", verb.Yellow(pluginName), verb.Blue(site.Name))
	} else {
		verb.Printf(verb.Normal, "Failed to install '%s' on %s. (It might already be installed)\n", verb.Yellow(pluginName), verb.Blue(site.Name))
	}
	return nil
}

func listPlugins(site models.CliSite) error {
	verb.Printf(verb.Verbose, "Listing plugins on %s (%s)...\n", verb.Blue(site.Name), site.ServerName)
	plugins, err := wpcli.GetPlugins(site)
	if err != nil {
		return err
	}

	if len(plugins) == 0 {
		verb.Printf(verb.Normal, "No plugins found on %s.\n", verb.Blue(site.Name))
		return nil
	}

	verb.Printf(verb.Normal, "Plugins on %s:\n", verb.Blue(site.Name))
	for _, plugin := range plugins {
		status := ""
		if plugin.Status != "active" {
			status += fmt.Sprintf("(%s) ", plugin.Status)
		}
		update := ""
		if plugin.Update != "" {
			update += fmt.Sprintf("-> %s available", plugin.Update)
		}
		verb.Printf(verb.Normal, "- %s %s%s %s\n", plugin.Name, status, plugin.Version, update)
	}
	return nil
}

func updatePlugin(site models.CliSite, pluginName string) error {
	if pluginName == "" {
		pluginName = "all"
	}
	verb.Printf(verb.Verbose, "Updating '%s' on %s (%s)...\n", verb.Yellow(pluginName), verb.Blue(site.Name), site.ServerName)

	plugins, err := getPluginUpdates(site)
	if err != nil {
		return err
	}

	if len(plugins) == 0 {
		verb.Printf(verb.Normal, "No plugins to update on %s.\n", verb.Blue(site.Name))
		return nil
	}
	verb.Printf(verb.Normal, "Found %d plugins with updates available:\n", len(plugins))

	var toUpdate []string
	for _, plugin := range plugins {
		selected := ""
		if pluginName != "all" && plugin.Name != pluginName {
			selected = " (skipped)"
		} else {
			toUpdate = append(toUpdate, plugin.Name)
		}
		verb.Printf(verb.Normal, "- %s (%s -> %s)%s\n", plugin.Name, plugin.Version, plugin.Update, selected)
	}
	if len(toUpdate) == 0 {
		verb.Printf(verb.Normal, "No matching plugins to update on %s.\n", verb.Blue(site.Name))
		return nil
	}
	verb.Printf(verb.Normal, "Updating %d plugins on %s...\n", len(toUpdate), verb.Blue(site.Name))

	updated, err := wpcli.UpdatePlugin(site.SSH, site.Path, toUpdate)

	verb.Printf(verb.Verbose, "Updated %d plugins on %s.\n", updated, verb.Blue(site.Name))

	if err != nil {
		verb.Printf(verb.Normal, "Error updating plugins\n")
	}

	return nil
}

func getPluginUpdates(site models.CliSite) ([]models.WPPlugin, error) {
	var updateList []models.WPPlugin
	plugins, err := wpcli.GetPlugins(site)
	if err != nil {
		return updateList, err
	}
	for _, plugin := range plugins {
		if plugin.Update != "" {
			updateList = append(updateList, plugin)
		}
	}
	return updateList, nil
}

func removePlugin(site models.CliSite, pluginName string) error {
	verb.Printf(verb.Verbose, "Removing '%s' from %s (%s)...\n", verb.Yellow(pluginName), verb.Blue(site.Name), site.ServerName)
	success, err := wpcli.RemovePlugin(site.SSH, site.Path, pluginName)
	if err != nil {
		if strings.Contains(err.Error(), "plugin could not be found") {
			return fmt.Errorf("plugin not found")
		}
		return err
	}

	if success {
		verb.Printf(verb.Normal, "Successfully removed '%s' from %s.\n", verb.Yellow(pluginName), verb.Blue(site.Name))
	} else {
		verb.Printf(verb.Normal, "Failed to remove '%s' from %s. (It might not be installed)\n", verb.Yellow(pluginName), verb.Blue(verb.Blue(site.Name)))
	}
	return nil
}
