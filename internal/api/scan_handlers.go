package api

import (
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Limits on what an agent may report in one site's scan.
const (
	maxAgentScanFindings  = 1000
	maxScanFindingSnippet = 2048
	maxScanFindingDetail  = 2048
	maxScanFindingPath    = 4096
)

var validScanSeverities = map[string]bool{"low": true, "medium": true, "high": true, "critical": true}

// ruleBaselineReset is the finding jman-api raises itself when an agent's
// baseline for a site was lost or isn't the one it last reported.
const ruleBaselineReset = "baseline-reset"

// truncateUTF8 shortens s to at most max bytes without splitting a rune.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// applyAgentScan stores a site's file scan reported by jman-agent: its
// findings, a baseline-reset finding when the agent's baseline isn't the
// one jman-api last stored, and the scan itself.
func applyAgentScan(siteID string, s *models.AgentScan, receivedAt time.Time) {
	scannedAt := receivedAt
	if t, err := time.Parse(time.RFC3339, s.ScannedAt); err == nil {
		scannedAt = t
	}

	if s.Error == "" {
		stored, scanned, err := db.GetSiteScanFingerprint(siteID)
		if err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		} else if scanned {
			var detail string
			switch {
			case s.Rebaselined:
				detail = "The agent had no baseline for the site and recorded a new one, so changes since the last scan weren't checked."
			case s.PrevFingerprint != stored:
				detail = "The agent compared against a baseline other than the one it last reported, so it was reset or changed on the server."
			}
			if detail != "" {
				// The new fingerprint keeps every reset a finding of its own.
				reset := models.ScanFinding{Engine: "api", Rule: ruleBaselineReset, Severity: "low", SHA256: s.Fingerprint, Detail: detail}
				if err := db.UpsertScanFinding(siteID, reset, s.RuleSetVersion, scannedAt); err != nil {
					verb.LogPrintf(verb.Normal, "%v", err)
				}
			}
		}

		findings := s.Findings
		if len(findings) > maxAgentScanFindings {
			verb.LogPrintf(verb.Normal, "Agent reported %d scan findings for site %s; keeping the first %d", len(findings), siteID, maxAgentScanFindings)
			findings = findings[:maxAgentScanFindings]
		}
		for _, f := range findings {
			if f.Engine == "api" || f.Rule == "" || !validScanSeverities[f.Severity] {
				verb.LogPrintf(verb.Normal, "Dropped invalid scan finding for site %s: engine=%q rule=%q severity=%q", siteID, f.Engine, f.Rule, f.Severity)
				continue
			}
			f.Path = truncateUTF8(f.Path, maxScanFindingPath)
			f.Snippet = truncateUTF8(f.Snippet, maxScanFindingSnippet)
			f.Detail = truncateUTF8(f.Detail, maxScanFindingDetail)
			if err := db.UpsertScanFinding(siteID, f, s.RuleSetVersion, scannedAt); err != nil {
				verb.LogPrintf(verb.Normal, "%v", err)
			}
		}
		if s.FindingsOmitted > 0 {
			verb.LogPrintf(verb.Normal, "Agent left %d scan finding(s) for site %s out of its report", s.FindingsOmitted, siteID)
		}
		triageScanFindings(siteID, true)
	}

	if err := db.RecordSiteScan(siteID, s, scannedAt); err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
	}
}

// ListScanFindingsHandler handles GET /api/scan/findings: the findings of
// file scans, most recently seen first. ?site_id=, ?incident_id=, ?status=
// and ?sha256= filter them, ?unassigned=1 keeps the ones without an
// incident, and ?limit= (default 200, at most 1000) caps how many are
// returned.
func ListScanFindingsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := db.ScanFindingFilter{
		SiteID:     q.Get("site_id"),
		Status:     q.Get("status"),
		SHA256:     q.Get("sha256"),
		Unassigned: q.Get("unassigned") == "1" || q.Get("unassigned") == "true",
		Limit:      200,
	}
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		filter.Limit = min(l, 1000)
	}
	if v := q.Get("incident_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid incident_id")
			return
		}
		filter.IncidentID = id
	}
	findings, err := db.ListScanFindings(filter)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListScanFindingsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"findings": findings})
}

// ListSiteScansHandler handles GET /api/scan/sites: the latest file scan
// of every scanned site.
func ListSiteScansHandler(w http.ResponseWriter, r *http.Request) {
	scans, err := db.ListSiteScans()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListSiteScansHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"scans": scans})
}
