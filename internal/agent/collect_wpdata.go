package agent

import (
	"context"
	"encoding/json"
	"fmt"
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
}

// resolvePath and resolveIdentity are seams for tests.
var (
	resolvePath     = ResolveSitePath
	resolveIdentity = resolveSiteIdentity
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

	out, err := runner.run(ctx, id, sitePath, "plugin", "list", "--format=json")
	if err != nil {
		return fail(err)
	}
	plugins, err := wpcli.ParsePluginList(out, "")
	if err != nil {
		return fail(fmt.Errorf("failed to parse the plugin list: %w", err))
	}
	if plugins == nil {
		plugins = []models.WPPlugin{}
	}

	out, err = runner.run(ctx, id, sitePath, "core", "version")
	if err != nil {
		return fail(err)
	}
	core := models.SiteCore{Version: lastLine(out)}
	if core.Version == "" {
		return fail(fmt.Errorf("wp-cli reported no WordPress version"))
	}
	// A failed update check still leaves the installed state worth
	// reporting, as the SSH refresh does.
	if out, err := runner.run(ctx, id, sitePath, "core", "check-update", "--format=json"); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to check core updates for %s: %v", site.Domain, err)
	} else if updates, err := wpcli.ParseCoreUpdates(out); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to parse core updates for %s: %v", site.Domain, err)
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

// collectSiteUsers lists a site's users without the excluded roles, which
// it counts, and counts each listed user's posts.
func collectSiteUsers(ctx context.Context, runner wpDataRunner, id siteIdentity, sitePath string) (*models.SiteUsers, error) {
	out, err := runner.run(ctx, id, sitePath, wpcli.UserListArgs()...)
	if err != nil {
		return nil, err
	}
	list, err := wpcli.ParseUserList(out, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse the user list: %w", err)
	}
	counts := make(map[string]int, len(wpcli.ExcludedUserRoles))
	for _, role := range wpcli.ExcludedUserRoles {
		out, err := runner.run(ctx, id, sitePath, wpcli.UserCountArgs(role)...)
		if err != nil {
			return nil, err
		}
		if counts[role], err = wpcli.ParseUserCount(out); err != nil {
			return nil, err
		}
	}
	users := wpcli.NewSiteUsers(list, counts)
	// Post counts only inform the delete checks, which the delete job
	// repeats on the site, so a failure leaves them unknown.
	out, err = runner.run(ctx, id, sitePath, "eval", wpcli.UserPostCountsEval)
	if err == nil {
		var posts map[int]int
		if posts, err = wpcli.ParseUserPostCounts(out); err == nil {
			wpcli.SetPostCounts(users.Users, posts)
		}
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to count posts per user at %s: %v", sitePath, err)
	}
	return &users, nil
}

// lastLine returns the last non-empty line of wp-cli output, skipping any
// PHP notices printed before the actual result.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
