package db

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// ErrSiteDiffPageExists is returned when a site already has the page.
var ErrSiteDiffPageExists = errors.New("the site already has this page")

// ListSiteDiffPages returns a site's extra text diff pages by path.
func ListSiteDiffPages(siteID string) ([]models.SiteDiffPage, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT id, site_id, path, created_at, created_by FROM site_diff_pages
		WHERE site_id = ? ORDER BY path`, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to list text diff pages: %w", err)
	}
	defer rows.Close()
	pages := []models.SiteDiffPage{}
	for rows.Next() {
		var p models.SiteDiffPage
		if err := rows.Scan(&p.ID, &p.SiteID, &p.Path, &p.CreatedAt, &p.CreatedBy); err != nil {
			return nil, fmt.Errorf("failed to scan text diff page: %w", err)
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// InsertSiteDiffPage adds an extra page to a site and sets its ID and
// CreatedAt.
func InsertSiteDiffPage(p *models.SiteDiffPage) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	p.CreatedAt = time.Now()
	err := dbConn.QueryRow(`INSERT INTO site_diff_pages (site_id, path, created_at, created_by)
		VALUES (?, ?, ?, ?) RETURNING id`, p.SiteID, p.Path, p.CreatedAt, p.CreatedBy).Scan(&p.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrSiteDiffPageExists
		}
		return fmt.Errorf("failed to add text diff page: %w", err)
	}
	return nil
}

// DeleteSiteDiffPage removes a site's extra page, reporting whether it
// existed.
func DeleteSiteDiffPage(siteID string, id int64) (bool, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return false, fmt.Errorf("database not initialized")
	}
	res, err := dbConn.Exec(`DELETE FROM site_diff_pages WHERE site_id = ? AND id = ?`, siteID, id)
	if err != nil {
		return false, fmt.Errorf("failed to delete text diff page: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

const textDiffMuteColumns = `id, site_id, path, direction, line, reason, expires_at,
	created_at, created_by, updated_at, updated_by`

func scanTextDiffMute(row interface{ Scan(...any) error }) (models.TextDiffMute, error) {
	var m models.TextDiffMute
	err := row.Scan(&m.ID, &m.SiteID, &m.Path, &m.Direction, &m.Line, &m.Reason, &m.ExpiresAt,
		&m.CreatedAt, &m.CreatedBy, &m.UpdatedAt, &m.UpdatedBy)
	return m, err
}

func queryTextDiffMutes(where string, args ...any) ([]models.TextDiffMute, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT `+textDiffMuteColumns+` FROM text_diff_mutes WHERE `+where+
		` ORDER BY path, line, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query text diff mutes: %w", err)
	}
	defer rows.Close()
	mutes := []models.TextDiffMute{}
	for rows.Next() {
		m, err := scanTextDiffMute(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan text diff mute: %w", err)
		}
		mutes = append(mutes, m)
	}
	return mutes, rows.Err()
}

// ListActiveTextDiffMutes returns a site's mutes that haven't expired at now.
func ListActiveTextDiffMutes(siteID string, now time.Time) ([]models.TextDiffMute, error) {
	return queryTextDiffMutes(`site_id = ? AND expires_at > ?`, siteID, now)
}

// ListTextDiffMutes returns all of a site's mutes, expired ones included.
func ListTextDiffMutes(siteID string) ([]models.TextDiffMute, error) {
	return queryTextDiffMutes(`site_id = ?`, siteID)
}

// GetTextDiffMute returns a mute by ID, or nil if there is none.
func GetTextDiffMute(id int64) (*models.TextDiffMute, error) {
	mutes, err := queryTextDiffMutes(`id = ?`, id)
	if err != nil || len(mutes) == 0 {
		return nil, err
	}
	return &mutes[0], nil
}

// InsertTextDiffMute stores a new mute and sets its ID and audit times.
func InsertTextDiffMute(m *models.TextDiffMute) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	now := time.Now()
	err := dbConn.QueryRow(`INSERT INTO text_diff_mutes (site_id, path, direction, line, reason, expires_at,
		created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		m.SiteID, m.Path, m.Direction, m.Line, m.Reason, m.ExpiresAt, now, m.CreatedBy, now, m.CreatedBy).Scan(&m.ID)
	if err != nil {
		return fmt.Errorf("failed to insert text diff mute: %w", err)
	}
	m.CreatedAt, m.UpdatedAt, m.UpdatedBy = now, now, m.CreatedBy
	return nil
}

// ExpireTextDiffMute ends a mute now, keeping it for the record.
func ExpireTextDiffMute(id int64, by string) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	now := time.Now()
	_, err := dbConn.Exec(`UPDATE text_diff_mutes SET expires_at = ?, updated_at = ?, updated_by = ?
		WHERE id = ? AND expires_at > ?`, now, now, by, id, now)
	if err != nil {
		return fmt.Errorf("failed to remove text diff mute %d: %w", id, err)
	}
	return nil
}
