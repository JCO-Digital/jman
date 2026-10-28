package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/agent/scan"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// scanConcurrency is how many sites are scanned at once.
const scanConcurrency = 3

// maxScanFindingsPerReport bounds the findings in one report. A site whose
// findings don't fit is left out and stays due, so it goes in a later
// report; a single site with more than this has the rest counted in
// FindingsOmitted.
const maxScanFindingsPerReport = 200

// lastScan holds when each site was last scanned and reported. It lives in
// memory: after a restart every site is scanned once more, which is cheap.
var (
	lastScan   = map[string]time.Time{}
	lastScanMu sync.Mutex
)

func scanDue(siteID string, interval time.Duration, now time.Time) bool {
	lastScanMu.Lock()
	defer lastScanMu.Unlock()
	last, ok := lastScan[siteID]
	return !ok || now.Sub(last) >= interval-time.Minute
}

// scanCanRun is a seam for tests: scanning needs root to start the scan
// as each site's owner.
var scanCanRun = func() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("jman-agent isn't running as root, so it can't scan as each site's owner")
	}
	return nil
}

// checksumSource fetches wordpress.org's checksums of WordPress and plugin
// releases through jman-api.
type checksumSource interface {
	FetchCoreChecksums(ctx context.Context, version, locale string) (*scan.CoreChecksums, error)
	FetchPluginChecksums(ctx context.Context, slug, version string) (*scan.PluginChecksums, error)
}

// checksums is set by Run and CheckScan to their jman-api client; while
// it's nil, core and plugin files aren't checked.
var checksums checksumSource

// checksumUnknownRetry is how long the agent remembers that a release has
// no checksums (a premium or custom plugin, usually) before asking again.
const checksumUnknownRetry = 6 * time.Hour

// maxChecksumCache bounds how many releases' checksums the agent keeps; the
// cache starts over when it's full.
const maxChecksumCache = 2000

// checksumCache keeps the checksums fetched for each release, which don't
// change, and the releases that have none.
type cachedChecksums struct {
	core    *scan.CoreChecksums
	plugin  *scan.PluginChecksums
	unknown time.Time // set when the release has none
}

var (
	checksumCache   = map[string]cachedChecksums{}
	checksumCacheMu sync.Mutex
)

// cachedFetch returns a release's checksums from the cache or fetch. It
// reports false when there are none to check against.
func cachedFetch(key string, fetch func() (cachedChecksums, error)) (cachedChecksums, bool, error) {
	checksumCacheMu.Lock()
	c, ok := checksumCache[key]
	checksumCacheMu.Unlock()
	if ok && (c.unknown.IsZero() || time.Since(c.unknown) < checksumUnknownRetry) {
		return c, c.unknown.IsZero(), nil
	}
	c, err := fetch()
	if errors.Is(err, errChecksumsUnknown) {
		c, err = cachedChecksums{unknown: time.Now()}, nil
	}
	if err != nil {
		return c, false, err
	}
	checksumCacheMu.Lock()
	if len(checksumCache) >= maxChecksumCache {
		checksumCache = map[string]cachedChecksums{}
	}
	checksumCache[key] = c
	checksumCacheMu.Unlock()
	return c, c.unknown.IsZero(), nil
}

// scanOptions returns the walk options for a site whose last baseline is
// prev: the files the content rules flagged then, and the checksums of the
// WordPress and plugin versions it had.
func scanOptions(ctx context.Context, prev *scan.State, domain string) scan.Options {
	var opts scan.Options
	if prev == nil {
		return opts
	}
	opts.Watch = prev.Watched()
	if checksums == nil {
		return opts
	}
	if prev.CoreVersion != "" {
		c, ok, err := cachedFetch("core|"+prev.CoreVersion+"|"+prev.CoreLocale, func() (cachedChecksums, error) {
			sums, err := checksums.FetchCoreChecksums(ctx, prev.CoreVersion, prev.CoreLocale)
			return cachedChecksums{core: sums}, err
		})
		if err != nil {
			verb.LogPrintf(verb.Verbose, "%s: not checking core files: %v", domain, err)
		} else if ok {
			opts.Core = c.core
		}
	}
	for slug, version := range prev.Plugins {
		c, ok, err := cachedFetch("plugin|"+slug+"|"+version, func() (cachedChecksums, error) {
			sums, err := checksums.FetchPluginChecksums(ctx, slug, version)
			return cachedChecksums{plugin: sums}, err
		})
		if err != nil {
			verb.LogPrintf(verb.Verbose, "%s: not checking plugin %s: %v", domain, slug, err)
			continue
		}
		if ok {
			if opts.Plugins == nil {
				opts.Plugins = map[string]*scan.PluginChecksums{}
			}
			opts.Plugins[slug] = c.plugin
		}
	}
	return opts
}

// scanDone is a site whose scan goes in this cycle's report. Its baseline
// (nil for a failed scan) is saved once the report has been sent.
type scanDone struct {
	siteID string
	state  *scan.State
}

// collectScansForSites scans the due sites in the manifest and returns the
// scans by site ID, plus what to save once the report is sent.
func collectScansForSites(ctx context.Context, stateDir string, manifest *models.AgentManifest, now time.Time) (map[string]*models.AgentScan, []scanDone) {
	if manifest.ScanIntervalMinutes <= 0 {
		return nil, nil
	}
	interval := time.Duration(manifest.ScanIntervalMinutes) * time.Minute

	var due []models.AgentManifestSite
	for _, site := range manifest.Sites {
		if scanDue(site.SiteID, interval, now) {
			due = append(due, site)
		}
	}
	if len(due) == 0 {
		return nil, nil
	}

	results := make(map[string]*models.AgentScan, len(due))
	states := make(map[string]*scan.State, len(due))
	if err := scanCanRun(); err != nil {
		verb.LogPrintf(verb.Normal, "File scanning disabled: %v", err)
		done := make([]scanDone, len(due))
		for i, site := range due {
			results[site.SiteID] = &models.AgentScan{ScannedAt: now.UTC().Format(time.RFC3339), RuleSetVersion: scan.RuleSetVersion, Error: err.Error()}
			done[i] = scanDone{siteID: site.SiteID}
		}
		return results, done
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, scanConcurrency)
	for _, site := range due {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, state := scanSite(ctx, stateDir, site, time.Duration(manifest.ContentScanIntervalMinutes)*time.Minute)
			mu.Lock()
			results[site.SiteID] = result
			states[site.SiteID] = state
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Cap the findings; sites that don't fit are left out and stay due.
	// The first site with findings always goes, so none is held back
	// forever.
	total := 0
	var done []scanDone
	for _, site := range due {
		result := results[site.SiteID]
		if n := len(result.Findings); n > 0 {
			if total > 0 && total+n > maxScanFindingsPerReport {
				delete(results, site.SiteID)
				continue
			}
			if n > maxScanFindingsPerReport {
				result.FindingsOmitted = n - maxScanFindingsPerReport
				result.Findings = result.Findings[:maxScanFindingsPerReport]
			}
			total += len(result.Findings)
		}
		done = append(done, scanDone{siteID: site.SiteID, state: states[site.SiteID]})
	}
	return results, done
}

// contentWindowEnd is the hour (server time) before which daily content
// scans run.
const contentWindowEnd = 6

// contentDue reports whether a site's scan should search all its PHP files
// for malicious code: once every interval, and for daily or rarer scans
// only at night.
func contentDue(prev *scan.State, interval time.Duration, now time.Time) bool {
	if interval <= 0 {
		return false
	}
	if prev != nil && prev.ContentScannedAt != "" {
		// An hour's slack keeps a daily scan at about the same time of night.
		if t, err := time.Parse(time.RFC3339, prev.ContentScannedAt); err == nil && now.Sub(t) < interval-time.Hour {
			return false
		}
	}
	return interval < 24*time.Hour || now.Hour() < contentWindowEnd
}

// scanSite scans one site as its owner and compares the result with its
// baseline, searching all its PHP files for malicious code when that's due
// (see contentDue). It returns the scan to report and the new baseline,
// which is nil when the scan failed.
func scanSite(ctx context.Context, stateDir string, site models.AgentManifestSite, contentInterval time.Duration) (*models.AgentScan, *scan.State) {
	start := time.Now()
	result := &models.AgentScan{ScannedAt: start.UTC().Format(time.RFC3339), RuleSetVersion: scan.RuleSetVersion}
	fail := func(err error) (*models.AgentScan, *scan.State) {
		verb.LogPrintf(verb.Normal, "Failed to scan %s: %v", site.Domain, err)
		result.Error = err.Error()
		return result, nil
	}

	sitePath, err := resolvePath(site.Domain, site.SiteUser)
	if err != nil {
		return fail(err)
	}
	id, err := resolveIdentity(sitePath, site.SiteUser)
	if err != nil {
		return fail(err)
	}
	prev, err := scan.LoadState(stateDir, site.SiteID)
	if err != nil {
		// A baseline that can't be read is replaced, and jman-api hears
		// about it as a re-baseline.
		verb.LogPrintf(verb.Normal, "Discarding the scan baseline of %s: %v", site.Domain, err)
		prev = nil
	}

	opts := scanOptions(ctx, prev, site.Domain)
	opts.Content = contentDue(prev, contentInterval, start)
	opts.Home = homeOf(id)
	walk, err := runScan(ctx, id, sitePath, opts)
	if err != nil {
		return fail(err)
	}
	if walk.CoreNote != "" {
		verb.LogPrintf(verb.Verbose, "%s: core files not checked: %s", site.Domain, walk.CoreNote)
	}
	for _, note := range walk.PluginNotes {
		verb.LogPrintf(verb.Verbose, "%s: plugin not checked: %s", site.Domain, note)
	}
	scan.CarryChecksums(prev, walk)
	addUserEntries(walk, id)
	scan.SeedUserEntries(prev, walk.Entries)
	state := scan.NewState(walk.Entries)
	state.UserChecked = true
	state.CoreVersion, state.CoreLocale, state.Plugins = walk.CoreVersion, walk.CoreLocale, walk.Plugins
	switch {
	case walk.ContentChecked:
		state.ContentScannedAt = start.UTC().Format(time.RFC3339)
	case prev != nil:
		state.ContentScannedAt = prev.ContentScannedAt
	}

	result.Fingerprint = state.Fingerprint
	result.Entries = len(walk.Entries)
	result.Omitted = walk.Omitted
	if prev == nil {
		result.Rebaselined = true
		result.Findings = scan.EvaluateFirst(walk.Entries, id.UID, userName)
	} else {
		result.PrevFingerprint = prev.Fingerprint
		result.Findings = scan.Evaluate(prev.Entries, walk.Entries, id.UID, userName)
		result.Cleared = clearedSince(prev, walk, id, sitePath)
	}
	result.DurationMs = time.Since(start).Milliseconds()
	if len(result.Findings) > 0 || result.Rebaselined {
		verb.LogPrintf(verb.Verbose, "%s: scanned %d file(s), %d finding(s), rebaselined=%t", site.Domain, result.Entries, len(result.Findings), result.Rebaselined)
	}
	return result, state
}

// maxScanCleared bounds the cleared paths one site's scan reports.
const maxScanCleared = 1000

// clearedSince returns what the site's previous baseline flagged that no
// longer holds. The agent checks, as root, whether a path the walk no
// longer records is still there; the result only words the report.
func clearedSince(prev *scan.State, walk *scan.Result, id siteIdentity, sitePath string) []models.ScanCleared {
	exists := func(p string) bool {
		if scan.IsUserPath(p) {
			ok, err := scan.UserPathExists(p, homeOf(id), id.Username)
			return err == nil && ok
		}
		_, err := os.Lstat(filepath.Join(sitePath, filepath.FromSlash(p)))
		return err == nil
	}
	cleared := scan.Cleared(prev.Entries, walk.Entries, id.UID, walk.Omitted == 0, exists)
	if len(cleared) > maxScanCleared {
		cleared = cleared[:maxScanCleared]
	}
	return cleared
}

// homeOf returns the site user's home directory for the walk, or "" when
// it has none worth walking.
func homeOf(id siteIdentity) string {
	if id.Home == "" || id.Home == "/" {
		return ""
	}
	return id.Home
}

// addUserEntries adds what the agent reads as root about the site user to a
// walk: its crontab, and processes that hide what they are.
func addUserEntries(walk *scan.Result, id siteIdentity) {
	walk.Entries = append(walk.Entries, readUserEntries(id)...)
}

// readUserEntries is a seam for tests, which run as a user whose processes
// (the test binary, run from /tmp) would be flagged.
var readUserEntries = func(id siteIdentity) []scan.Entry {
	var entries []scan.Entry
	if e, ok := scan.ReadCrontab(id.Username, id.UID); ok {
		entries = append(entries, e)
	}
	return append(entries, scan.UserProcesses(id.UID, homeOf(id))...)
}

// saveScans saves the baselines of the reported scans and marks their
// sites scanned.
func saveScans(stateDir string, done []scanDone, at time.Time) {
	lastScanMu.Lock()
	defer lastScanMu.Unlock()
	for _, d := range done {
		lastScan[d.siteID] = at
		if d.state == nil {
			continue
		}
		if err := scan.SaveState(stateDir, d.siteID, d.state); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to save the scan baseline of site %s: %v", d.siteID, err)
		}
	}
}

// userName names a uid for findings.
func userName(uid uint32) string {
	id := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(id); err == nil {
		return fmt.Sprintf("%s (uid %s)", u.Username, id)
	}
	return "uid " + id
}
