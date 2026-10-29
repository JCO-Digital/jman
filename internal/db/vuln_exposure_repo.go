package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const vulnExposureColumns = `id, kind, slug, site_id, version, vuln_uuids, max_cvss, fix_state, fixed_in,
	possible_fix_version, fix_available_since, first_seen_at, last_seen_at, resolved_at`

func scanVulnExposure(row interface{ Scan(...any) error }) (models.VulnExposure, error) {
	var e models.VulnExposure
	var uuids string
	var maxCvss sql.NullFloat64
	var fixSince, resolvedAt sql.NullTime
	err := row.Scan(&e.ID, &e.Kind, &e.Slug, &e.SiteID, &e.Version, &uuids, &maxCvss, &e.FixState, &e.FixedIn,
		&e.PossibleFixVersion, &fixSince, &e.FirstSeenAt, &e.LastSeenAt, &resolvedAt)
	if err != nil {
		return e, err
	}
	if err := json.Unmarshal([]byte(uuids), &e.VulnUUIDs); err != nil {
		return e, fmt.Errorf("exposure %d: bad vuln_uuids: %w", e.ID, err)
	}
	if maxCvss.Valid {
		e.MaxCvss = &maxCvss.Float64
	}
	if fixSince.Valid {
		e.FixAvailableSince = &fixSince.Time
	}
	if resolvedAt.Valid {
		e.ResolvedAt = &resolvedAt.Time
	}
	return e, nil
}

func queryVulnExposures(where string, args ...any) ([]models.VulnExposure, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT `+vulnExposureColumns+` FROM vuln_exposures WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query vulnerability exposures: %w", err)
	}
	defer rows.Close()
	exposures := []models.VulnExposure{}
	for rows.Next() {
		e, err := scanVulnExposure(rows)
		if err != nil {
			return nil, err
		}
		exposures = append(exposures, e)
	}
	return exposures, rows.Err()
}

// ListOpenVulnExposures returns every open exposure.
func ListOpenVulnExposures() ([]models.VulnExposure, error) {
	return queryVulnExposures(`resolved_at IS NULL`)
}

// ListVulnExposuresResolvedSince returns exposures resolved at or after
// since, newest last.
func ListVulnExposuresResolvedSince(since time.Time) ([]models.VulnExposure, error) {
	return queryVulnExposures(`resolved_at IS NOT NULL AND resolved_at >= ?`, since)
}

// InsertVulnExposure stores a new open exposure and sets its ID.
func InsertVulnExposure(e *models.VulnExposure) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	uuids, err := json.Marshal(e.VulnUUIDs)
	if err != nil {
		return err
	}
	err = dbConn.QueryRow(`
	INSERT INTO vuln_exposures (kind, slug, site_id, version, vuln_uuids, max_cvss, fix_state, fixed_in,
		possible_fix_version, fix_available_since, first_seen_at, last_seen_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id`, e.Kind, e.Slug, e.SiteID, e.Version, string(uuids), e.MaxCvss, e.FixState, e.FixedIn,
		e.PossibleFixVersion, e.FixAvailableSince, e.FirstSeenAt, e.LastSeenAt).Scan(&e.ID)
	if err != nil {
		return fmt.Errorf("failed to insert vulnerability exposure: %w", err)
	}
	return nil
}

// UpdateVulnExposure saves an exposure's observed state, including
// resolved_at, so it also reopens or resolves it.
func UpdateVulnExposure(e models.VulnExposure) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	uuids, err := json.Marshal(e.VulnUUIDs)
	if err != nil {
		return err
	}
	_, err = dbConn.Exec(`
	UPDATE vuln_exposures SET version = ?, vuln_uuids = ?, max_cvss = ?, fix_state = ?, fixed_in = ?,
		possible_fix_version = ?, fix_available_since = ?, last_seen_at = ?, resolved_at = ?
	WHERE id = ?`, e.Version, string(uuids), e.MaxCvss, e.FixState, e.FixedIn,
		e.PossibleFixVersion, e.FixAvailableSince, e.LastSeenAt, e.ResolvedAt, e.ID)
	if err != nil {
		return fmt.Errorf("failed to update vulnerability exposure %d: %w", e.ID, err)
	}
	return nil
}

// ResolveVulnExposures marks the given open exposures resolved at at.
func ResolveVulnExposures(ids []int64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	args := []any{at}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := dbConn.Exec(`UPDATE vuln_exposures SET resolved_at = ? WHERE resolved_at IS NULL AND id IN (?`+
		strings.Repeat(", ?", len(ids)-1)+`)`, args...)
	if err != nil {
		return fmt.Errorf("failed to resolve vulnerability exposures: %w", err)
	}
	return nil
}
