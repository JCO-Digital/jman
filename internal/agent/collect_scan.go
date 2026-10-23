package agent

import (
	"context"
	"fmt"
	"os"
	"os/user"
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
			result, state := scanSite(ctx, stateDir, site)
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

// scanSite scans one site as its owner and compares the result with its
// baseline. It returns the scan to report and the new baseline, which is
// nil when the scan failed.
func scanSite(ctx context.Context, stateDir string, site models.AgentManifestSite) (*models.AgentScan, *scan.State) {
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

	walk, err := runScan(ctx, id, sitePath)
	if err != nil {
		return fail(err)
	}
	state := scan.NewState(walk.Entries)

	result.Fingerprint = state.Fingerprint
	result.Entries = len(walk.Entries)
	result.Omitted = walk.Omitted
	if prev == nil {
		result.Rebaselined = true
	} else {
		result.PrevFingerprint = prev.Fingerprint
		result.Findings = scan.Evaluate(prev.Entries, walk.Entries, id.UID, userName)
	}
	result.DurationMs = time.Since(start).Milliseconds()
	if len(result.Findings) > 0 || result.Rebaselined {
		verb.LogPrintf(verb.Verbose, "%s: scanned %d file(s), %d finding(s), rebaselined=%t", site.Domain, result.Entries, len(result.Findings), result.Rebaselined)
	}
	return result, state
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
