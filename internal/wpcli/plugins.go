package wpcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// GetPlugins returns a list of installed plugins on the target site.
func GetPlugins(site models.CliSite, skipPlugins bool) ([]models.WPPlugin, error) {
	opts := CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: !skipPlugins, PremiumUpdates: true}
	args := []string{"plugin", "list", PluginListFields, "--format=json"}
	if skipPlugins {
		args = SkippedPluginListArgs(PluginListFields)
	}
	res, err := RunWP(opts, args...)
	if err != nil && IsInvalidFieldError(res.Error) {
		args = slices.DeleteFunc(args, func(a string) bool { return a == PluginListFields })
		res, err = RunWP(opts, args...)
	}
	if err != nil {
		return nil, err
	}

	return ParsePluginList(res.Output, site.ID)
}

// GetPluginsWithUpdates lists a site's plugins with the plugins loaded, so
// premium plugins whose updaters hook into WordPress's update check report
// their updates too; with --skip-plugins they show none, and the update
// check WordPress may run during the listing can save the update list
// without them. If loading the plugins fails (a broken plugin), it lists
// them again skipped, which still reports their installed state.
func GetPluginsWithUpdates(site models.CliSite) ([]models.WPPlugin, error) {
	plugins, err := GetPlugins(site, false)
	if err == nil {
		return plugins, nil
	}
	verb.PrintErrorf(verb.Normal, "Warning: listing plugins on %s with plugins loaded failed, retrying with them skipped: %v\n", site.Name, err)
	return GetPlugins(site, true)
}

// PluginListFields are the fields `wp plugin list` is asked for, for
// ParsePluginList: wp-cli's default ones, update_unavailable_reason and
// update_package. wp-cli versions without one of them fail (see
// IsInvalidFieldError).
//
// update_package is only read to see whether it's empty (see
// models.WPPlugin.UpdateNoPackage): a premium plugin's package URL often
// carries its license key, so it's never kept.
const PluginListFields = "--fields=name,status,update,version,update_version,auto_update,update_unavailable_reason,update_package"

// SkippedPluginListArgs are the `wp plugin list` arguments for a listing
// with --skip-plugins, the fallback when loading the plugins fails. Each
// `wp plugin list` deletes WordPress's cached plugin updates and checks
// again; with the plugins skipped, premium plugins can't add theirs, and
// the saved list would lack them until WordPress checks again, up to 12
// hours later (in wp-admin, the dashboard and the next listing alike).
// --skip-update-check reads the cached list instead, which has them if the
// last check ran with the plugins loaded.
func SkippedPluginListArgs(fields ...string) []string {
	return append(append([]string{"plugin", "list"}, fields...), "--skip-update-check", "--format=json")
}

// IsInvalidFieldError reports whether wp-cli's output (or an error
// carrying it) says a --fields name doesn't exist: wp-cli checks them
// against the first item and fails. A wp-cli older than one of the
// PluginListFields is then asked for its default fields instead, which
// still lets ParsePluginList read the update field, just without a reason;
// falling back to --skip-plugins would lose premium plugins' updates.
func IsInvalidFieldError(s string) bool {
	return strings.Contains(s, "Invalid field")
}

// looseString is a JSON string that also accepts any other value, which it
// reads as "": wp-cli fills some fields with false or null where it has
// nothing to say.
type looseString string

func (s *looseString) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		*s = ""
		return nil
	}
	*s = looseString(v)
	return nil
}

// pluginUpdateUnavailable is wp-cli's update field for a plugin whose
// update needs a newer WordPress or PHP than the site has.
const pluginUpdateUnavailable = "unavailable"

// ParsePluginList parses `wp plugin list --format=json` output (with the
// PluginListFields, or at least name, status, version, update_version and
// auto_update) into plugins of the given site.
func ParsePluginList(output, siteID string) ([]models.WPPlugin, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "[]" {
		return nil, nil
	}

	idx := strings.Index(output, "[")
	if idx == -1 {
		return nil, fmt.Errorf("no valid JSON array found in output")
	}
	output = output[idx:]

	// Every field but the name is loose: wp-cli gives must-use plugins and
	// drop-ins "update": false, and one odd value mustn't fail the whole
	// list.
	type rawPlugin struct {
		Name              string      `json:"name"`
		Status            looseString `json:"status"`
		Update            looseString `json:"update"`
		Version           looseString `json:"version"`
		UpdateVersion     looseString `json:"update_version"`
		AutoUpdate        looseString `json:"auto_update"`
		UnavailableReason looseString `json:"update_unavailable_reason"`
		// Raw so an absent field (a wp-cli asked for its default fields)
		// tells apart from null or "".
		UpdatePackage json.RawMessage `json:"update_package"`
	}

	var raw []rawPlugin
	decoder := json.NewDecoder(strings.NewReader(output))
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to parse plugins JSON: %w", err)
	}

	var plugins []models.WPPlugin
	for _, rp := range raw {
		p := models.WPPlugin{
			SiteID:     siteID,
			Name:       rp.Name,
			Status:     string(rp.Status),
			Version:    string(rp.Version),
			Update:     string(rp.UpdateVersion),
			AutoUpdate: rp.AutoUpdate == "on",
		}
		// An available update without a package: WordPress has nothing to
		// download. Only when the field was asked for.
		if rp.Update == "available" && p.Update != "" && rp.UpdatePackage != nil {
			var pkg string
			_ = json.Unmarshal(rp.UpdatePackage, &pkg)
			p.UpdateNoPackage = strings.TrimSpace(pkg) == ""
		}
		// wp-cli doesn't reset the reason between plugins, so another
		// plugin can carry it: it only belongs to an unavailable update.
		if rp.Update == pluginUpdateUnavailable && p.Update != "" {
			p.UpdateBlocked = strings.TrimSpace(string(rp.UnavailableReason))
			if p.UpdateBlocked == "" {
				p.UpdateBlocked = "The update needs a newer WordPress or PHP version than the site has."
			}
		}
		plugins = append(plugins, p)
	}

	return plugins, nil
}

// AddPlugin installs and optionally activates a plugin. rec, if not nil,
// gets the wp-cli call recorded.
func AddPlugin(site models.CliSite, plugin string, activate bool, rec *[]models.WPCLICall) (bool, error) {
	args := []string{"plugin", "install", plugin}

	// If the plugin is a ZIP file (local or URL), add --force to allow updating.
	lowerPlugin := strings.ToLower(plugin)
	if strings.HasSuffix(lowerPlugin, ".zip") || strings.Contains(lowerPlugin, ".zip?") {
		args = append(args, "--force")
	}

	if activate {
		args = append(args, "--activate")
	}
	// WordPress checks for plugin updates again once the install is done
	// and saves them, so it runs like UpdatePlugin, where premium plugins
	// can add theirs.
	opts := CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: true, PremiumUpdates: true, EnglishMessages: true, Timeout: WriteTimeout, Record: rec}
	res, err := RunWP(opts, args...)
	if err != nil && opts.User != 0 && isInvalidUserError(res.Error) {
		// The cached administrator is gone. wp-cli stops before
		// installing anything, so run it again without one.
		opts.User = 0
		res, err = RunWP(opts, args...)
	}
	if err != nil {
		if strings.Contains(res.Error, "Plugin not found.") {
			return false, fmt.Errorf("plugin not found")
		} else if strings.Contains(res.Error, "Destination folder already exists.") {
			return false, fmt.Errorf("plugin already installed")
		}
		return false, fmt.Errorf("failed to install plugin: %s", shortError(res.Error, err))
	}
	return strings.Contains(res.Output, "Success:"), nil
}

// isInvalidUserError reports whether wp-cli refused its --user: "Error:
// Invalid user ID, email or login: '3'".
func isInvalidUserError(stderr string) bool {
	return strings.Contains(stderr, "Invalid user ID, email or login")
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
// hold whatever per-plugin outcomes WP-CLI reported before failing. A
// plugin that failed has wp-cli's errors and warnings as its Error. rec,
// if not nil, gets the update call recorded.
func UpdatePlugin(site models.CliSite, plugins []string, rec *[]models.WPCLICall) ([]UpdateResult, error) {
	return updatePlugins(site, plugins, false, rec)
}

// UpdatePluginPatch is UpdatePlugin limited to fix releases: each plugin is
// updated to the newest stable release with the same major.minor version
// (e.g. 9.3.0 → 9.3.3 even if 9.4.1 is the latest), looked up on
// WordPress.org. Plugins with no such release, or that aren't on
// WordPress.org, are left alone and don't appear in the results.
func UpdatePluginPatch(site models.CliSite, plugins []string, rec *[]models.WPCLICall) ([]UpdateResult, error) {
	return updatePlugins(site, plugins, true, rec)
}

func updatePlugins(site models.CliSite, plugins []string, patchOnly bool, rec *[]models.WPCLICall) ([]UpdateResult, error) {
	if len(plugins) == 0 {
		return nil, nil
	}

	refreshUpdateCache(site)

	args := []string{"plugin", "update"}
	args = append(args, plugins...)
	if patchOnly {
		args = append(args, "--patch")
	}
	// No --format=json: with it (or csv) wp-cli silences the upgrader, and
	// a failed plugin says only "Error", never why. Its table, tab
	// separated when not on a terminal, keeps the upgrader's warnings
	// ("Download failed. Unauthorized", "some files could not be copied")
	// on stderr.

	timeout := WriteTimeout + time.Duration(len(plugins)-1)*perPluginWriteTimeout
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: true, PremiumUpdates: true, EnglishMessages: true, Timeout: timeout, Record: rec}, args...)
	// WP-CLI prints the per-plugin JSON table even when some plugins fail,
	// so parse it either way.
	updates, parseErr := parseUpdateOutput(res.Output)
	// The short message is wp-cli's own errors and warnings; the whole
	// output, PHP notices and all, is in rec.
	msgs := Messages(res.Error)
	for i := range updates {
		if updates[i].Status != "Updated" && msgs != "" {
			updates[i].Error = msgs
		}
	}
	if err != nil {
		var failure error
		if errors.Is(err, ErrTimeout) {
			failure = fmt.Errorf("failed to update plugin: %w", err)
		} else {
			failure = fmt.Errorf("failed to update plugin: %s", shortError(res.Error, err))
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

	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: true, PremiumUpdates: true}, "plugin", "list", "--fields=name,update_package", "--format=json")
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

// updateTableHeader is the header of `wp plugin update`'s results table.
var updateTableHeader = []string{"name", "old_version", "new_version", "status"}

// parseUpdateOutput extracts the per-plugin results from `wp plugin update`
// output: its results table, which wp-cli prints tab separated when not on
// a terminal, after the upgrader's progress lines, or a JSON array (from
// --format=json). Output with no updates ("already up to date") yields no
// results and no error.
func parseUpdateOutput(output string) ([]UpdateResult, error) {
	output = strings.TrimSpace(output)
	if output == "" || strings.Contains(output, "Success: Plugin already up to date") || strings.Contains(output, "Success: Plugins already up to date") {
		return nil, nil
	}
	if updates, ok := parseUpdateTable(output); ok {
		return updates, nil
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

// parseUpdateTable reads `wp plugin update`'s tab-separated results table,
// reporting whether output has one.
func parseUpdateTable(output string) ([]UpdateResult, bool) {
	lines := strings.Split(output, "\n")
	start := -1
	for i, line := range lines {
		if slices.Equal(strings.Split(strings.TrimRight(line, "\r"), "\t"), updateTableHeader) {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil, false
	}
	updates := []UpdateResult{}
	for _, line := range lines[start:] {
		cols := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(cols) != len(updateTableHeader) {
			break
		}
		updates = append(updates, UpdateResult{Name: cols[0], OldVersion: cols[1], NewVersion: cols[2], Status: cols[3]})
	}
	return updates, true
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

// ErrPluginNotFound is GetPluginInfo's error for a plugin that isn't
// installed on the site.
var ErrPluginNotFound = errors.New("plugin not found")

// GetPluginInfo returns a plugin's info as the site reports it (`wp plugin
// get`), or an error wrapping ErrPluginNotFound if it isn't installed.
func GetPluginInfo(site models.CliSite, plugin string, timeout ...time.Duration) (*models.PluginInfo, error) {
	t := time.Duration(0)
	if len(timeout) > 0 {
		t = timeout[0]
	}
	res, err := RunWP(CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, IncludePlugins: true, Timeout: t}, "plugin", "get", plugin, "--format=json")
	if err != nil {
		if strings.Contains(res.Error, "plugin could not be found.") {
			return nil, fmt.Errorf("plugin %s: %w", plugin, ErrPluginNotFound)
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

// GetPluginDetails returns the details of every plugin on a site with one
// `wp plugin list` call. WP-CLI versions without the author field are
// retried without it.
func GetPluginDetails(site models.CliSite) ([]models.PluginDetails, error) {
	fields := "name,title,version,status,author,update_version"
	// Like GetPlugins: the listing checks for updates again and saves
	// them, so premium plugins must be able to add theirs.
	opts := CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, User: resolveAdminUser(site), IncludePlugins: true, PremiumUpdates: true}
	res, err := RunWP(opts, "plugin", "list", "--fields="+fields, "--format=json")
	if err != nil && strings.Contains(res.Error, "Invalid field") {
		res, err = RunWP(opts, "plugin", "list", "--fields=name,title,version,status,update_version", "--format=json")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list plugins: %w (stderr: %s)", err, res.Error)
	}
	return parsePluginDetails(res.Output)
}

func parsePluginDetails(output string) ([]models.PluginDetails, error) {
	output = strings.TrimSpace(output)
	idx := strings.Index(output, "[")
	if idx == -1 {
		return nil, fmt.Errorf("no valid JSON array found in output")
	}
	var details []models.PluginDetails
	if err := json.NewDecoder(strings.NewReader(output[idx:])).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to parse plugin list JSON: %w", err)
	}
	return details, nil
}
