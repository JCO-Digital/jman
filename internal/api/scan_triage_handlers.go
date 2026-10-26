package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Limits on triage requests.
const (
	maxScanTriageFindings = 500
	maxScanTriageReason   = 1000
)

// scanTriageRequest is the body of the triage endpoints that act on
// findings.
type scanTriageRequest struct {
	FindingIDs []int64 `json:"finding_ids"`
	Reason     string  `json:"reason"`
}

// decodeScanTriage reads a triage request, writing an error and returning
// nil findings when it's invalid. needReason requires a reason.
func decodeScanTriage(w http.ResponseWriter, r *http.Request, needReason bool) (*scanTriageRequest, []models.StoredScanFinding) {
	var req scanTriageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return nil, nil
	}
	req.Reason = strings.TrimSpace(req.Reason)
	switch {
	case len(req.FindingIDs) == 0:
		WriteError(w, http.StatusBadRequest, "finding_ids is required")
		return nil, nil
	case len(req.FindingIDs) > maxScanTriageFindings:
		WriteError(w, http.StatusBadRequest, "Too many findings")
		return nil, nil
	case needReason && req.Reason == "":
		WriteError(w, http.StatusBadRequest, "A reason is required")
		return nil, nil
	case len(req.Reason) > maxScanTriageReason:
		WriteError(w, http.StatusBadRequest, "The reason is too long")
		return nil, nil
	}
	findings, err := db.GetScanFindings(req.FindingIDs)
	if err != nil {
		verb.LogPrintf(verb.Normal, "decodeScanTriage: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return nil, nil
	}
	if len(findings) != len(uniqueIDs(req.FindingIDs)) {
		WriteError(w, http.StatusNotFound, "Finding not found")
		return nil, nil
	}
	return &req, findings
}

func uniqueIDs(ids []int64) map[int64]bool {
	set := make(map[int64]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// findingIncidents returns the incidents the findings belong to.
func findingIncidents(findings []models.StoredScanFinding) map[int64]bool {
	ids := map[int64]bool{}
	for _, f := range findings {
		if f.IncidentID != nil {
			ids[*f.IncidentID] = true
		}
	}
	return ids
}

func findingIDs(findings []models.StoredScanFinding) []int64 {
	ids := make([]int64, len(findings))
	for i, f := range findings {
		ids[i] = f.ID
	}
	return ids
}

// CloseScanFindingsHandler handles POST /api/scan/findings/close: it
// dismisses open findings. A finding reported again opens again. An
// incident left without open findings is closed.
func CloseScanFindingsHandler(w http.ResponseWriter, r *http.Request) {
	req, findings := decodeScanTriage(w, r, false)
	if req == nil {
		return
	}
	by := requestUsername(r)

	scanTriageMu.Lock()
	defer scanTriageMu.Unlock()
	n, err := db.SetScanFindingStatus(findingIDs(findings), models.ScanFindingClosed, by)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CloseScanFindingsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	closeEmptySecurityIncidents(findingIncidents(findings), by)
	verb.LogPrintf(verb.Normal, "%s closed %d scan finding(s)", by, n)
	WriteJSON(w, http.StatusOK, map[string]int{"closed": n})
}

// addScanAllowlistRequest is the body of POST /api/scan/allowlist.
type addScanAllowlistRequest struct {
	FindingID int64  `json:"finding_id"`
	Reason    string `json:"reason"`
	// CloseMatching also marks the open findings with the same hash on
	// other sites allowlisted.
	CloseMatching bool `json:"close_matching"`
}

// AddScanAllowlistHandler handles POST /api/scan/allowlist: it trusts the
// file hash of a finding on every site and marks that finding allowlisted
// (with close_matching, every open finding with the hash). An incident
// left without open findings is closed.
func AddScanAllowlistHandler(w http.ResponseWriter, r *http.Request) {
	var req addScanAllowlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		WriteError(w, http.StatusBadRequest, "A reason is required")
		return
	}
	if len(req.Reason) > maxScanTriageReason {
		WriteError(w, http.StatusBadRequest, "The reason is too long")
		return
	}
	found, err := db.GetScanFindings([]int64{req.FindingID})
	if err != nil {
		verb.LogPrintf(verb.Normal, "AddScanAllowlistHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if len(found) == 0 {
		WriteError(w, http.StatusNotFound, "Finding not found")
		return
	}
	f := found[0]
	if f.SHA256 == "" || f.Engine == "api" {
		WriteError(w, http.StatusUnprocessableEntity, "This finding has no file hash to allowlist")
		return
	}
	by := requestUsername(r)

	scanTriageMu.Lock()
	defer scanTriageMu.Unlock()
	entry, err := db.AddScanAllowlistEntry(models.ScanAllowlistEntry{
		SHA256:      f.SHA256,
		Reason:      req.Reason,
		SiteID:      f.SiteID,
		Rule:        f.Rule,
		Path:        f.Path,
		Snippet:     f.Snippet,
		AuditFields: models.AuditFields{CreatedBy: by},
	})
	if err != nil {
		verb.LogPrintf(verb.Normal, "AddScanAllowlistHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	affected := []models.StoredScanFinding{f}
	if req.CloseMatching {
		matching, err := db.ListScanFindings(db.ScanFindingFilter{SHA256: f.SHA256, Status: models.ScanFindingOpen})
		if err != nil {
			verb.LogPrintf(verb.Normal, "AddScanAllowlistHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		affected = append(affected, matching...)
	}
	n, err := db.SetScanFindingStatus(findingIDs(affected), models.ScanFindingAllowlisted, by)
	if err != nil {
		verb.LogPrintf(verb.Normal, "AddScanAllowlistHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	closeEmptySecurityIncidents(findingIncidents(affected), by)
	verb.LogPrintf(verb.Normal, "%s allowlisted %s (%s on site %s): %s", by, f.SHA256, f.Path, f.SiteID, req.Reason)
	WriteJSON(w, http.StatusOK, map[string]any{"entry": entry, "allowlisted": n})
}

// ListScanAllowlistHandler handles GET /api/scan/allowlist, with removed
// entries when ?include_removed=1.
func ListScanAllowlistHandler(w http.ResponseWriter, r *http.Request) {
	entries, err := db.ListScanAllowlist(r.URL.Query().Get("include_removed") == "1")
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListScanAllowlistHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// RemoveScanAllowlistHandler handles DELETE /api/scan/allowlist/{id}. The
// entry is kept for the record; findings it already covered stay
// allowlisted.
func RemoveScanAllowlistHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid allowlist entry ID")
		return
	}
	by := requestUsername(r)
	removed, err := db.RemoveScanAllowlistEntry(id, by)
	if err != nil {
		verb.LogPrintf(verb.Normal, "RemoveScanAllowlistHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !removed {
		WriteError(w, http.StatusNotFound, "Allowlist entry not found")
		return
	}
	verb.LogPrintf(verb.Normal, "%s removed allowlist entry %d", by, id)
	w.WriteHeader(http.StatusNoContent)
}

// AddScanPathAcksHandler handles POST /api/scan/acks: it acknowledges the
// paths of stray, foreign-owned or unreadable files on their sites, or
// administrators jman didn't create (new-admin, path "users/<login>"), so
// those rules don't flag them again, and marks the findings acknowledged.
// An incident left without open findings is closed.
func AddScanPathAcksHandler(w http.ResponseWriter, r *http.Request) {
	req, findings := decodeScanTriage(w, r, true)
	if req == nil {
		return
	}
	for _, f := range findings {
		if !scanStateRules[f.Rule] {
			WriteError(w, http.StatusUnprocessableEntity, "Only stray-file, foreign-owner, unreadable and new-admin findings can be acknowledged by path; allowlist the file's hash instead")
			return
		}
	}
	by := requestUsername(r)

	scanTriageMu.Lock()
	defer scanTriageMu.Unlock()
	acks := []models.ScanPathAck{}
	for _, f := range findings {
		ack, err := db.AddScanPathAck(models.ScanPathAck{
			SiteID:      f.SiteID,
			Rule:        f.Rule,
			Path:        f.Path,
			Reason:      req.Reason,
			AuditFields: models.AuditFields{CreatedBy: by},
		})
		if err != nil {
			verb.LogPrintf(verb.Normal, "AddScanPathAcksHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		acks = append(acks, *ack)
	}
	n, err := db.SetScanFindingStatus(findingIDs(findings), models.ScanFindingAcknowledged, by)
	if err != nil {
		verb.LogPrintf(verb.Normal, "AddScanPathAcksHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	closeEmptySecurityIncidents(findingIncidents(findings), by)
	verb.LogPrintf(verb.Normal, "%s acknowledged %d scan finding path(s): %s", by, len(acks), req.Reason)
	WriteJSON(w, http.StatusOK, map[string]any{"acks": acks, "acknowledged": n})
}

// ListScanPathAcksHandler handles GET /api/scan/acks: path
// acknowledgements, for one site with ?site_id=, with removed ones when
// ?include_removed=1.
func ListScanPathAcksHandler(w http.ResponseWriter, r *http.Request) {
	acks, err := db.ListScanPathAcks(r.URL.Query().Get("site_id"), r.URL.Query().Get("include_removed") == "1")
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListScanPathAcksHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"acks": acks})
}

// RemoveScanPathAckHandler handles DELETE /api/scan/acks/{id}. The
// acknowledgement is kept for the record.
func RemoveScanPathAckHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid acknowledgement ID")
		return
	}
	by := requestUsername(r)
	removed, err := db.RemoveScanPathAck(id, by)
	if err != nil {
		verb.LogPrintf(verb.Normal, "RemoveScanPathAckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !removed {
		WriteError(w, http.StatusNotFound, "Acknowledgement not found")
		return
	}
	verb.LogPrintf(verb.Normal, "%s removed path acknowledgement %d", by, id)
	w.WriteHeader(http.StatusNoContent)
}
