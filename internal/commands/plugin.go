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
	Use:   "plugin",
	Short: "Plugin actions on target sites.",
	Long:  "List, install, update or remove plugins on target sites. Supports WordPress.org slugs, custom repo URLs, or aliases defined in config.",
}

var listPluginCmd = &cobra.Command{
	Use:           "list <target>",
	Short:         "List plugins on target sites.",
	Args:          cobra.ExactArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		sites, err := search.PromptSearch(target)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		for _, site := range sites {
			err := listPlugins(site)
			if err != nil {
				verb.Printf(verb.Normal, "Error listing plugins on %s:\n%v\n", verb.Blue(site.Name), verb.Red(err))
			}
		}
		return nil
	},
}

var installPluginCmd = &cobra.Command{
	Use:           "install <target> <plugin-name>",
	Short:         "Install a plugin on target sites.",
	Long:          "Install a plugin on target sites. Supports WordPress.org slugs, custom repo URLs, local ZIP files, or aliases defined in config.",
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		if len(args) == 1 {
			return getPluginCompletions(toComplete, true)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		pluginName := args[1]
		resolvePluginAlias(&pluginName)

		sites, err := search.PromptSearch(target)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		for _, site := range sites {
			err := installPlugin(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error installing plugin on %s: %v\n", verb.Blue(site.Name), verb.Red(err))
			}
		}
		return nil
	},
}

var updatePluginCmd = &cobra.Command{
	Use:           "update <target> [plugin-name]",
	Short:         "Update plugins on target sites.",
	Long:          "Update a specific plugin or all plugins if no plugin name is provided.",
	Args:          cobra.RangeArgs(1, 2),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		if len(args) == 1 {
			return getPluginCompletions(toComplete, false)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		pluginName := ""
		if len(args) > 1 {
			pluginName = args[1]
		}

		sites, err := search.PromptSearch(target)
		if err != nil {
			return err
		}

		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		type siteReport struct {
			siteName string
			updated  []wpcli.UpdateResult
			failed   []string
		}
		var reports []siteReport

		for _, site := range sites {
			updated, failed, updateErr := updatePlugin(site, pluginName)
			if updateErr != nil {
				verb.Printf(verb.Normal, "Error updating plugin on %s: %v\n", verb.Blue(site.Name), verb.Red(updateErr))
			}
			if len(updated) > 0 || len(failed) > 0 {
				reports = append(reports, siteReport{site.Name, updated, failed})
			}
		}

		if len(sites) > 1 && len(reports) > 0 {
			verb.Printf(verb.Normal, "\n%s\n", verb.Bold("Update Report:"))

			hasUpdated := false
			for _, r := range reports {
				if len(r.updated) > 0 {
					hasUpdated = true
					break
				}
			}

			if hasUpdated {
				verb.Printf(verb.Normal, "\n%s\n", verb.Green("Updated Plugins:"))
				for _, r := range reports {
					if len(r.updated) > 0 {
						verb.Printf(verb.Normal, "  %s:\n", verb.Blue(r.siteName))
						for _, u := range r.updated {
							verb.Printf(verb.Normal, "    - %s (%s -> %s)\n", u.Name, u.OldVersion, u.NewVersion)
						}
					}
				}
			}

			hasFailed := false
			for _, r := range reports {
				if len(r.failed) > 0 {
					hasFailed = true
					break
				}
			}

			if hasFailed {
				verb.Printf(verb.Normal, "\n%s\n", verb.Red("Failed Updates:"))
				for _, r := range reports {
					if len(r.failed) > 0 {
						verb.Printf(verb.Normal, "  %s:\n", verb.Blue(r.siteName))
						for _, f := range r.failed {
							verb.Printf(verb.Normal, "    - %s\n", verb.Red(f))
						}
					}
				}
			}
		}

		return nil
	},
}

var removePluginCmd = &cobra.Command{
	Use:           "remove <target> <plugin-name>",
	Short:         "Remove a plugin from target sites.",
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		if len(args) == 1 {
			return getPluginCompletions(toComplete, false)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
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
			err := removePlugin(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error removing plugin from %s: %v\n", verb.Blue(site.Name), verb.Red(err))
			}
		}
		return nil
	},
}

var infoPluginCmd = &cobra.Command{
	Use:           "info <target> <plugin-name>",
	Short:         "Get info for a plugin on target sites.",
	Args:          cobra.ExactArgs(2),
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return getSiteCompletions()
		}
		if len(args) == 1 {
			return getPluginCompletions(toComplete, false)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
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
			err := pluginInfo(site, pluginName)
			if err != nil {
				verb.Printf(verb.Normal, "Error fetching plugin info on %s: %v\n", verb.Blue(site.Name), verb.Red(err))
			}
		}
		return nil
	},
}

func init() {
	pluginCmd.AddCommand(listPluginCmd)
	pluginCmd.AddCommand(installPluginCmd)
	pluginCmd.AddCommand(updatePluginCmd)
	pluginCmd.AddCommand(removePluginCmd)
	pluginCmd.AddCommand(infoPluginCmd)
	rootCmd.AddCommand(pluginCmd)
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

func updatePlugin(site models.CliSite, pluginName string) ([]wpcli.UpdateResult, []string, error) {
	if pluginName == "" {
		pluginName = "all"
	}
	verb.Printf(verb.Verbose, "Updating '%s' on %s (%s)...\n", verb.Yellow(pluginName), verb.Blue(site.Name), site.ServerName)

	plugins, err := getPluginUpdates(site)
	if err != nil {
		return nil, nil, err
	}

	if len(plugins) == 0 {
		verb.Printf(verb.Normal, "No plugins to update on %s.\n", verb.Blue(site.Name))
		return nil, nil, nil
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
		return nil, nil, nil
	}
	verb.Printf(verb.Normal, "Updating %d plugins on %s...\n", len(toUpdate), verb.Blue(site.Name))

	var failed []string
	var allUpdated []wpcli.UpdateResult
	for _, p := range toUpdate {
		results, err := wpcli.UpdatePlugin(site.SSH, site.Path, []string{p})
		if err != nil {
			failed = append(failed, p)
		} else {
			for _, res := range results {
				if res.Status == "Updated" {
					allUpdated = append(allUpdated, res)
				} else {
					failed = append(failed, res.Name)
				}
			}
		}
	}

	if len(failed) > 0 {
		verb.Printf(verb.Normal, "Failed to update %d plugins on %s: %s\n", len(failed), verb.Blue(site.Name), verb.Red(strings.Join(failed, ", ")))
	}

	if len(allUpdated) > 0 || len(failed) == 0 {
		verb.Printf(verb.Normal, "Successfully updated %d plugins on %s.\n", len(allUpdated), verb.Blue(site.Name))
	}

	// Update cache after updates
	if err := cache.UpdateSitePluginCache(site); err != nil {
		verb.Printf(verb.Normal, "Warning: failed to update cache for site %s: %v\n", verb.Blue(site.Name), verb.Red(err))
	}

	return allUpdated, failed, nil
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
