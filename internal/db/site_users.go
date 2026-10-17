package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// ReplaceSiteUsers replaces a site's users and summary with users in one
// transaction, stamped with observedAt (when they were read from the
// site), so a reader never sees a half-replaced list.
func ReplaceSiteUsers(siteID string, users models.SiteUsers, observedAt time.Time) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	counts := users.ExcludedCounts
	if counts == nil {
		counts = map[string]int{}
	}
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		return fmt.Errorf("failed to encode user counts for site %s: %w", siteID, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to replace users for site %s: %w", siteID, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM site_users WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to clear users for site %s: %w", siteID, err)
	}
	for _, u := range users.Users {
		if _, err := tx.Exec(
			`INSERT INTO site_users (site_id, user_id, login, email, display_name, roles, registered)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(site_id, user_id) DO UPDATE SET
				login = excluded.login,
				email = excluded.email,
				display_name = excluded.display_name,
				roles = excluded.roles,
				registered = excluded.registered`,
			siteID, u.ID, u.Login, u.Email, u.DisplayName, u.Roles, u.Registered,
		); err != nil {
			return fmt.Errorf("failed to save user %d for site %s: %w", u.ID, siteID, err)
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO site_user_summary (site_id, excluded_counts, truncated, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(site_id) DO UPDATE SET
			excluded_counts = excluded.excluded_counts,
			truncated = excluded.truncated,
			updated_at = excluded.updated_at`,
		siteID, string(countsJSON), users.Truncated, inventoryTimestamp(observedAt),
	); err != nil {
		return fmt.Errorf("failed to save user summary for site %s: %w", siteID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to replace users for site %s: %w", siteID, err)
	}
	return nil
}

// GetSiteUsers returns a site's stored users, ordered by ID, and false if
// its users were never read.
func GetSiteUsers(siteID string) (models.SiteUsers, bool, error) {
	db := GetInventoryDB()
	if db == nil {
		return models.SiteUsers{}, false, fmt.Errorf("database not initialized")
	}
	summary, found, err := getSiteUserSummary(db, siteID)
	if err != nil || !found {
		return models.SiteUsers{}, false, err
	}
	users, err := querySiteUsers(db, `WHERE site_id = ?`, siteID)
	if err != nil {
		return models.SiteUsers{}, false, err
	}
	return models.SiteUsers{Users: users, ExcludedCounts: summary.ExcludedCounts, Truncated: summary.Truncated}, true, nil
}

// GetAllSiteUsers returns every stored user of every site, ordered by
// site and ID.
func GetAllSiteUsers() ([]models.SiteUser, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return querySiteUsers(db, "")
}

func querySiteUsers(db *sql.DB, where string, args ...any) ([]models.SiteUser, error) {
	rows, err := db.Query(`SELECT site_id, user_id, login, email, display_name, roles, registered
		FROM site_users `+where+` ORDER BY site_id, user_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query site users: %w", err)
	}
	defer rows.Close()

	users := []models.SiteUser{}
	for rows.Next() {
		var u models.SiteUser
		if err := rows.Scan(&u.SiteID, &u.ID, &u.Login, &u.Email, &u.DisplayName, &u.Roles, &u.Registered); err != nil {
			return nil, fmt.Errorf("failed to scan site user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site users: %w", err)
	}
	return users, nil
}

// GetSiteUserSummary returns a site's user summary, and false if its users
// were never read.
func GetSiteUserSummary(siteID string) (models.SiteUserSummary, bool, error) {
	db := GetInventoryDB()
	if db == nil {
		return models.SiteUserSummary{}, false, fmt.Errorf("database not initialized")
	}
	return getSiteUserSummary(db, siteID)
}

func getSiteUserSummary(db *sql.DB, siteID string) (models.SiteUserSummary, bool, error) {
	summaries, err := querySiteUserSummaries(db, `WHERE site_id = ?`, siteID)
	if err != nil || len(summaries) == 0 {
		return models.SiteUserSummary{}, false, err
	}
	return summaries[0], true, nil
}

// GetAllSiteUserSummaries returns the user summaries of every site whose
// users were read, ordered by site.
func GetAllSiteUserSummaries() ([]models.SiteUserSummary, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return querySiteUserSummaries(db, "")
}

func querySiteUserSummaries(db *sql.DB, where string, args ...any) ([]models.SiteUserSummary, error) {
	rows, err := db.Query(`SELECT site_id, excluded_counts, truncated, updated_at
		FROM site_user_summary `+where+` ORDER BY site_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query site user summaries: %w", err)
	}
	defer rows.Close()

	summaries := []models.SiteUserSummary{}
	for rows.Next() {
		var s models.SiteUserSummary
		var counts string
		var updatedAt sql.NullString
		if err := rows.Scan(&s.SiteID, &counts, &s.Truncated, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site user summary: %w", err)
		}
		if err := json.Unmarshal([]byte(counts), &s.ExcludedCounts); err != nil || s.ExcludedCounts == nil {
			s.ExcludedCounts = map[string]int{}
		}
		if updatedAt.Valid {
			if t, err := parseInventoryTimestamp(updatedAt.String); err == nil {
				s.UpdatedAt = t.UTC().Format(time.RFC3339)
			}
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site user summaries: %w", err)
	}
	return summaries, nil
}

// GetSiteUsersObservedAt returns when a site's users were last read, and
// false if they never were.
func GetSiteUsersObservedAt(siteID string) (time.Time, bool, error) {
	db := GetInventoryDB()
	if db == nil {
		return time.Time{}, false, fmt.Errorf("database not initialized")
	}
	var updatedAt sql.NullString
	err := db.QueryRow(`SELECT updated_at FROM site_user_summary WHERE site_id = ?`, siteID).Scan(&updatedAt)
	if err == sql.ErrNoRows || (err == nil && (!updatedAt.Valid || updatedAt.String == "")) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to read user cache time for site %s: %w", siteID, err)
	}
	t, err := parseInventoryTimestamp(updatedAt.String)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid user cache time %q for site %s: %w", updatedAt.String, siteID, err)
	}
	return t, true, nil
}

// DeleteSiteUsers removes a site's stored users and summary.
func DeleteSiteUsers(siteID string) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to delete users for site %s: %w", siteID, err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM site_users WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to delete users for site %s: %w", siteID, err)
	}
	if _, err := tx.Exec(`DELETE FROM site_user_summary WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to delete user summary for site %s: %w", siteID, err)
	}
	return tx.Commit()
}
