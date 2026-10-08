package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const updateLockColumns = `id, site_id, plugin, comment, created_by, created_at`

// SaveUpdateLock creates a site or plugin lock, or replaces the comment and
// author of the existing one for the same site and plugin. It sets the
// lock's ID and CreatedAt.
func SaveUpdateLock(lock *models.UpdateLock) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	lock.CreatedAt = time.Now()
	err := db.QueryRow(
		`INSERT INTO update_locks (site_id, plugin, comment, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(site_id, plugin) DO UPDATE SET
			comment = excluded.comment,
			created_by = excluded.created_by,
			created_at = excluded.created_at
		 RETURNING id`,
		lock.SiteID, lock.Plugin, lock.Comment, lock.CreatedBy, lock.CreatedAt,
	).Scan(&lock.ID)
	if err != nil {
		return fmt.Errorf("failed to save update lock: %w", err)
	}
	return nil
}

// GetUpdateLock returns a lock by ID, or nil if it doesn't exist.
func GetUpdateLock(id int64) (*models.UpdateLock, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var l models.UpdateLock
	var comment, createdBy sql.NullString
	err := db.QueryRow(`SELECT `+updateLockColumns+` FROM update_locks WHERE id = ?`, id).Scan(
		&l.ID, &l.SiteID, &l.Plugin, &comment, &createdBy, &l.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get update lock: %w", err)
	}
	l.Comment = comment.String
	l.CreatedBy = createdBy.String
	return &l, nil
}

// ListUpdateLocks returns every lock, ordered by site and plugin (site
// locks first).
func ListUpdateLocks() ([]models.UpdateLock, error) {
	return queryUpdateLocks(`SELECT ` + updateLockColumns + ` FROM update_locks ORDER BY site_id, plugin`)
}

// GetSiteLocks returns the locks of one site.
func GetSiteLocks(siteID string) (models.SiteLocks, error) {
	locks, err := queryUpdateLocks(`SELECT `+updateLockColumns+` FROM update_locks WHERE site_id = ?`, siteID)
	if err != nil {
		return models.SiteLocks{}, err
	}
	result := models.SiteLocks{Plugins: map[string]models.UpdateLock{}}
	for i, l := range locks {
		if l.Plugin == "" {
			result.Site = &locks[i]
		} else {
			result.Plugins[l.Plugin] = l
		}
	}
	return result, nil
}

// DeleteUpdateLock removes a lock and reports whether it existed. Removing
// a site lock leaves the site's plugin locks in place.
func DeleteUpdateLock(id int64) (bool, error) {
	db := GetAPIDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	res, err := db.Exec(`DELETE FROM update_locks WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("failed to delete update lock: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to delete update lock: %w", err)
	}
	return n > 0, nil
}

func queryUpdateLocks(query string, args ...any) ([]models.UpdateLock, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query update locks: %w", err)
	}
	defer rows.Close()

	locks := []models.UpdateLock{}
	for rows.Next() {
		var l models.UpdateLock
		var comment, createdBy sql.NullString
		if err := rows.Scan(&l.ID, &l.SiteID, &l.Plugin, &comment, &createdBy, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan update lock: %w", err)
		}
		l.Comment = comment.String
		l.CreatedBy = createdBy.String
		locks = append(locks, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate update locks: %w", err)
	}
	return locks, nil
}
