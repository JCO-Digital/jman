package agent

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/agent/scan"
	"github.com/JCO-Digital/jman/internal/models"
)

// maxCheckScanFindings caps how many findings CheckScan prints per site.
const maxCheckScanFindings = 50

// CheckScan scans this server's sites once and prints what the rules
// flag, without reporting anything to jman-api, self-updating or changing
// any baseline. A site without a baseline gets what its first scan would
// report. It ignores the manifest's scan interval. domain limits it to one
// site. It returns how many sites failed.
func CheckScan(ctx context.Context, cfg Config, domain string, out io.Writer) (int, error) {
	manifest, err := NewClient(cfg).FetchManifest(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	var sites []models.AgentManifestSite
	for _, site := range manifest.Sites {
		if domain == "" || strings.EqualFold(site.Domain, domain) {
			sites = append(sites, site)
		}
	}
	if len(sites) == 0 {
		if domain != "" {
			return 0, fmt.Errorf("no site %q in this server's manifest", domain)
		}
		return 0, fmt.Errorf("no sites in this server's manifest")
	}
	if err := scanCanRun(); err != nil {
		return len(sites), fmt.Errorf("file scanning is unavailable: %w", err)
	}
	if manifest.ScanIntervalMinutes == 0 {
		fmt.Fprintln(out, "Note: this jman-api doesn't enable file scanning yet; the service won't scan until it does.")
	}

	failed := 0
	for _, site := range sites {
		fmt.Fprintf(out, "\n%s\n", site.Domain)
		if !checkScanSite(ctx, cfg.StateDir, site, out) {
			failed++
		}
	}
	fmt.Fprintf(out, "\n%d of %d site(s) scanned", len(sites)-failed, len(sites))
	if failed > 0 {
		fmt.Fprintf(out, ", %d failed", failed)
	}
	fmt.Fprintln(out)
	return failed, nil
}

func checkScanSite(ctx context.Context, stateDir string, site models.AgentManifestSite, out io.Writer) bool {
	sitePath, err := resolvePath(site.Domain, site.SiteUser)
	if err != nil {
		fmt.Fprintf(out, "  FAILED: %v\n", err)
		return false
	}
	fmt.Fprintf(out, "  path:    %s\n", sitePath)
	id, err := resolveIdentity(sitePath, site.SiteUser)
	if err != nil {
		fmt.Fprintf(out, "  FAILED: %v\n", err)
		return false
	}
	fmt.Fprintf(out, "  runs as: %s (uid %d, gid %d)\n", id.Username, id.UID, id.GID)

	prev, err := scan.LoadState(stateDir, site.SiteID)
	if err != nil {
		fmt.Fprintf(out, "  baseline unreadable, the service would replace it: %v\n", err)
	}
	start := time.Now()
	walk, err := runScan(ctx, id, sitePath)
	if err != nil {
		fmt.Fprintf(out, "  FAILED: %v\n", err)
		return false
	}
	fmt.Fprintf(out, "  scanned: %d file(s) recorded", len(walk.Entries))
	if walk.Omitted > 0 {
		fmt.Fprintf(out, ", %d omitted", walk.Omitted)
	}
	fmt.Fprintf(out, " in %s\n", time.Since(start).Round(time.Millisecond))

	var findings []models.ScanFinding
	if prev != nil {
		fmt.Fprintln(out, "  compared with the baseline of the last reported scan")
		findings = scan.Evaluate(prev.Entries, walk.Entries, id.UID, userName)
	} else {
		fmt.Fprintln(out, "  no baseline yet: a first scan, which only flags stray, foreign-owned and unreadable files")
		findings = scan.EvaluateFirst(walk.Entries, id.UID, userName)
	}
	if len(findings) == 0 {
		fmt.Fprintln(out, "  ok: nothing flagged")
		return true
	}
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Severity+" "+f.Rule]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		fmt.Fprintf(out, "  %4d %s\n", counts[k], k)
	}
	for i, f := range findings {
		if i == maxCheckScanFindings {
			fmt.Fprintf(out, "  … and %d more\n", len(findings)-i)
			break
		}
		detail, _, _ := strings.Cut(f.Detail, "\n")
		fmt.Fprintf(out, "  [%s] %s %s: %s\n", f.Severity, f.Rule, f.Path, detail)
	}
	return true
}
