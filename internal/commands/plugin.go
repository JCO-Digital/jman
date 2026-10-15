package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/mirror"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
	"github.com/spf13/cobra"
)

var (
	flagPluginLive bool
	flagAllowMajor bool
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Plugin actions on target sites.",
	Long: `List, install, update or remove plugins on target sites. Supports WordPress.org slugs, ZIP URLs, local ZIP files, or aliases defined in config.

Installs, updates and removals run as jobs in jman-api, which applies update locks and keeps its plugin data current; jman waits for them and shows the results. They need the execute level.`,
}

var listPluginCmd = &cobra.Command{
	Use:   "list <target>",
	Short: "List plugins on target sites.",
	Long: `Lists the plugins on target sites from jman-api's data. Plugins whose updates are limited to fix releases by an update lock are marked as locked.

With --live, the plugins are read from the sites over SSH instead.`,
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
		sites, m, err := promptSites(cmd, args[0])
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		for _, site := range sites {
			plugins := m.SitePlugins(site.ID)
			if flagPluginLive {
				verb.Printf(verb.Verbose, "Listing plugins on %s (%s)...\n", verb.Blue(site.Name), site.ServerName)
				if plugins, err = wpcli.GetPlugins(site, false); err != nil {
					verb.Printf(verb.Normal, "Error listing plugins on %s:\n%v\n", verb.Blue(site.Name), verb.Red(err))
					continue
				}
			}
			printPlugins(m, site, plugins)
		}
		return nil
	},
}

var installPluginCmd = &cobra.Command{
	Use:           "install <target> <plugin-name>",
	Short:         "Install a plugin on target sites.",
	Long:          "Installs and activates a plugin on target sites. Supports WordPress.org slugs, ZIP URLs, local ZIP files, or aliases defined in config.",
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
		pluginName := args[1]
		resolvePluginAlias(&pluginName)

		sites, _, err := promptSites(cmd, args[0])
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}
		c, err := apiClient()
		if err != nil {
			return err
		}
		ctx, stop := jobContext(cmd)
		defer stop()

		localZip := isLocalZip(pluginName)
		var queued []models.UpdateJob
		for _, site := range sites {
			verb.Printf(verb.Verbose, "Queueing install of '%s' on %s (%s)...\n", pluginName, verb.Blue(site.Name), verb.Yellow(site.ServerName))
			var job models.UpdateJob
			path := "/sites/" + site.ID + "/plugin-install"
			if localZip {
				err = c.PostFile(ctx, path, pluginName, map[string]string{"activate": "true"}, &job)
			} else {
				err = c.Post(ctx, path, map[string]any{"source": pluginName, "activate": true}, &job)
			}
			if err != nil {
				if isStatus(err, http.StatusConflict) {
					verb.Printf(verb.Normal, "'%s' is already installed on %s.\n", verb.Yellow(pluginName), verb.Blue(site.Name))
					continue
				}
				if fatal := fatalAPIError(err); fatal != nil {
					return fatal
				}
				verb.Printf(verb.Normal, "Error installing plugin on %s: %v\n", verb.Blue(site.Name), verb.Red(err))
				continue
			}
			queued = append(queued, job)
		}

		return waitForJobs(ctx, c, queued, sites, func(site models.CliSite, job models.UpdateJob) {
			r, ok := firstResult(job)
			if !ok || r.Status == models.UpdateFailed {
				verb.Printf(verb.Normal, "Failed to install '%s' on %s: %s\n", verb.Yellow(pluginName), verb.Blue(site.Name), verb.Red(jobError(job, r)))
				return
			}
			verb.Printf(verb.Normal, "Successfully installed and activated '%s' on %s.\n", verb.Yellow(r.Name), verb.Blue(site.Name))
		})
	},
}

var updatePluginCmd = &cobra.Command{
	Use:   "update <target> [plugin-name]",
	Short: "Update plugins on target sites.",
	Long: `Updates a specific plugin, or all plugins if no plugin name is given.

Update-locked plugins only get fix releases. To update one locked plugin on one site past that, pass --allow-major.`,
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
	RunE: runPluginUpdate,
}

var removePluginCmd = &cobra.Command{
	Use:           "remove <target> <plugin-name>",
	Short:         "Remove a plugin from target sites.",
	Long:          "Deactivates a plugin if it is active, then uninstalls it: its uninstall routine runs, which usually removes its data, and its files are deleted.",
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
		pluginName := args[1]
		sites, m, err := promptSites(cmd, args[0])
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}
		c, err := apiClient()
		if err != nil {
			return err
		}
		ctx, stop := jobContext(cmd)
		defer stop()

		for _, site := range sites {
			if err := removePlugin(ctx, c, m, site, pluginName); err != nil {
				if fatal := fatalAPIError(err); fatal != nil {
					return fatal
				}
				verb.Printf(verb.Normal, "Error removing plugin from %s: %v\n", verb.Blue(site.Name), verb.Red(err))
			}
		}
		return nil
	},
}

var infoPluginCmd = &cobra.Command{
	Use:           "info <target> <plugin-name>",
	Short:         "Get info for a plugin on target sites.",
	Long:          "Reads a plugin's details from target sites over SSH.",
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
		pluginName := args[1]
		sites, _, err := promptSites(cmd, args[0])
		if err != nil {
			return err
		}
		if len(sites) == 0 {
			fmt.Println("Operation cancelled or no sites matched.")
			return nil
		}

		for _, site := range sites {
			if err := pluginInfo(site, pluginName); err != nil {
				verb.Printf(verb.Normal, "Error fetching plugin info on %s: %v\n", verb.Blue(site.Name), verb.Red(err))
			}
		}
		return nil
	},
}

func init() {
	listPluginCmd.Flags().BoolVar(&flagPluginLive, "live", false, "read the plugins from the sites over SSH")
	updatePluginCmd.Flags().BoolVar(&flagAllowMajor, "allow-major", false, "update a locked plugin past fix releases (one plugin on one site only)")
	pluginCmd.AddCommand(listPluginCmd)
	pluginCmd.AddCommand(installPluginCmd)
	pluginCmd.AddCommand(updatePluginCmd)
	pluginCmd.AddCommand(removePluginCmd)
	pluginCmd.AddCommand(infoPluginCmd)
	rootCmd.AddCommand(pluginCmd)
}

func printPlugins(m *mirror.Mirror, site models.CliSite, plugins []models.WPPlugin) {
	if len(plugins) == 0 {
		verb.Printf(verb.Normal, "No plugins found on %s.\n", verb.Blue(site.Name))
		return
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
		if m.IsUpdateLocked(site.ID, plugin.Name) {
			update += " " + verb.Gray("[locked]")
		}
		verb.Printf(verb.Normal, "- %s %s%s %s\n", m.DisplayPluginName(plugin.Name, true, true), status, plugin.Version, update)
	}
}

func runPluginUpdate(cmd *cobra.Command, args []string) error {
	pluginName := ""
	if len(args) > 1 {
		pluginName = args[1]
	}
	sites, m, err := promptSites(cmd, args[0])
	if err != nil {
		return err
	}
	if len(sites) == 0 {
		fmt.Println("Operation cancelled or no sites matched.")
		return nil
	}
	if flagAllowMajor && (pluginName == "" || len(sites) != 1) {
		return errors.New("--allow-major needs a single plugin on a single site")
	}
	c, err := apiClient()
	if err != nil {
		return err
	}
	ctx, stop := jobContext(cmd)
	defer stop()

	var queued []models.UpdateJob
	for _, site := range sites {
		job, err := queuePluginUpdates(ctx, c, m, site, pluginName)
		if err != nil {
			if fatal := fatalAPIError(err); fatal != nil {
				return fatal
			}
			verb.Printf(verb.Normal, "Error updating plugins on %s: %v\n", verb.Blue(site.Name), verb.Red(err))
			continue
		}
		if job != nil {
			queued = append(queued, *job)
		}
	}
	if len(queued) == 0 {
		return nil
	}

	type siteReport struct {
		siteName string
		updated  []models.UpdateResult
		failed   []string
	}
	var reports []siteReport
	verb.Printf(verb.Normal, "\nWaiting for jman-api to update %d site(s)...\n", len(queued))
	err = waitForJobs(ctx, c, queued, sites, func(site models.CliSite, job models.UpdateJob) {
		rep := siteReport{siteName: site.Name}
		if job.Status != models.UpdateJobDone && len(job.Results) == 0 {
			verb.Printf(verb.Normal, "%s: %s\n", verb.Blue(site.Name), verb.Red(jobError(job, models.UpdateResult{})))
			rep.failed = append(rep.failed, "all ("+jobError(job, models.UpdateResult{})+")")
			reports = append(reports, rep)
			return
		}
		verb.Printf(verb.Normal, "%s:\n", verb.Blue(site.Name))
		for _, r := range job.Results {
			switch r.Status {
			case models.UpdateUpdated:
				verb.Printf(verb.Normal, "  %s %s (%s -> %s)\n", verb.Green("✓"), r.Name, r.OldVersion, r.NewVersion)
				rep.updated = append(rep.updated, r)
			case models.UpdateUpToDate:
				verb.Printf(verb.Normal, "  - %s is up to date\n", r.Name)
			case models.UpdateSkippedLocked:
				verb.Printf(verb.Normal, "  - %s skipped: locked, and no fix release is available\n", r.Name)
			default:
				verb.Printf(verb.Normal, "  %s %s: %s\n", verb.Red("✗"), r.Name, verb.Red(jobError(job, r)))
				rep.failed = append(rep.failed, r.Name)
			}
			if r.Note != "" {
				verb.Printf(verb.Normal, "    %s\n", verb.Yellow(r.Note))
			}
		}
		if job.Error != "" && job.Status == models.UpdateJobDone {
			verb.Printf(verb.Normal, "  %s %s\n", verb.Yellow("Warning:"), job.Error)
		}
		if len(rep.updated) > 0 || len(rep.failed) > 0 {
			reports = append(reports, rep)
		}
	})
	if err != nil {
		return err
	}

	if len(queued) > 1 && len(reports) > 0 {
		verb.Printf(verb.Normal, "\n%s\n", verb.Bold("Update Report:"))
		printed := false
		for _, r := range reports {
			if len(r.updated) == 0 {
				continue
			}
			if !printed {
				verb.Printf(verb.Normal, "\n%s\n", verb.Green("Updated Plugins:"))
				printed = true
			}
			verb.Printf(verb.Normal, "  %s:\n", verb.Blue(r.siteName))
			for _, u := range r.updated {
				verb.Printf(verb.Normal, "    - %s (%s -> %s)\n", u.Name, u.OldVersion, u.NewVersion)
			}
		}
		printed = false
		for _, r := range reports {
			if len(r.failed) == 0 {
				continue
			}
			if !printed {
				verb.Printf(verb.Normal, "\n%s\n", verb.Red("Failed Updates:"))
				printed = true
			}
			verb.Printf(verb.Normal, "  %s:\n", verb.Blue(r.siteName))
			for _, f := range r.failed {
				verb.Printf(verb.Normal, "    - %s\n", verb.Red(f))
			}
		}
	}
	return nil
}

// queuePluginUpdates checks a site for plugin updates through jman-api,
// shows them, and queues an update job for pluginName (or all plugins with
// updates if it is ""). It returns nil if there was nothing to update.
func queuePluginUpdates(ctx context.Context, c *client.Client, m *mirror.Mirror, site models.CliSite, pluginName string) (*models.UpdateJob, error) {
	verb.Printf(verb.Verbose, "Checking for plugin updates on %s (%s)...\n", verb.Blue(site.Name), site.ServerName)
	var updates []models.WPPlugin
	if err := c.Get(ctx, "/sites/"+site.ID+"/plugin-updates", &updates); err != nil {
		return nil, err
	}
	if len(updates) == 0 {
		verb.Printf(verb.Normal, "No plugins to update on %s.\n", verb.Blue(site.Name))
		return nil, nil
	}

	verb.Printf(verb.Normal, "Found %d plugins with updates available on %s:\n", len(updates), verb.Blue(site.Name))
	var toUpdate []string
	for _, p := range updates {
		note := ""
		if pluginName != "" && p.Name != pluginName {
			note = " (skipped)"
		} else {
			toUpdate = append(toUpdate, p.Name)
		}
		if m.IsUpdateLocked(site.ID, p.Name) {
			note += " " + verb.Gray("[locked: fix releases only]")
		}
		verb.Printf(verb.Normal, "- %s (%s -> %s)%s\n", m.DisplayPluginName(p.Name, false, false), p.Version, p.Update, note)
	}
	if len(toUpdate) == 0 {
		verb.Printf(verb.Normal, "No matching plugins to update on %s.\n", verb.Blue(site.Name))
		return nil, nil
	}

	body := map[string]any{"jobs": []map[string]any{{"site_id": site.ID, "plugins": toUpdate, "allow_major": flagAllowMajor}}}
	var jobs []models.UpdateJob
	if err := c.Post(ctx, "/plugin-update-jobs", body, &jobs); err != nil {
		return nil, err
	}
	if len(jobs) != 1 {
		return nil, fmt.Errorf("jman-api queued %d jobs, expected 1", len(jobs))
	}
	return &jobs[0], nil
}

// removePlugin deactivates the plugin on a site if it is active, then
// uninstalls it, each as a jman-api job.
func removePlugin(ctx context.Context, c *client.Client, m *mirror.Mirror, site models.CliSite, pluginName string) error {
	status := ""
	for _, p := range m.SitePlugins(site.ID) {
		if p.Name == pluginName {
			status = p.Status
		}
	}
	switch status {
	case "":
		verb.Printf(verb.Normal, "'%s' is not installed on %s.\n", verb.Yellow(pluginName), verb.Blue(site.Name))
		return nil
	case "active":
		verb.Printf(verb.Verbose, "Deactivating '%s' on %s...\n", pluginName, site.Name)
		if err := runPluginAction(ctx, c, site, "deactivate", pluginName); err != nil {
			return fmt.Errorf("deactivating failed: %w", err)
		}
	}

	verb.Printf(verb.Verbose, "Uninstalling '%s' from %s...\n", pluginName, site.Name)
	if err := runPluginAction(ctx, c, site, "uninstall", pluginName); err != nil {
		return err
	}
	verb.Printf(verb.Normal, "Successfully removed '%s' from %s.\n", verb.Yellow(pluginName), verb.Blue(site.Name))
	return nil
}

// runPluginAction queues one plugin action job and waits for it.
func runPluginAction(ctx context.Context, c *client.Client, site models.CliSite, action, pluginName string) error {
	var job models.UpdateJob
	if err := c.Post(ctx, "/sites/"+site.ID+"/plugin-actions", map[string]any{"action": action, "plugins": []string{pluginName}}, &job); err != nil {
		return err
	}
	var result error
	err := c.WaitForJobs(ctx, []models.UpdateJob{job}, func(j models.UpdateJob) {
		if r, ok := firstResult(j); !ok || r.Status == models.UpdateFailed {
			result = errors.New(jobError(j, r))
		}
	})
	if err != nil {
		return err
	}
	return result
}

func pluginInfo(site models.CliSite, pluginName string) error {
	verb.Printf(verb.Verbose, "Fetching info for '%s' on %s (%s)...\n", verb.Yellow(pluginName), verb.Blue(site.Name), site.ServerName)
	info, err := wpcli.GetPluginInfo(site, pluginName)
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

// jobContext returns the context jman waits for jobs in, cancelled on
// Ctrl-C. Cancelling only stops the waiting: queued jobs still run.
func jobContext(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(cmd.Context(), os.Interrupt)
}

// waitForJobs waits for queued jobs and calls done for each, with its
// site, as it finishes.
func waitForJobs(ctx context.Context, c *client.Client, jobs []models.UpdateJob, sites []models.CliSite, done func(models.CliSite, models.UpdateJob)) error {
	if len(jobs) == 0 {
		return nil
	}
	byID := make(map[string]models.CliSite, len(sites))
	for _, s := range sites {
		byID[s.ID] = s
	}
	err := c.WaitForJobs(ctx, jobs, func(job models.UpdateJob) {
		site, ok := byID[job.SiteID]
		if !ok {
			site = models.CliSite{ID: job.SiteID, Name: job.SiteID}
		}
		done(site, job)
	})
	if errors.Is(err, context.Canceled) {
		return errors.New("stopped waiting; the queued jobs keep running in jman-api, see the web UI for their results")
	}
	return err
}

// firstResult returns a job's first result, for jobs with one target.
func firstResult(job models.UpdateJob) (models.UpdateResult, bool) {
	if len(job.Results) == 0 {
		return models.UpdateResult{}, false
	}
	return job.Results[0], true
}

// jobError describes why a job or one of its results failed.
func jobError(job models.UpdateJob, r models.UpdateResult) string {
	switch {
	case r.Error != "":
		return r.Error
	case job.Error != "":
		return job.Error
	case job.Status == models.UpdateJobInterrupted:
		return "jman-api restarted while the job was running; check the site"
	}
	return "job " + job.Status
}

// fatalAPIError returns an error that should stop a command instead of
// moving on to the next site: a missing login or level, or an outdated CLI.
func fatalAPIError(err error) error {
	var upgrade *client.UpgradeRequiredError
	switch {
	case errors.Is(err, client.ErrUnauthorized), errors.As(err, &upgrade):
		return err
	case isStatus(err, http.StatusForbidden):
		return errors.New("changing plugins and WordPress core needs the execute level in jman-api")
	}
	return nil
}

func isStatus(err error, status int) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

// isLocalZip reports whether name is the path of a local ZIP file.
func isLocalZip(name string) bool {
	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		return false
	}
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
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
