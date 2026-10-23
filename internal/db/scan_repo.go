package db

import (
	"database/sql"
	"errors"
	"fmt"
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
// the site already has the same one.
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
		last_seen_at = excluded.last_seen_at;
	`, siteID, f.Engine, f.Rule, f.Severity, f.Path, f.SHA256, f.Snippet, f.Detail, ruleSetVersion, seenAt, seenAt)
	if err != nil {
		return fmt.Errorf("failed to store scan finding for site %s: %w", siteID, err)
	}
	return nil
}

// ListScanFindings returns findings, most recently seen first, for one
// site or (siteID "") all sites.
func ListScanFindings(siteID string, limit int) ([]models.StoredScanFinding, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	query := `SELECT id, site_id, engine, rule, severity, path, sha256, snippet, detail, rule_set_version, first_seen_at, last_seen_at FROM scan_findings`
	args := []any{}
	if siteID != "" {
		query += ` WHERE site_id = ?`
		args = append(args, siteID)
	}
	query += ` ORDER BY last_seen_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := dbConn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query scan findings: %w", err)
	}
	defer rows.Close()

	findings := []models.StoredScanFinding{}
	for rows.Next() {
		var f models.StoredScanFinding
		if err := rows.Scan(&f.ID, &f.SiteID, &f.Engine, &f.Rule, &f.Severity, &f.Path, &f.SHA256, &f.Snippet, &f.Detail, &f.RuleSetVersion, &f.FirstSeenAt, &f.LastSeenAt); err != nil {
			return nil, fmt.Errorf("failed to scan scan finding: %w", err)
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scan findings: %w", err)
	}
	return findings, nil
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
