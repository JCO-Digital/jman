package agent

import (
	"context"
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
			data := collectSiteWPData(ctx, runner, site)
			mu.Lock()
			results[site.SiteID] = data
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Cap the full payloads; capped sites are left out entirely and stay
	// due, so the next cycle collects and sends them.
	payloads := 0
	var done []string
	for _, site := range due {
		data := results[site.SiteID]
		if data.Plugins != nil || data.Core != nil {
			if payloads == maxWPDataPayloads {
				delete(results, site.SiteID)
				continue
			}
			payloads++
		}
		done = append(done, site.SiteID)
	}
	return results, done
}

// collectSiteWPData collects one site's plugins and core state as its
// owner. The data is included only if its hash differs from jman-api's.
func collectSiteWPData(ctx context.Context, runner wpDataRunner, site models.AgentManifestSite) *models.AgentWPData {
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

	data.Hash = models.WPDataHash(plugins, core)
	if data.Hash != site.WPDataHash {
		data.Plugins = plugins
		data.Core = &core
	}
	return data
}

// lastLine returns the last non-empty line of wp-cli output, skipping any
// PHP notices printed before the actual result.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
