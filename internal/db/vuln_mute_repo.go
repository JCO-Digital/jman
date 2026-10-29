package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const vulnMuteColumns = `id, kind, slug, site_id, reason, status, fix_state_at_mute, vuln_uuids_at_mute,
	possible_fix_at_mute, review_at, lifted_at, lifted_reason, created_at, created_by, updated_at, updated_by`

func scanVulnMute(row interface{ Scan(...any) error }) (models.VulnMute, error) {
	var m models.VulnMute
	var uuids string
	var liftedAt sql.NullTime
	err := row.Scan(&m.ID, &m.Kind, &m.Slug, &m.SiteID, &m.Reason, &m.Status, &m.FixStateAtMute, &uuids,
		&m.PossibleFixAtMute, &m.ReviewAt, &liftedAt, &m.LiftedReason, &m.CreatedAt, &m.CreatedBy, &m.UpdatedAt, &m.UpdatedBy)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal([]byte(uuids), &m.VulnUUIDsAtMute); err != nil {
		return m, fmt.Errorf("mute %d: bad vuln_uuids_at_mute: %w", m.ID, err)
	}
	if liftedAt.Valid {
		m.LiftedAt = &liftedAt.Time
	}
	return m, nil
}

func queryVulnMutes(where, order string, args ...any) ([]models.VulnMute, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT `+vulnMuteColumns+` FROM vuln_mutes WHERE `+where+` ORDER BY `+order, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query vulnerability mutes: %w", err)
	}
	defer rows.Close()
	mutes := []models.VulnMute{}
	for rows.Next() {
		m, err := scanVulnMute(rows)
		if err != nil {
			return nil, err
		}
		mutes = append(mutes, m)
	}
	return mutes, rows.Err()
}

// ListOpenVulnMutes returns the mutes that still apply (active or needing
// review), soonest review first.
func ListOpenVulnMutes() ([]models.VulnMute, error) {
	return queryVulnMutes(`status != 'lifted'`, `review_at, id`)
}

// ListVulnMutesLiftedSince returns mutes lifted at or after since, newest
// first.
func ListVulnMutesLiftedSince(since time.Time) ([]models.VulnMute, error) {
	return queryVulnMutes(`status = 'lifted' AND lifted_at >= ?`, `lifted_at DESC, id DESC`, since)
}

// GetVulnMute returns a mute by ID, or nil if there is none.
func GetVulnMute(id int64) (*models.VulnMute, error) {
	mutes, err := queryVulnMutes(`id = ?`, `id`, id)
	if err != nil || len(mutes) == 0 {
		return nil, err
	}
	return &mutes[0], nil
}

// ErrVulnMuteExists is returned when the exposure already has an open mute.
var ErrVulnMuteExists = errors.New("the exposure is already muted")

// InsertVulnMute stores a new open mute and sets its ID and audit times.
func InsertVulnMute(m *models.VulnMute) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	var open int
	if err := dbConn.QueryRow(`SELECT COUNT(*) FROM vuln_mutes WHERE kind = ? AND slug = ? AND site_id = ? AND status != 'lifted'`,
		m.Kind, m.Slug, m.SiteID).Scan(&open); err != nil {
		return err
	}
	if open > 0 {
		return ErrVulnMuteExists
	}
	uuids, err := json.Marshal(m.VulnUUIDsAtMute)
	if err != nil {
		return err
	}
	now := time.Now()
	err = dbConn.QueryRow(`
	INSERT INTO vuln_mutes (kind, slug, site_id, reason, status, fix_state_at_mute, vuln_uuids_at_mute,
		possible_fix_at_mute, review_at, created_at, created_by, updated_at, updated_by)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING id`, m.Kind, m.Slug, m.SiteID, m.Reason, m.Status, m.FixStateAtMute, string(uuids),
		m.PossibleFixAtMute, m.ReviewAt, now, m.CreatedBy, now, m.CreatedBy).Scan(&m.ID)
	if err != nil {
		return fmt.Errorf("failed to insert vulnerability mute: %w", err)
	}
	m.CreatedAt, m.UpdatedAt, m.UpdatedBy = now, now, m.CreatedBy
	return nil
}

// UpdateVulnMute saves a mute's reason, status, snapshot, review date and
// lifted fields, and stamps it as updated by m.UpdatedBy now.
func UpdateVulnMute(m *models.VulnMute) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	uuids, err := json.Marshal(m.VulnUUIDsAtMute)
	if err != nil {
		return err
	}
	m.UpdatedAt = time.Now()
	_, err = dbConn.Exec(`
	UPDATE vuln_mutes SET reason = ?, status = ?, fix_state_at_mute = ?, vuln_uuids_at_mute = ?,
		possible_fix_at_mute = ?, review_at = ?, lifted_at = ?, lifted_reason = ?, updated_at = ?, updated_by = ?
	WHERE id = ?`, m.Reason, m.Status, m.FixStateAtMute, string(uuids), m.PossibleFixAtMute, m.ReviewAt,
		m.LiftedAt, m.LiftedReason, m.UpdatedAt, m.UpdatedBy, m.ID)
	if err != nil {
		return fmt.Errorf("failed to update vulnerability mute %d: %w", m.ID, err)
	}
	return nil
}
