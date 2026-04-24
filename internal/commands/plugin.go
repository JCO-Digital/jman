package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/search"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin [list|install|info|update|remove] <target> <plugin-name>",
	Short: "Plugin actions on target sites.",
	Long:  "List, install, update or remove plugins on target sites. Supports WordPress.org slugs, custom repo URLs, or aliases defined in config (install only).",
	Args:  cobra.MinimumNArgs(2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"list", "install", "info", "update", "remove"}, cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			sites, err := search.SearchSites(toComplete)
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
		if len(args) == 2 {
			// For install and update, suggest plugin names from cache
			if args[0] == "install" || args[0] == "update" || args[0] == "info" || args[0] == "remove" {
				plugins, err := cache.GetCachedPlugins()
				if err != nil {
					return nil, cobra.ShellCompDirectiveError
				}
				var completions []string
				for _, plugin := range plugins {
					completions = append(completions, fmt.Sprintf("%s\t%s", plugin.Name, plugin.Name))
				}
				return completions, cobra.ShellCompDirectiveDefault
			}
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: pluginCommand,
}

func init() {
	rootCmd.AddCommand(pluginCmd)
}

func pluginCommand(cmd *cobra.Command, args []string) error {
	operation := args[0]
	target := args[1]

	pluginName := ""
	if len(args) > 2 {
		pluginName = args[2]
	}

	switch operation {
	case "list":
		if pluginName != "" {
			verb.Printf(verb.Verbose, "Plugin name '%s' will be ignored for 'list' operation.\n", pluginName)
		}
	case "install", "remove", "info":
		if pluginName == "" {
			return fmt.Errorf("plugin name is required for '%s' operation", operation)
		}
		if operation == "install" {
			resolvePluginAlias(&pluginName)
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
				verb.Printf(verb.Normal, "Error listing plugins on %s:\n%v\n", verb.Blue(site.Name), verb.Red(err))
				continue
			}
		case "update":
			updateErr := updatePlugin(site, pluginName)
			if updateErr != nil {
				verb.Printf(verb.Normal, "Error updating plugin on %s: %v\n", verb.Blue(site.Name), verb.Red(updateErr))
				continue
			}
		case "remove":
			err := removePlugin(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error removing plugin from %s: %v\n", verb.Blue(site.Name), verb.Red(err))
				continue
			}
		case "install":
			err := installPlugin(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error installing plugin on %s: %v\n", verb.Blue(site.Name), verb.Red(err))
				continue
			}
		case "info":
			err := pluginInfo(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error fetching plugin info on %s: %v\n", verb.Blue(site.Name), verb.Red(err))
				continue
			}
		}
	}

	return nil
}

func installPlugin(site models.CliSite, pluginName string) error {
	installSource := pluginName

	// Check if pluginName is a local ZIP file
	if strings.HasSuffix(strings.ToLower(pluginName), ".zip") {
		if info, err := os.Stat(pluginName); err == nil && !info.IsDir() {
			if site.SSH != "" {
				// Remote site: upload the file
				remoteTempPath := fmt.Sprintf("/tmp/jman-%d-%s", time.Now().Unix(), filepath.Base(pluginName))
				verb.Printf(verb.Verbose, "Uploading %s to %s:%s...\n", pluginName, site.ServerName, remoteTempPath)

				if err := wpcli.UploadFile(site.SSH, pluginName, remoteTempPath); err != nil {
					return fmt.Errorf("failed to upload plugin: %w", err)
				}

				installSource = remoteTempPath
				// Ensure cleanup on the remote server
				defer func() {
					verb.Printf(verb.Debug, "Cleaning up remote file %s...\n", remoteTempPath)
					wpcli.RunSSH(site.SSH, "rm", remoteTempPath)
				}()
			} else {
				// Local site: use absolute path
				absPath, err := filepath.Abs(pluginName)
				if err == nil {
					installSource = absPath
				}
			}
		}
	}

	verb.Printf(verb.Verbose, "Installing '%s' on %s (%s)...\n", pluginName, verb.Blue(site.Name), verb.Yellow(site.ServerName))
	success, err := wpcli.AddPlugin(site.SSH, site.Path, installSource, true)
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
	plugins, err := wpcli.GetPlugins(site, false)
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
		verb.Printf(verb.Normal, "- %s %s%s %s\n", cache.DisplayPluginName(plugin.Name, true, true), status, plugin.Version, update)
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
		verb.Printf(verb.Normal, "- %s (%s) (%s -> %s)%s\n", cache.GetPluginName(plugin.Name), plugin.Name, plugin.Version, plugin.Update, selected)
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
	plugins, err := wpcli.GetPlugins(site, false)
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
		verb.Printf(verb.Normal, "Failed to remove '%s' from %s. (It might not be installed)\n", verb.Yellow(pluginName), verb.Blue(site.Name))
	}
	return nil
}

func pluginInfo(site models.CliSite, pluginName string) error {
	verb.Printf(verb.Verbose, "Fetching info for '%s' on %s (%s)...\n", verb.Yellow(pluginName), verb.Blue(site.Name), site.ServerName)
	info, err := wpcli.GetPluginInfo(site.SSH, site.Path, pluginName)
	if err != nil {
		return err
	}

	if info == nil {
		verb.Printf(verb.Normal, "Plugin '%s' not found on %s.\n", verb.Yellow(pluginName), verb.Blue(site.Name))
		return nil
	}

	verb.Printf(verb.Normal, "Plugin info for '%s' on %s:\n", verb.Yellow(pluginName), verb.Blue(site.Name))
	verb.Printf(verb.Normal, "- Name: %s\n", info.Name)
	verb.Printf(verb.Normal, "- Slug: %s\n", info.Slug)
	verb.Printf(verb.Normal, "- Version: %s\n", info.Version)
	verb.Printf(verb.Normal, "- Author: %s\n", info.Author)
	return nil
}

var satisRegex = regexp.MustCompile(`(https?):\/\/([^\/]+)\/satispress\/([^\/]+)\/([^\/\s?#]+)`)

func resolvePluginAlias(pluginName *string) {
	if alias, ok := config.Cfg.PluginAliases[*pluginName]; ok {
		verb.Printf(verb.Verbose, "Using alias for '%s': %s\n", *pluginName, alias)
		*pluginName = alias
	}

	if strings.HasSuffix(*pluginName, ".zip") {
		return
	}

	if match := satisRegex.FindStringSubmatch(*pluginName); match != nil {
		protocol := match[1]
		baseURL := match[2]
		slug := match[3]
		version := strings.TrimSuffix(match[4], "/")

		if alias, ok := config.Cfg.PluginAliases[strings.ReplaceAll(baseURL, ".", "_")]; ok {
			*pluginName = fmt.Sprintf("%s://%s/%s/%s/%s-%s.zip", protocol, baseURL, strings.Trim(alias, "/"), slug, slug, version)
			verb.Printf(verb.Verbose, "Resolved Satispress URL to ZIP: %s\n", *pluginName)
		}

	}
}
