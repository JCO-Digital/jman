package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/JCO-Digital/jman/internal/agent/scan"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// wpDataConcurrency is how many sites' WordPress data is collected at once.
const wpDataConcurrency = 3

// maxWPDataPayloads bounds how many sites may send full WordPress data in
// one report (each up to a few tens of KB), keeping reports well inside
// jman-api's body limit even when every site changed at once (e.g. the
// first collection on a server). Sites beyond it are collected again on
// the next cycle.
const maxWPDataPayloads = 40

// maxWPDataPayloadBytes bounds the encoded size of the full payloads in one
// report, as a site's user list can make a payload much larger than its
// plugins do.
const maxWPDataPayloadBytes = 1 << 20

// payloadSize returns the encoded size of a site's WordPress data.
func payloadSize(data *models.AgentWPData) int {
	b, err := json.Marshal(data)
	if err != nil {
		return 0
	}
	return len(b)
}

// lastWPCollection holds when each site's WordPress data was last
// collected and reported. It lives in memory: after a restart every site
// is collected once more, which costs a round of wp-cli calls but sends
// little, since unchanged sites only send their hash.
var (
	lastWPCollection   = map[string]time.Time{}
	lastWPCollectionMu sync.Mutex
)

// wpDataDue reports whether a site's WordPress data should be collected
// this cycle. A minute of slack keeps a site from slipping a whole report
// cycle because the previous collection finished slightly late.
func wpDataDue(siteID string, interval time.Duration, now time.Time) bool {
	lastWPCollectionMu.Lock()
	defer lastWPCollectionMu.Unlock()
	last, ok := lastWPCollection[siteID]
	return !ok || now.Sub(last) >= interval-time.Minute
}

func markWPCollected(siteIDs []string, at time.Time) {
	lastWPCollectionMu.Lock()
	defer lastWPCollectionMu.Unlock()
	for _, id := range siteIDs {
		lastWPCollection[id] = at
	}
}

// newRunner is a seam for tests.
var newRunner = func() (wpDataRunner, error) { return newWPRunner() }

// wpDataRunner runs wp-cli for a site; *wpRunner in production.
type wpDataRunner interface {
	run(ctx context.Context, id siteIdentity, sitePath string, args ...string) (string, error)
	runWithPlugins(ctx context.Context, id siteIdentity, sitePath string, args ...string) (string, error)
}

// resolvePath and resolveIdentity are seams for tests.
var (
	resolvePath = ResolveSitePath
	// readSiteCoreVersion reads a site's WordPress version and locale.
	readSiteCoreVersion = scan.ReadCoreVersion
	resolveIdentity     = resolveSiteIdentity
)

// collectWPDataForSites collects the WordPress data of the due WordPress
// sites in the manifest and returns it by site ID, plus the IDs of the
// sites whose collection should count as done once the report is sent.
func collectWPDataForSites(ctx context.Context, manifest *models.AgentManifest, now time.Time) (map[string]*models.AgentWPData, []string) {
	if manifest.WPDataIntervalMinutes <= 0 {
		return nil, nil
	}
	interval := time.Duration(manifest.WPDataIntervalMinutes) * time.Minute

	var due []models.AgentManifestSite
	for _, site := range manifest.Sites {
		if site.IsWordpress && wpDataDue(site.SiteID, interval, now) {
			due = append(due, site)
		}
	}
	if len(due) == 0 {
		return nil, nil
	}

	results := make(map[string]*models.AgentWPData, len(due))
	runner, err := newRunner()
	if err != nil {
		// Report why, so jman-api can show it, rather than going quiet.
		verb.LogPrintf(verb.Normal, "WordPress data collection disabled: %v", err)
		ids := make([]string, len(due))
		for i, site := range due {
			results[site.SiteID] = &models.AgentWPData{CollectedAt: now.UTC().Format(time.RFC3339), Error: err.Error()}
			ids[i] = site.SiteID
		}
		return results, ids
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, wpDataConcurrency)
	for _, site := range due {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			data := collectSiteWPData(ctx, runner, site, manifest.CollectUsers)
			mu.Lock()
			results[site.SiteID] = data
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Cap the full payloads by count and size; capped sites are left out
	// entirely and stay due, so the next cycle collects and sends them.
	// The first payload always goes, so no site can be held back forever.
	payloads, size := 0, 0
	var done []string
	for _, site := range due {
		data := results[site.SiteID]
		if data.Plugins != nil || data.Core != nil {
			n := payloadSize(data)
			if payloads == maxWPDataPayloads || (payloads > 0 && size+n > maxWPDataPayloadBytes) {
				delete(results, site.SiteID)
				continue
			}
			payloads++
			size += n
		}
		done = append(done, site.SiteID)
	}
	return results, done
}

// collectSiteWPData collects one site's plugins, core state and users as its
// owner. The data is included only if its hash differs from jman-api's.
// Users are collected only if collectUsers (the manifest's CollectUsers).
func collectSiteWPData(ctx context.Context, runner wpDataRunner, site models.AgentManifestSite, collectUsers bool) *models.AgentWPData {
	data := &models.AgentWPData{CollectedAt: time.Now().UTC().Format(time.RFC3339)}
	fail := func(err error) *models.AgentWPData {
		verb.LogPrintf(verb.Normal, "Failed to collect WordPress data for %s: %v", site.Domain, err)
		data.Error = err.Error()
		return data
	}

	sitePath, err := resolvePath(site.Domain, site.SiteUser)
	if err != nil {
		return fail(err)
	}
	id, err := resolveIdentity(sitePath, site.SiteUser)
	if err != nil {
		return fail(err)
	}

	// Listed with the plugins loaded so premium plugins report their
	// updates, as an administrator, since some (FileBird Pro) check for
	// updates only when current_user_can('manage_options'), and with the
	// admin-only update checks of others (Yoast add-ons) registered by
	// runWithPlugins. If a
	// broken plugin (or a since-deleted administrator) stops that, listed
	// again with plugins skipped (see wpcli.SkippedPluginListArgs).
	fields := []string{wpcli.PluginListFields}
	listArgs := []string{"plugin", "list", wpcli.PluginListFields, "--format=json"}
	if admin := siteAdminUser(ctx, runner, id, sitePath, site); admin > 0 {
		listArgs = append(listArgs, fmt.Sprintf("--user=%d", admin))
	}
	out, err := runner.runWithPlugins(ctx, id, sitePath, listArgs...)
	if err != nil && wpcli.IsInvalidFieldError(err.Error()) {
		// An older wp-cli: its default fields, still with plugins loaded.
		fields = nil
		listArgs = slices.DeleteFunc(listArgs, func(a string) bool { return a == wpcli.PluginListFields })
		out, err = runner.runWithPlugins(ctx, id, sitePath, listArgs...)
	}
	if err != nil {
		forgetAdminUser(site.SiteID)
		verb.LogPrintf(verb.Normal, "Failed to list plugins for %s with plugins loaded, retrying with them skipped: %v", site.Domain, err)
		if out, err = runner.run(ctx, id, sitePath, wpcli.SkippedPluginListArgs(fields...)...); err != nil {
			return fail(err)
		}
	}
	plugins, err := wpcli.ParsePluginList(out, "")
	if err != nil {
		return fail(fmt.Errorf("failed to parse the plugin list: %w", err))
	}
	if plugins == nil {
		plugins = []models.WPPlugin{}
	}

	// `wp core version` only reads wp-includes/version.php, so it's read
	// here without starting PHP; wp-cli is the fallback for a site where
	// that doesn't work.
	version, locale := readSiteCoreVersion(sitePath)
	if version == "" {
		if out, err = runner.run(ctx, id, sitePath, "core", "version"); err != nil {
			return fail(err)
		}
		version = lastLine(out)
	}
	core := models.SiteCore{Version: version}
	if core.Version == "" {
		return fail(fmt.Errorf("wp-cli reported no WordPress version"))
	}
	// A failed update check still leaves the installed state worth
	// reporting, as the SSH refresh does.
	if updates, err := coreUpdates(ctx, runner, id, sitePath, version, locale); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to check core updates for %s: %v", site.Domain, err)
	} else {
		core.MinorUpdate, core.MajorUpdate = wpcli.SplitCoreUpdates(updates)
	}

	// Users that can't be listed leave the rest worth reporting; jman-api
	// then drops the site's stored users, as it would for an agent that
	// doesn't collect them.
	var users *models.SiteUsers
	if collectUsers {
		if users, err = collectSiteUsers(ctx, runner, id, sitePath); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to collect users for %s: %v", site.Domain, err)
		}
	}

	data.Hash = models.WPDataHash(plugins, core, users)
	if data.Hash != site.WPDataHash {
		data.Plugins = plugins
		data.Core = &core
		data.Users = users
	}
	return data
}

// coreUpdatesTTL is how long the core updates found for a WordPress
// version and locale are reused for other sites on the server.
const coreUpdatesTTL = time.Hour

type cachedCoreUpdates struct {
	updates []wpcli.CoreUpdate
	at      time.Time
}

// coreUpdatesCache holds the core updates found per version and locale.
// `wp core check-update` starts WordPress and asks api.wordpress.org, and
// its answer depends only on the installed version and package locale,
// so sites running the same ones share it.
var (
	coreUpdatesCache   = map[string]cachedCoreUpdates{}
	coreUpdatesCacheMu sync.Mutex
)

// coreUpdates returns the core updates available for version, checking on
// the site unless another site with the same version and locale was
// checked within coreUpdatesTTL. Without a known locale it always checks.
func coreUpdates(ctx context.Context, runner wpDataRunner, id siteIdentity, sitePath, version, locale string) ([]wpcli.CoreUpdate, error) {
	key := version + "|" + locale
	if locale != "" {
		coreUpdatesCacheMu.Lock()
		c, ok := coreUpdatesCache[key]
		coreUpdatesCacheMu.Unlock()
		if ok && time.Since(c.at) < coreUpdatesTTL {
			return c.updates, nil
		}
	}
	out, err := runner.run(ctx, id, sitePath, "core", "check-update", "--format=json")
	if err != nil {
		return nil, err
	}
	updates, err := wpcli.ParseCoreUpdates(out)
	if err != nil {
		return nil, fmt.Errorf("failed to parse core updates: %w", err)
	}
	if locale != "" {
		coreUpdatesCacheMu.Lock()
		coreUpdatesCache[key] = cachedCoreUpdates{updates: updates, at: time.Now()}
		coreUpdatesCacheMu.Unlock()
	}
	return updates, nil
}

// adminUserTTL is how long a site's administrator user ID is reused before
// it's looked up again, as on the SSH side (wpcli.AdminUsers).
const adminUserTTL = 24 * time.Hour

type cachedAdminUser struct {
	id int
	at time.Time
}

// adminUsers caches each site's administrator user ID in memory; after a
// restart it's looked up again on the first collection.
var (
	adminUsers   = map[string]cachedAdminUser{}
	adminUsersMu sync.Mutex
)

// siteAdminUser returns the site's lowest-numbered administrator user ID,
// cached for adminUserTTL, or 0 (list without --user) if there is none or
// it can't be looked up. A failed lookup reuses an expired ID.
func siteAdminUser(ctx context.Context, runner wpDataRunner, id siteIdentity, sitePath string, site models.AgentManifestSite) int {
	adminUsersMu.Lock()
	cached, ok := adminUsers[site.SiteID]
	adminUsersMu.Unlock()
	if ok && time.Since(cached.at) < adminUserTTL {
		return cached.id
	}

	out, err := runner.run(ctx, id, sitePath, wpcli.AdminUserListArgs()...)
	var admin int
	if err == nil {
		admin, err = wpcli.ParseLowestAdminUserID(out)
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to find an administrator for %s, listing plugins without one: %v", site.Domain, err)
		return cached.id
	}

	adminUsersMu.Lock()
	adminUsers[site.SiteID] = cachedAdminUser{id: admin, at: time.Now()}
	adminUsersMu.Unlock()
	return admin
}

// forgetAdminUser drops a site's cached administrator, so the next
// collection looks it up again.
func forgetAdminUser(siteID string) {
	adminUsersMu.Lock()
	delete(adminUsers, siteID)
	adminUsersMu.Unlock()
}

// collectSiteUsers lists a site's users without the excluded roles, which
// it counts, and reads each listed user's post count and application
// passwords (see wpcli.CollectSiteUsers). Post counts only inform the
// delete checks, which the delete job repeats on the site, and
// application passwords only the security checks in jman-api, so either
// can be missing.
func collectSiteUsers(ctx context.Context, runner wpDataRunner, id siteIdentity, sitePath string) (*models.SiteUsers, error) {
	run := func(args ...string) (string, error) { return runner.run(ctx, id, sitePath, args...) }
	users, warnings, err := wpcli.CollectSiteUsers(run, "")
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		verb.LogPrintf(verb.Normal, "Users at %s: %s", sitePath, w)
	}
	return &users, nil
}

// lastLine returns the last non-empty line of wp-cli output, skipping any
// PHP notices printed before the actual result.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
