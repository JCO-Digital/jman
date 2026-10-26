package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/agent/scan"
	"github.com/JCO-Digital/jman/internal/models"
)

// stubScan makes collectScansForSites scan the directories in roots (by
// domain) in-process, as the test's own user.
func stubScan(t *testing.T, roots map[string]string) {
	t.Helper()
	oldPath, oldIdentity, oldRun, oldCanRun := resolvePath, resolveIdentity, runScan, scanCanRun
	resolvePath = func(domain, _ string) (string, error) {
		if root, ok := roots[domain]; ok {
			return root, nil
		}
		return "", fmt.Errorf("no such site")
	}
	resolveIdentity = func(string, string) (siteIdentity, error) {
		return siteIdentity{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())}, nil
	}
	runScan = func(_ context.Context, id siteIdentity, sitePath string, opts scan.Options) (*scan.Result, error) {
		return scan.Walk(sitePath, id.UID, opts)
	}
	scanCanRun = func() error { return nil }
	lastScanMu.Lock()
	lastScan = map[string]time.Time{}
	lastScanMu.Unlock()
	t.Cleanup(func() { resolvePath, resolveIdentity, runScan, scanCanRun = oldPath, oldIdentity, oldRun, oldCanRun })
}

func writeSiteFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanManifest(domains ...string) *models.AgentManifest {
	m := &models.AgentManifest{ScanIntervalMinutes: 60}
	for _, d := range domains {
		m.Sites = append(m.Sites, models.AgentManifestSite{SiteID: "id-" + d[:1], Domain: d})
	}
	return m
}

func TestScanRebaselinesThenReportsChanges(t *testing.T) {
	root, stateDir := t.TempDir(), t.TempDir()
	writeSiteFile(t, root, "wp-content/uploads/index.php", "")
	stubScan(t, map[string]string{"a.example": root})
	manifest := scanManifest("a.example")
	now := time.Now()

	scans, done := collectScansForSites(context.Background(), stateDir, manifest, now)
	first := scans["id-a"]
	if first == nil || !first.Rebaselined || len(first.Findings) != 0 || first.Fingerprint == "" || first.Entries != 1 {
		t.Fatalf("first scan = %+v, want a re-baseline without findings", first)
	}
	saveScans(stateDir, done, now)

	// Not due again within the interval.
	if scans, _ := collectScansForSites(context.Background(), stateDir, manifest, now.Add(30*time.Minute)); len(scans) != 0 {
		t.Fatalf("scanned again after 30 minutes: %+v", scans)
	}

	writeSiteFile(t, root, "wp-content/uploads/2026/x.php", "<?php eval($_POST['x']);")
	later := now.Add(time.Hour)
	scans, done = collectScansForSites(context.Background(), stateDir, manifest, later)
	second := scans["id-a"]
	if second == nil || second.Rebaselined || second.PrevFingerprint != first.Fingerprint {
		t.Fatalf("second scan = %+v, want one compared against the first", second)
	}
	if len(second.Findings) != 1 || second.Findings[0].Rule != scan.RuleUploadsPHP || second.Findings[0].Snippet == "" {
		t.Fatalf("second scan findings = %+v", second.Findings)
	}
	saveScans(stateDir, done, later)

	// Once saved, the same file isn't flagged again.
	scans, _ = collectScansForSites(context.Background(), stateDir, manifest, later.Add(time.Hour))
	if third := scans["id-a"]; third == nil || len(third.Findings) != 0 || third.PrevFingerprint != second.Fingerprint {
		t.Fatalf("third scan = %+v", third)
	}
}

func TestScanDefersSitesOverTheFindingsBudget(t *testing.T) {
	a, b, stateDir := t.TempDir(), t.TempDir(), t.TempDir()
	stubScan(t, map[string]string{"a.example": a, "b.example": b})
	manifest := scanManifest("a.example", "b.example")
	now := time.Now()
	_, done := collectScansForSites(context.Background(), stateDir, manifest, now)
	saveScans(stateDir, done, now)

	for i := range maxScanFindingsPerReport + 5 {
		writeSiteFile(t, a, fmt.Sprintf("wp-content/uploads/%d.php", i), "<?php")
	}
	writeSiteFile(t, b, "wp-content/uploads/x.php", "<?php")

	later := now.Add(time.Hour)
	scans, done := collectScansForSites(context.Background(), stateDir, manifest, later)
	if got := scans["id-a"]; got == nil || len(got.Findings) != maxScanFindingsPerReport || got.FindingsOmitted != 5 {
		t.Fatalf("site a = %d findings, %d omitted", len(got.Findings), got.FindingsOmitted)
	}
	if _, ok := scans["id-b"]; ok || len(done) != 1 {
		t.Fatalf("site b went in the same report: done = %+v", done)
	}
	saveScans(stateDir, done, later)

	// b is still due and goes next.
	scans, _ = collectScansForSites(context.Background(), stateDir, manifest, later.Add(time.Minute))
	if got := scans["id-b"]; got == nil || len(got.Findings) != 1 {
		t.Fatalf("site b next cycle = %+v", got)
	}
}

func TestScanReportsFailures(t *testing.T) {
	stateDir := t.TempDir()
	stubScan(t, map[string]string{})
	scans, done := collectScansForSites(context.Background(), stateDir, scanManifest("gone.example"), time.Now())
	if got := scans["id-g"]; got == nil || got.Error == "" || len(done) != 1 || done[0].state != nil {
		t.Fatalf("scan of a missing site = %+v, done = %+v", got, done)
	}

	scanCanRun = func() error { return errors.New("not root") }
	lastScan = map[string]time.Time{}
	scans, _ = collectScansForSites(context.Background(), stateDir, scanManifest("gone.example"), time.Now())
	if got := scans["id-g"]; got == nil || got.Error != "not root" {
		t.Fatalf("scan without root = %+v", got)
	}

	if scans, _ := collectScansForSites(context.Background(), stateDir, &models.AgentManifest{Sites: scanManifest("x.example").Sites}, time.Now()); scans != nil {
		t.Fatalf("scanned with scanning off: %+v", scans)
	}
}

func TestRunScanChildPrintsTheWalk(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the scan child refuses to run as root")
	}
	root := t.TempDir()
	writeSiteFile(t, root, "wp-content/uploads/x.php", "<?php")
	var out bytes.Buffer
	if err := RunScanChild(root, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	var res scan.Result
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Path != "wp-content/uploads/x.php" || res.Entries[0].SHA256 == "" {
		t.Fatalf("result = %+v", res)
	}
}

// A first scan records a baseline but still reports what's already wrong.
func TestFirstScanReportsStateRules(t *testing.T) {
	root, stateDir := t.TempDir(), t.TempDir()
	writeSiteFile(t, root, "backup.sql", "dump")
	writeSiteFile(t, root, "wp-content/uploads/x.php", "<?php eval($_POST['x']);")
	writeSiteFile(t, root, "wp-content/mu-plugins/loader.php", "<?php")
	stubScan(t, map[string]string{"a.example": root})

	scans, _ := collectScansForSites(context.Background(), stateDir, scanManifest("a.example"), time.Now())
	first := scans["id-a"]
	if first == nil || !first.Rebaselined || len(first.Findings) != 1 || first.Findings[0].Rule != scan.RuleStrayFile || first.Findings[0].Path != "backup.sql" {
		t.Fatalf("first scan = %+v, want only the stray file", first)
	}
}
