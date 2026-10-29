package db

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

// ListPendingScanFindings returns a site's open findings that aren't in an
// active incident: new ones, and ones reported again after their incident
// was closed.
func ListPendingScanFindings(siteID string) ([]models.StoredScanFinding, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT `+scanFindingColumns+` FROM scan_findings
	WHERE site_id = ? AND status = 'open' AND (incident_id IS NULL OR incident_id IN (
		SELECT id FROM incidents WHERE status NOT IN ('open', 'acknowledged')))
	ORDER BY id`, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending scan findings for site %s: %w", siteID, err)
	}
	defer rows.Close()
	var findings []models.StoredScanFinding
	for rows.Next() {
		f, err := scanScanFinding(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan scan finding: %w", err)
		}
		findings = append(findings, f)
	}
	return findings, rows.Err()
}

// ListSitesWithPendingScanFindings returns the sites that have open
// findings without an incident.
func ListSitesWithPendingScanFindings() ([]string, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT DISTINCT site_id FROM scan_findings WHERE status = 'open' AND incident_id IS NULL ORDER BY site_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites with pending scan findings: %w", err)
	}
	defer rows.Close()
	var sites []string
	for rows.Next() {
		var siteID string
		if err := rows.Scan(&siteID); err != nil {
			return nil, err
		}
		sites = append(sites, siteID)
	}
	return sites, rows.Err()
}

// SetScanFindingIncident puts a finding in an incident.
func SetScanFindingIncident(findingID, incidentID int64) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`UPDATE scan_findings SET incident_id = ? WHERE id = ?`, incidentID, findingID); err != nil {
		return fmt.Errorf("failed to assign scan finding %d to incident %d: %w", findingID, incidentID, err)
	}
	return nil
}

// SetScanFindingStatus changes the status of the given findings that are
// open, recording who did it. It returns how many changed.
func SetScanFindingStatus(ids []int64, status, by string) (int, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	args := []any{status, by, time.Now()}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := dbConn.Exec(`UPDATE scan_findings SET status = ?, status_by = ?, status_at = ?
	WHERE status = 'open' AND id IN (?`+strings.Repeat(", ?", len(ids)-1)+`)`, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to set scan finding status: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// CreateSecurityIncident opens a security incident for a site, starting
// when the finding it's opened for was first seen.
func CreateSecurityIncident(siteID, domain string, finding models.StoredScanFinding) (*models.Incident, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	now := time.Now()
	var id int64
	err := dbConn.QueryRow(`
	INSERT INTO incidents (type, site_id, severity, domain, status, error_message, error_code, down_since, pd_triggered, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, '', 0, COALESCE((SELECT first_seen_at FROM scan_findings WHERE id = ?), ?), FALSE, ?, ?)
	RETURNING id`, models.IncidentTypeSecurity, siteID, finding.Severity, domain, models.IncidentStatusOpen, finding.ID, now, now, now).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create security incident for site %s: %w", siteID, err)
	}
	return GetIncidentByID(id)
}

// GetActiveSecurityIncident returns a site's open or acknowledged security
// incident, or nil if it has none.
func GetActiveSecurityIncident(siteID string) (*models.Incident, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	inc, err := scanIncident(dbConn.QueryRow(`SELECT `+incidentColumns+` FROM incidents
	WHERE type = ? AND site_id = ? AND status IN ('open', 'acknowledged')
	ORDER BY id DESC LIMIT 1`, models.IncidentTypeSecurity, siteID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query active security incident for site %s: %w", siteID, err)
	}
	return inc, nil
}

// FindClosedSecurityIncidentByHash returns the most recent closed security
// incident of a site that has a finding with the given file hash, or 0.
func FindClosedSecurityIncidentByHash(siteID, sha256 string) (int64, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	if sha256 == "" {
		return 0, nil
	}
	var id int64
	err := dbConn.QueryRow(`SELECT i.id FROM incidents i JOIN scan_findings f ON f.incident_id = i.id
	WHERE i.type = ? AND i.site_id = ? AND i.status NOT IN ('open', 'acknowledged') AND f.sha256 = ?
	ORDER BY i.id DESC LIMIT 1`, models.IncidentTypeSecurity, siteID, sha256).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to look up closed security incidents for site %s: %w", siteID, err)
	}
	return id, nil
}

// ReopenIncident makes a closed or resolved incident open again, clearing
// who acknowledged and closed it.
func ReopenIncident(id int64) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`UPDATE incidents
	SET status = ?, acknowledged_by = NULL, acknowledged_at = NULL, resolved_by = NULL, resolved_at = NULL,
		pd_triggered = FALSE, updated_at = ?
	WHERE id = ?`, models.IncidentStatusOpen, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to reopen incident %d: %w", id, err)
	}
	return nil
}

// RefreshSecurityIncident recomputes a security incident's severity (its
// worst open finding's, or worst finding's when none is open; critical
// when its open findings correlate) and summary.
func RefreshSecurityIncident(id int64) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	findings, err := ListScanFindings(ScanFindingFilter{IncidentID: id})
	if err != nil {
		return err
	}
	severity, summary := summarizeScanFindings(findings)
	escalation := models.CorrelateScanFindings(findings)
	if escalation != "" {
		severity = "critical"
	}
	if _, err := dbConn.Exec(`UPDATE incidents SET severity = ?, error_message = ?, escalation = ?, updated_at = ? WHERE id = ?`, severity, summary, escalation, time.Now(), id); err != nil {
		return fmt.Errorf("failed to update security incident %d: %w", id, err)
	}
	return nil
}

// summarizeScanFindings returns the worst severity of the open findings
// (of all, when none is open), and a summary such as "3 open findings:
// 2 uploads-php, 1 foreign-owner (1 cleared)", worst rule first.
func summarizeScanFindings(findings []models.StoredScanFinding) (string, string) {
	var open []models.StoredScanFinding
	for _, f := range findings {
		if f.Status == models.ScanFindingOpen {
			open = append(open, f)
		}
	}
	counted, label := open, "open finding"
	if len(open) == 0 {
		counted, label = findings, "finding"
	}
	severity := ""
	var rules []string
	perRule := map[string]int{}
	for _, f := range counted {
		if config.ScanSeverityRank(f.Severity) > config.ScanSeverityRank(severity) {
			severity = f.Severity
		}
		if perRule[f.Rule] == 0 {
			rules = append(rules, f.Rule)
		}
		perRule[f.Rule]++
	}
	// The worst rule first.
	worstOf := map[string]int{}
	for _, f := range counted {
		worstOf[f.Rule] = max(worstOf[f.Rule], config.ScanSeverityRank(f.Severity))
	}
	slices.SortStableFunc(rules, func(a, b string) int {
		if worstOf[a] != worstOf[b] {
			return worstOf[b] - worstOf[a]
		}
		return strings.Compare(a, b)
	})
	if len(counted) != 1 {
		label += "s"
	}
	parts := make([]string, len(rules))
	for i, rule := range rules {
		parts[i] = fmt.Sprintf("%d %s", perRule[rule], rule)
	}
	summary := fmt.Sprintf("%d %s", len(counted), label)
	if len(parts) > 0 {
		summary += ": " + strings.Join(parts, ", ")
	}
	cleared := 0
	for _, f := range counted {
		if f.ClearedAt != nil {
			cleared++
		}
	}
	switch {
	case cleared > 0 && cleared == len(counted) && len(counted) > 1:
		summary += " (all cleared)"
	case cleared == 1 && len(counted) == 1:
		summary += " (cleared)"
	case cleared > 0:
		summary += fmt.Sprintf(" (%d cleared)", cleared)
	}
	return severity, summary
}

// CloseSecurityIncident closes a security incident and its open findings.
func CloseSecurityIncident(id int64, by string) (*models.Incident, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`UPDATE scan_findings SET status = ?, status_by = ?, status_at = ? WHERE incident_id = ? AND status = 'open'`,
		models.ScanFindingClosed, by, time.Now(), id); err != nil {
		return nil, fmt.Errorf("failed to close the findings of incident %d: %w", id, err)
	}
	if err := RefreshSecurityIncident(id); err != nil {
		return nil, err
	}
	return CloseIncident(id, by)
}

// CountOpenScanFindings returns how many of an incident's findings are open.
func CountOpenScanFindings(incidentID int64) (int, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	var n int
	if err := dbConn.QueryRow(`SELECT COUNT(*) FROM scan_findings WHERE incident_id = ? AND status = 'open'`, incidentID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count open findings of incident %d: %w", incidentID, err)
	}
	return n, nil
}

// AddScanAllowlistEntry trusts a file hash on every site. It returns the
// existing entry when the hash is already allowlisted.
func AddScanAllowlistEntry(e models.ScanAllowlistEntry) (*models.ScanAllowlistEntry, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if existing, err := GetScanAllowlistEntryByHash(e.SHA256); err != nil || existing != nil {
		return existing, err
	}
	now := time.Now()
	var id int64
	err := dbConn.QueryRow(`
	INSERT INTO scan_allowlist (sha256, reason, site_id, rule, path, snippet, created_at, created_by, updated_at, updated_by)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id`, e.SHA256, e.Reason, e.SiteID, e.Rule, e.Path, e.Snippet, now, e.CreatedBy, now, e.CreatedBy).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to add allowlist entry: %w", err)
	}
	return getScanAllowlistEntry(`id = ?`, id)
}

// GetScanAllowlistEntryByHash returns the allowlist entry for a hash, or
// nil if it isn't allowlisted.
func GetScanAllowlistEntryByHash(sha256 string) (*models.ScanAllowlistEntry, error) {
	if sha256 == "" {
		return nil, nil
	}
	return getScanAllowlistEntry(`sha256 = ? AND removed = 0`, sha256)
}

const scanAllowlistColumns = `id, sha256, reason, site_id, rule, path, snippet, removed, created_at, created_by, updated_at, updated_by`

func scanAllowlistEntry(row interface{ Scan(...any) error }) (*models.ScanAllowlistEntry, error) {
	var e models.ScanAllowlistEntry
	err := row.Scan(&e.ID, &e.SHA256, &e.Reason, &e.SiteID, &e.Rule, &e.Path, &e.Snippet, &e.Removed, &e.CreatedAt, &e.CreatedBy, &e.UpdatedAt, &e.UpdatedBy)
	return &e, err
}

func getScanAllowlistEntry(where string, args ...any) (*models.ScanAllowlistEntry, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	e, err := scanAllowlistEntry(dbConn.QueryRow(`SELECT `+scanAllowlistColumns+` FROM scan_allowlist WHERE `+where+` ORDER BY id DESC LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read allowlist entry: %w", err)
	}
	return e, nil
}

// ListScanAllowlist returns the allowlist, newest first, with removed
// entries when includeRemoved is set.
func ListScanAllowlist(includeRemoved bool) ([]models.ScanAllowlistEntry, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	query := `SELECT ` + scanAllowlistColumns + ` FROM scan_allowlist`
	if !includeRemoved {
		query += ` WHERE removed = 0`
	}
	rows, err := dbConn.Query(query + ` ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query the allowlist: %w", err)
	}
	defer rows.Close()
	entries := []models.ScanAllowlistEntry{}
	for rows.Next() {
		e, err := scanAllowlistEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan allowlist entry: %w", err)
		}
		entries = append(entries, *e)
	}
	return entries, rows.Err()
}

// RemoveScanAllowlistEntry removes an entry from the allowlist, keeping it
// for the record. It reports whether there was such an entry to remove.
func RemoveScanAllowlistEntry(id int64, by string) (bool, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return false, fmt.Errorf("database not initialized")
	}
	res, err := dbConn.Exec(`UPDATE scan_allowlist SET removed = 1, updated_at = ?, updated_by = ? WHERE id = ? AND removed = 0`, time.Now(), by, id)
	if err != nil {
		return false, fmt.Errorf("failed to remove allowlist entry %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AddScanPathAck acknowledges a rule's findings for a path on a site. It
// returns the existing acknowledgement when there already is one.
func AddScanPathAck(a models.ScanPathAck) (*models.ScanPathAck, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if existing, err := GetScanPathAck(a.SiteID, a.Rule, a.Path); err != nil || existing != nil {
		return existing, err
	}
	now := time.Now()
	var id int64
	err := dbConn.QueryRow(`
	INSERT INTO scan_path_acks (site_id, rule, path, reason, created_at, created_by, updated_at, updated_by)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id`, a.SiteID, a.Rule, a.Path, a.Reason, now, a.CreatedBy, now, a.CreatedBy).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to add path acknowledgement: %w", err)
	}
	return getScanPathAck(`id = ?`, id)
}

// GetScanPathAck returns the acknowledgement of a rule's findings for a
// path on a site, or nil if there is none.
func GetScanPathAck(siteID, rule, path string) (*models.ScanPathAck, error) {
	return getScanPathAck(`site_id = ? AND rule = ? AND path = ? AND removed = 0`, siteID, rule, path)
}

const scanPathAckColumns = `id, site_id, rule, path, reason, removed, created_at, created_by, updated_at, updated_by`

func scanPathAck(row interface{ Scan(...any) error }) (*models.ScanPathAck, error) {
	var a models.ScanPathAck
	err := row.Scan(&a.ID, &a.SiteID, &a.Rule, &a.Path, &a.Reason, &a.Removed, &a.CreatedAt, &a.CreatedBy, &a.UpdatedAt, &a.UpdatedBy)
	return &a, err
}

func getScanPathAck(where string, args ...any) (*models.ScanPathAck, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	a, err := scanPathAck(dbConn.QueryRow(`SELECT `+scanPathAckColumns+` FROM scan_path_acks WHERE `+where+` ORDER BY id DESC LIMIT 1`, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read path acknowledgement: %w", err)
	}
	return a, nil
}

// ListScanPathAcks returns path acknowledgements, newest first, for one
// site or (siteID "") all, with removed ones when includeRemoved is set.
func ListScanPathAcks(siteID string, includeRemoved bool) ([]models.ScanPathAck, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var where []string
	var args []any
	if siteID != "" {
		where = append(where, "site_id = ?")
		args = append(args, siteID)
	}
	if !includeRemoved {
		where = append(where, "removed = 0")
	}
	query := `SELECT ` + scanPathAckColumns + ` FROM scan_path_acks`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	rows, err := dbConn.Query(query+` ORDER BY id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query path acknowledgements: %w", err)
	}
	defer rows.Close()
	acks := []models.ScanPathAck{}
	for rows.Next() {
		a, err := scanPathAck(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan path acknowledgement: %w", err)
		}
		acks = append(acks, *a)
	}
	return acks, rows.Err()
}

// RemoveScanPathAck removes a path acknowledgement, keeping it for the
// record. It reports whether there was one to remove.
func RemoveScanPathAck(id int64, by string) (bool, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return false, fmt.Errorf("database not initialized")
	}
	res, err := dbConn.Exec(`UPDATE scan_path_acks SET removed = 1, updated_at = ?, updated_by = ? WHERE id = ? AND removed = 0`, time.Now(), by, id)
	if err != nil {
		return false, fmt.Errorf("failed to remove path acknowledgement %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
