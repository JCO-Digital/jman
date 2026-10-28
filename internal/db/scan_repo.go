package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// GetSiteScanFingerprint returns the baseline fingerprint stored with a
// site's latest successful scan, and whether the site has been scanned
// successfully at all.
func GetSiteScanFingerprint(siteID string) (string, bool, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return "", false, fmt.Errorf("database not initialized")
	}
	var fingerprint string
	err := dbConn.QueryRow(`SELECT fingerprint FROM site_scans WHERE site_id = ? AND fingerprint != ''`, siteID).Scan(&fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to read scan fingerprint for site %s: %w", siteID, err)
	}
	return fingerprint, true, nil
}

// RecordSiteScan stores a site's latest scan. A failed scan keeps the
// fingerprint of the last successful one, so the next scan is still
// checked against it.
func RecordSiteScan(siteID string, s *models.AgentScan, scannedAt time.Time) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	var rebaselinedAt sql.NullTime
	if s.Rebaselined && s.Error == "" {
		rebaselinedAt = sql.NullTime{Time: scannedAt, Valid: true}
	}
	_, err := dbConn.Exec(`
	INSERT INTO site_scans (site_id, scanned_at, rule_set_version, fingerprint, entries, omitted, duration_ms, error, rebaselined_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id) DO UPDATE SET
		scanned_at = excluded.scanned_at,
		rule_set_version = excluded.rule_set_version,
		fingerprint = CASE WHEN excluded.error = '' THEN excluded.fingerprint ELSE site_scans.fingerprint END,
		entries = CASE WHEN excluded.error = '' THEN excluded.entries ELSE site_scans.entries END,
		omitted = CASE WHEN excluded.error = '' THEN excluded.omitted ELSE site_scans.omitted END,
		duration_ms = excluded.duration_ms,
		error = excluded.error,
		rebaselined_at = COALESCE(excluded.rebaselined_at, site_scans.rebaselined_at),
		updated_at = CURRENT_TIMESTAMP;
	`, siteID, scannedAt, s.RuleSetVersion, s.Fingerprint, s.Entries, s.Omitted, s.DurationMs, s.Error, rebaselinedAt)
	if err != nil {
		return fmt.Errorf("failed to record scan for site %s: %w", siteID, err)
	}
	return nil
}

// UpsertScanFinding stores a finding, or updates when it was last seen if
// the site already has the same one. A closed finding reported again is
// open again; allowlisted and acknowledged ones keep their status. One
// that was cleared isn't any more.
func UpsertScanFinding(siteID string, f models.ScanFinding, ruleSetVersion string, seenAt time.Time) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`
	INSERT INTO scan_findings (site_id, engine, rule, severity, path, sha256, snippet, detail, rule_set_version, first_seen_at, last_seen_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(site_id, rule, path, sha256) DO UPDATE SET
		severity = excluded.severity,
		snippet = excluded.snippet,
		detail = excluded.detail,
		rule_set_version = excluded.rule_set_version,
		last_seen_at = excluded.last_seen_at,
		status = CASE WHEN scan_findings.status = 'closed' THEN 'open' ELSE scan_findings.status END,
		status_by = CASE WHEN scan_findings.status = 'closed' THEN '' ELSE scan_findings.status_by END,
		status_at = CASE WHEN scan_findings.status = 'closed' THEN excluded.last_seen_at ELSE scan_findings.status_at END,
		cleared_at = NULL,
		cleared_detail = '',
		redacted_at = NULL,
		redacted_by = '';
	`, siteID, f.Engine, f.Rule, f.Severity, f.Path, f.SHA256, f.Snippet, f.Detail, ruleSetVersion, seenAt, seenAt)
	if err != nil {
		return fmt.Errorf("failed to store scan finding for site %s: %w", siteID, err)
	}
	return nil
}

// ScanFindingFilter selects findings for ListScanFindings. Zero fields
// don't filter.
type ScanFindingFilter struct {
	SiteID     string
	IncidentID int64
	Status     string
	SHA256     string
	// Unassigned keeps only findings without an incident.
	Unassigned bool
	Limit      int
}

const scanFindingColumns = `id, site_id, engine, rule, severity, path, sha256, snippet, detail, rule_set_version,
	first_seen_at, last_seen_at, incident_id, status, status_by, status_at, cleared_at, cleared_detail, redacted_at, redacted_by`

func scanScanFinding(row interface{ Scan(...any) error }) (models.StoredScanFinding, error) {
	var f models.StoredScanFinding
	var incidentID sql.NullInt64
	var statusAt, clearedAt, redactedAt sql.NullString
	err := row.Scan(&f.ID, &f.SiteID, &f.Engine, &f.Rule, &f.Severity, &f.Path, &f.SHA256, &f.Snippet, &f.Detail, &f.RuleSetVersion,
		&f.FirstSeenAt, &f.LastSeenAt, &incidentID, &f.Status, &f.StatusBy, &statusAt, &clearedAt, &f.ClearedDetail, &redactedAt, &f.RedactedBy)
	if redactedAt.Valid {
		f.RedactedAt = &redactedAt.String
	}
	if clearedAt.Valid {
		f.ClearedAt = &clearedAt.String
	}
	if incidentID.Valid {
		f.IncidentID = &incidentID.Int64
	}
	if statusAt.Valid {
		f.StatusAt = &statusAt.String
	}
	return f, err
}

// ListScanFindings returns the findings filter selects, most recently seen
// first.
func ListScanFindings(filter ScanFindingFilter) ([]models.StoredScanFinding, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var where []string
	var args []any
	if filter.SiteID != "" {
		where = append(where, "site_id = ?")
		args = append(args, filter.SiteID)
	}
	if filter.IncidentID != 0 {
		where = append(where, "incident_id = ?")
		args = append(args, filter.IncidentID)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.SHA256 != "" {
		where = append(where, "sha256 = ?")
		args = append(args, filter.SHA256)
	}
	if filter.Unassigned {
		where = append(where, "incident_id IS NULL")
	}
	query := `SELECT ` + scanFindingColumns + ` FROM scan_findings`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY last_seen_at DESC, id DESC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := dbConn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scan findings: %w", err)
	}
	defer rows.Close()

	findings := []models.StoredScanFinding{}
	for rows.Next() {
		f, err := scanScanFinding(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan scan finding: %w", err)
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scan findings: %w", err)
	}
	return findings, nil
}

// GetScanFindings returns the findings with the given IDs that exist.
func GetScanFindings(ids []int64) ([]models.StoredScanFinding, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := dbConn.Query(`SELECT `+scanFindingColumns+` FROM scan_findings WHERE id IN (?`+strings.Repeat(", ?", len(ids)-1)+`) ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scan findings: %w", err)
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

// ListSiteScans returns the latest scan of every scanned site.
func ListSiteScans() ([]models.SiteScan, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT site_id, scanned_at, rule_set_version, entries, omitted, duration_ms, error, rebaselined_at FROM site_scans ORDER BY site_id`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site scans: %w", err)
	}
	defer rows.Close()

	scans := []models.SiteScan{}
	for rows.Next() {
		var s models.SiteScan
		var rebaselinedAt sql.NullString
		if err := rows.Scan(&s.SiteID, &s.ScannedAt, &s.RuleSetVersion, &s.Entries, &s.Omitted, &s.DurationMs, &s.Error, &rebaselinedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site scan: %w", err)
		}
		if rebaselinedAt.Valid {
			s.RebaselinedAt = &rebaselinedAt.String
		}
		scans = append(scans, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site scans: %w", err)
	}
	return scans, nil
}

// ClearScanFindings marks a site's findings that match c as cleared, and
// returns the incidents they belong to.
func ClearScanFindings(siteID string, c models.ScanCleared, at time.Time) ([]int64, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`
	UPDATE scan_findings SET cleared_at = ?, cleared_detail = ?
	WHERE site_id = ? AND path = ? AND cleared_at IS NULL AND engine != 'api'
		AND (? = '' OR rule = ?) AND (? = '' OR sha256 = ?)
	RETURNING incident_id`, at, c.Detail, siteID, c.Path, c.Rule, c.Rule, c.SHA256, c.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to clear scan findings for site %s: %w", siteID, err)
	}
	defer rows.Close()
	var incidents []int64
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id.Valid {
			incidents = append(incidents, id.Int64)
		}
	}
	return incidents, rows.Err()
}

// RedactScanFindings removes the snippet and detail of the given findings,
// recording who did it, and returns how many it changed.
func RedactScanFindings(ids []int64, by string) (int, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	args := []any{time.Now(), by}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := dbConn.Exec(`UPDATE scan_findings SET snippet = '', detail = '', redacted_at = ?, redacted_by = ?
	WHERE redacted_at IS NULL AND id IN (?`+strings.Repeat(", ?", len(ids)-1)+`)`, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to redact scan findings: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// RedactExpiredScanFindings removes the snippet and detail of findings that
// have been closed, allowlisted or acknowledged since before before, as by,
// and returns how many it changed. Open findings keep theirs for triage.
func RedactExpiredScanFindings(before time.Time, by string) (int, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	res, err := dbConn.Exec(`UPDATE scan_findings SET snippet = '', detail = '', redacted_at = ?, redacted_by = ?
	WHERE status != 'open' AND redacted_at IS NULL AND (snippet != '' OR detail != '')
		AND COALESCE(status_at, last_seen_at) < ?`, time.Now(), by, before)
	if err != nil {
		return 0, fmt.Errorf("failed to redact expired scan findings: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
