package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func listScanFindings(t *testing.T, query string) []models.StoredScanFinding {
	t.Helper()
	w := httptest.NewRecorder()
	ListScanFindingsHandler(w, httptest.NewRequest("GET", "/api/scan/findings"+query, nil))
	var resp struct {
		Findings []models.StoredScanFinding `json:"findings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("findings: %v: %s", err, w.Body.String())
	}
	return resp.Findings
}

func countRule(findings []models.StoredScanFinding, rule string) int {
	n := 0
	for _, f := range findings {
		if f.Rule == rule {
			n++
		}
	}
	return n
}

func TestAgentScanRoundTrip(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{401, 7, "scan.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(401)

	if m := fetchManifest(t, 7); m.ScanIntervalMinutes != 60 {
		t.Fatalf("manifest scan interval = %d, want 60", m.ScanIntervalMinutes)
	}

	report := func(s models.AgentScan) {
		t.Helper()
		s.RuleSetVersion = "1"
		postAgentReport(t, 7, models.AgentReportSite{SiteID: site, Scan: &s})
	}

	// The first scan only records a baseline.
	report(models.AgentScan{Rebaselined: true, Fingerprint: "fp1", Entries: 3})
	if f := listScanFindings(t, ""); len(f) != 0 {
		t.Fatalf("findings after the first scan: %+v", f)
	}
	scans, err := db.ListSiteScans()
	if err != nil || len(scans) != 1 || scans[0].Entries != 3 || scans[0].RebaselinedAt == nil {
		t.Fatalf("site scans = %+v, %v", scans, err)
	}

	// Findings are stored; invalid ones and ones claiming to be jman-api's
	// are dropped. Reporting the same finding again doesn't duplicate it.
	shell := models.ScanFinding{Engine: "builtin", Rule: "uploads-php", Severity: "high", Path: "wp-content/uploads/x.php", SHA256: "abc", Snippet: "<?php eval("}
	for range 2 {
		report(models.AgentScan{PrevFingerprint: "fp1", Fingerprint: "fp1", Findings: []models.ScanFinding{
			shell,
			{Engine: "api", Rule: "baseline-reset", Severity: "low"},
			{Engine: "builtin", Rule: "stray-file", Severity: "urgent", Path: "x.sql"},
		}})
	}
	findings := listScanFindings(t, "?site_id="+site)
	if len(findings) != 1 || findings[0].Rule != "uploads-php" || findings[0].Snippet != "<?php eval(" || findings[0].RuleSetVersion != "1" {
		t.Fatalf("findings = %+v", findings)
	}

	// A failed scan keeps the stored fingerprint, so the next scan still
	// matches it.
	report(models.AgentScan{Error: "the scan timed out"})
	report(models.AgentScan{PrevFingerprint: "fp1", Fingerprint: "fp2"})
	if n := countRule(listScanFindings(t, ""), ruleBaselineReset); n != 0 {
		t.Fatalf("%d baseline resets after a matching scan", n)
	}

	// A scan against an unexpected baseline, and a lost baseline, are
	// each flagged.
	report(models.AgentScan{PrevFingerprint: "forged", Fingerprint: "fp3"})
	report(models.AgentScan{Rebaselined: true, Fingerprint: "fp4"})
	if n := countRule(listScanFindings(t, ""), ruleBaselineReset); n != 2 {
		t.Fatalf("%d baseline resets, want 2", n)
	}

	if f := listScanFindings(t, "?site_id=someone-else"); len(f) != 0 {
		t.Fatalf("findings for another site: %+v", f)
	}
}
