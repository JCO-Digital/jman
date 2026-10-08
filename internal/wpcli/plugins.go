package wpcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// GetPlugins returns a list of installed plugins on the target site.
func GetPlugins(site models.CliSite, skipPlugins bool) ([]models.WPPlugin, error) {
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: !skipPlugins}, "plugin", "list", "--format=json")
	if err != nil {
		return nil, err
	}

	output := strings.TrimSpace(res.Output)
	if output == "" || output == "[]" {
		return nil, nil
	}

	idx := strings.Index(output, "[")
	if idx == -1 {
		return nil, fmt.Errorf("no valid JSON array found in output")
	}
	output = output[idx:]

	type rawPlugin struct {
		Name          string `json:"name"`
		Status        string `json:"status"`
		Version       string `json:"version"`
		UpdateVersion string `json:"update_version"`
		AutoUpdate    string `json:"auto_update"`
	}

	var raw []rawPlugin
	decoder := json.NewDecoder(strings.NewReader(output))
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to parse plugins JSON: %w", err)
	}

	var plugins []models.WPPlugin
	for _, rp := range raw {
		plugins = append(plugins, models.WPPlugin{
			SiteID:     site.ID,
			Name:       rp.Name,
			Status:     rp.Status,
			Version:    rp.Version,
			Update:     rp.UpdateVersion,
			AutoUpdate: rp.AutoUpdate == "on",
		})
	}

	return plugins, nil
}

// AddPlugin installs and optionally activates a plugin.
func AddPlugin(site models.CliSite, plugin string, activate bool) (bool, error) {
	args := []string{"plugin", "install", plugin}

	// If the plugin is a ZIP file (local or URL), add --force to allow updating.
	lowerPlugin := strings.ToLower(plugin)
	if strings.HasSuffix(lowerPlugin, ".zip") || strings.Contains(lowerPlugin, ".zip?") {
		args = append(args, "--force")
	}

	if activate {
		args = append(args, "--activate")
	}
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, IncludePlugins: true, Timeout: WriteTimeout}, args...)
	if err != nil {
		if strings.Contains(res.Error, "Plugin not found.") {
			return false, fmt.Errorf("plugin not found")
		} else if strings.Contains(res.Error, "Destination folder already exists.") {
			return false, fmt.Errorf("plugin already installed")
		}
		return false, fmt.Errorf("failed to install plugin: %w (stderr: %s)", err, res.Error)
	}
	return strings.Contains(res.Output, "Success:"), nil
}

type UpdateResult struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// refreshUpdateCache forces WordPress to drop its cached update_plugins
// transient. `wp plugin update` tries to refresh it itself, but WordPress
// core throttles that refresh (it no-ops if the installed version hasn't
// changed since the last check), so a plugin whose update was only just
// reported by `wp plugin list` can be silently skipped as "up to date".
// Debounced per site since the transient is site-wide: refreshing once
// covers every plugin in a batch, not just the one being updated.
func refreshUpdateCache(site models.CliSite) {
	if !shouldRefreshUpdateCache(site.ID) {
		return
	}
	if _, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path}, "eval", "delete_site_transient('update_plugins');"); err != nil {
		verb.Printf(verb.Verbose, "Failed to refresh plugin update cache: %v\n", err)
	}
}

// perPluginWriteTimeout is added to WriteTimeout for every plugin beyond the
// first in one `wp plugin update` call, since each is downloaded and
// installed in turn.
const perPluginWriteTimeout = 2 * time.Minute

// UpdatePlugin updates one or more plugins in a single WP-CLI call. On
// failure the error is an *UpdateFailure, and the returned results still
// hold whatever per-plugin outcomes WP-CLI reported before failing.
func UpdatePlugin(site models.CliSite, plugins []string) ([]UpdateResult, error) {
	return updatePlugins(site, plugins, false)
}

// UpdatePluginPatch is UpdatePlugin limited to fix releases: each plugin is
// updated to the newest stable release with the same major.minor version
// (e.g. 9.3.0 → 9.3.3 even if 9.4.1 is the latest), looked up on
// WordPress.org. Plugins with no such release, or that aren't on
// WordPress.org, are left alone and don't appear in the results.
func UpdatePluginPatch(site models.CliSite, plugins []string) ([]UpdateResult, error) {
	return updatePlugins(site, plugins, true)
}

func updatePlugins(site models.CliSite, plugins []string, patchOnly bool) ([]UpdateResult, error) {
	if len(plugins) == 0 {
		return nil, nil
	}

	refreshUpdateCache(site)

	args := []string{"plugin", "update"}
	args = append(args, plugins...)
	if patchOnly {
		args = append(args, "--patch")
	}
	args = append(args, "--format=json")

	timeout := WriteTimeout + time.Duration(len(plugins)-1)*perPluginWriteTimeout
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: true, Timeout: timeout}, args...)
	// WP-CLI prints the per-plugin JSON table even when some plugins fail,
	// so parse it either way.
	updates, parseErr := parseUpdateOutput(res.Output)
	if err != nil {
		var failure error
		switch {
		case errors.Is(err, ErrTimeout):
			failure = fmt.Errorf("failed to update plugin: %w", err)
		// If the error message from RunWP is a specific WP-CLI error, return it
		// without the full stderr blob to avoid noise from PHP warnings/notices.
		case strings.HasPrefix(err.Error(), "Error:") || strings.HasPrefix(err.Error(), "Fatal error:"):
			// For the specific "No plugins updated" failure, return a clean message.
			if strings.Contains(err.Error(), "No plugins updated (1 failed)") {
				failure = fmt.Errorf("failed to update plugin")
			} else {
				failure = fmt.Errorf("failed to update plugin: %w", err)
			}
		default:
			failure = fmt.Errorf("failed to update plugin: %w (stderr: %s)", err, res.Error)
		}
		return updates, checkFailedUpdate(site, plugins, failure)
	}
	if parseErr != nil {
		return nil, parseErr
	}

	for _, update := range updates {
		if update.Status == "Updated" {
			verb.Printf(verb.Normal, "Updated %s from %s to %s\n", update.Name, update.OldVersion, update.NewVersion)
		} else {
			verb.Printf(verb.Normal, "Failed to update %s: %s\n", update.Name, update.Status)
		}
	}
	return updates, nil
}

// PluginUpdatePackages returns the package URL of each plugin's pending
// update on the site, keyed by plugin slug; plugins without an update are
// left out. It refreshes WordPress's cached update information first, like
// UpdatePlugin.
func PluginUpdatePackages(site models.CliSite) (map[string]string, error) {
	refreshUpdateCache(site)

	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: true}, "plugin", "list", "--fields=name,update_package", "--format=json")
	if err != nil {
		return nil, err
	}

	output := strings.TrimSpace(res.Output)
	idx := strings.Index(output, "[")
	if idx == -1 {
		if output == "" {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("no valid JSON array found in output")
	}

	var raw []struct {
		Name          string  `json:"name"`
		UpdatePackage *string `json:"update_package"`
	}
	if err := json.NewDecoder(strings.NewReader(output[idx:])).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to parse plugin update packages: %w", err)
	}
	packages := make(map[string]string, len(raw))
	for _, p := range raw {
		if p.UpdatePackage != nil && *p.UpdatePackage != "" {
			packages[p.Name] = *p.UpdatePackage
		}
	}
	return packages, nil
}

// IsWordPressOrgPackage reports whether a plugin update package is
// downloaded from WordPress.org.
func IsWordPressOrgPackage(pkg string) bool {
	u, err := url.Parse(pkg)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), "downloads.wordpress.org")
}

// parseUpdateOutput extracts the per-plugin results from `wp plugin update
// --format=json` output. Output with no updates ("already up to date")
// yields no results and no error.
func parseUpdateOutput(output string) ([]UpdateResult, error) {
	output = strings.TrimSpace(output)
	if output == "" || strings.Contains(output, "Success: Plugin already up to date") || strings.Contains(output, "Success: Plugins already up to date") {
		return nil, nil
	}

	// wp-cli might output non-JSON text before the JSON array (e.g. update notices)
	idx := strings.Index(output, "[")
	if idx == -1 {
		if strings.Contains(output, "Success:") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to parse update result: no JSON array found")
	}

	var updates []UpdateResult
	decoder := json.NewDecoder(strings.NewReader(output[idx:]))
	if err := decoder.Decode(&updates); err != nil {
		return nil, fmt.Errorf("failed to parse update result: %w", err)
	}
	return updates, nil
}

// UpdateFailure is UpdatePlugin's error when the update command failed or
// timed out. A failed or killed update can leave plugins half-updated or the
// site stuck in maintenance mode, so it also carries what a follow-up check
// of the site found.
type UpdateFailure struct {
	Err error
	// Versions holds the version now installed for each requested plugin
	// the check found; nil if the check itself failed.
	Versions map[string]string
	// MaintenanceMode reports whether the site is left in maintenance mode.
	MaintenanceMode bool
}

func (e *UpdateFailure) Error() string {
	msg := e.Err.Error()
	if e.MaintenanceMode {
		msg += "; the site is in maintenance mode (WordPress clears it after 10 minutes, or run `wp maintenance-mode deactivate` once the update has stopped)"
	}
	return msg
}

func (e *UpdateFailure) Unwrap() error { return e.Err }

// checkFailedUpdate inspects the site after a failed plugin update and
// wraps cause in an UpdateFailure describing what it found. The check
// skips plugins so a plugin broken by the update can't break the check.
func checkFailedUpdate(site models.CliSite, plugins []string, cause error) *UpdateFailure {
	failure := &UpdateFailure{Err: cause}

	if installed, err := GetPlugins(site, true); err != nil {
		verb.Printf(verb.Normal, "Could not check plugin versions on %s after failed update: %v\n", site.Name, err)
	} else {
		failure.Versions = map[string]string{}
		for _, p := range installed {
			for _, name := range plugins {
				if p.Name == name {
					failure.Versions[name] = p.Version
				}
			}
		}
	}

	failure.MaintenanceMode = maintenanceModeActive(site)
	return failure
}

// maintenanceModeActive reports whether the site is in maintenance mode.
// `wp maintenance-mode is-active` exits 0 when active and 1, silently, when
// not; anything else (e.g. an old WP-CLI without the command) counts as not
// active. It deliberately doesn't count towards the site's failure tracker.
func maintenanceModeActive(site models.CliSite) bool {
	res, err := RunWP(CliOptions{SSH: site.SSH, Path: site.Path}, "maintenance-mode", "is-active")
	if err == nil {
		return true
	}
	if strings.TrimSpace(res.Error) != "" {
		verb.Printf(verb.Verbose, "Could not check maintenance mode on %s: %v\n", site.Name, err)
	}
	return false
}

// PluginAction runs `wp plugin <action>` for the given plugins in one call.
// action is "activate", "deactivate", "delete" (files only) or "uninstall"
// (runs each plugin's uninstall routine, deactivating it first).
//
// Deactivate and delete run with other plugins skipped, so they still work
// when a broken plugin makes WordPress fatal on load, which is often why a
// plugin is being deactivated. Activate and uninstall need the plugins'
// code loaded to run their hooks.
func PluginAction(site models.CliSite, action string, plugins []string) error {
	if len(plugins) == 0 {
		return nil
	}
	includePlugins := true
	switch action {
	case "activate", "uninstall":
	case "deactivate", "delete":
		includePlugins = false
	default:
		return fmt.Errorf("unsupported plugin action %q", action)
	}

	args := append([]string{"plugin", action}, plugins...)
	if action == "uninstall" {
		args = append(args, "--deactivate")
	}
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, IncludePlugins: includePlugins, Timeout: WriteTimeout}, args...)
	if err != nil {
		if msg := strings.TrimSpace(res.Error); msg != "" {
			return fmt.Errorf("failed to %s plugin: %s", action, msg)
		}
		return fmt.Errorf("failed to %s plugin: %w", action, err)
	}
	return nil
}

// RemovePlugin uninstalls and deactivates a plugin.
func RemovePlugin(site models.CliSite, plugin string) (bool, error) {
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, IncludePlugins: true, Timeout: WriteTimeout}, "plugin", "uninstall", plugin, "--deactivate")
	if err != nil {
		return false, fmt.Errorf("failed to remove plugin: %w (stderr: %s)", err, res.Error)
	}
	return strings.Contains(res.Output, "Success: Uninstalled"), nil
}

func GetPluginInfo(site models.CliSite, plugin string, timeout ...time.Duration) (*models.PluginInfo, error) {
	t := time.Duration(0)
	if len(timeout) > 0 {
		t = timeout[0]
	}
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, IncludePlugins: true, Timeout: t}, "plugin", "get", plugin, "--format=json")
	if err != nil {
		if strings.Contains(res.Error, "plugin could not be found.") {
			return nil, fmt.Errorf("plugin %s not found", plugin)
		}
		return nil, err
	}

	output := strings.TrimSpace(res.Output)
	idx := strings.Index(output, "{")
	if idx == -1 {
		return nil, fmt.Errorf("no valid JSON object found in output")
	}

	type rawInfo struct {
		Slug        string `json:"name"`
		Name        string `json:"title"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Status      string `json:"status"`
	}

	var info rawInfo
	decoder := json.NewDecoder(strings.NewReader(output[idx:]))
	if err := decoder.Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse plugin info JSON: %w", err)
	}

	return &models.PluginInfo{
		Name:    info.Name,
		Slug:    info.Slug,
		Version: info.Version,
		Author:  info.Author,
	}, nil
}
