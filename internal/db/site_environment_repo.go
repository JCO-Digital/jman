package db

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// SetSiteEnvironment inserts or updates the environment classification for a site (by UUID).
func SetSiteEnvironment(siteID string, environment string, updatedBy string) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_environment (site_id, environment, updated_at, updated_by)
	VALUES (?, ?, CURRENT_TIMESTAMP, ?)
	ON CONFLICT(site_id) DO UPDATE SET
		environment = excluded.environment,
		updated_at = CURRENT_TIMESTAMP,
		updated_by = excluded.updated_by;
	`

	if _, err := db.Exec(query, siteID, environment, updatedBy); err != nil {
		return fmt.Errorf("failed to set environment for site %s: %w", siteID, err)
	}

	return nil
}

// ClearSiteEnvironment removes the environment classification for a site,
// making it unclassified again.
func ClearSiteEnvironment(siteID string) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	if _, err := db.Exec(`DELETE FROM site_environment WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to clear environment for site %s: %w", siteID, err)
	}

	return nil
}

// GetAllSiteEnvironments returns a map of site UUID to environment for every classified site.
func GetAllSiteEnvironments() (map[string]string, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT site_id, environment FROM site_environment`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site environments: %w", err)
	}
	defer rows.Close()

	environments := make(map[string]string)
	for rows.Next() {
		var siteID string
		var environment string
		if err := rows.Scan(&siteID, &environment); err != nil {
			return nil, fmt.Errorf("failed to scan site environment: %w", err)
		}
		environments[siteID] = environment
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site environments: %w", err)
	}

	return environments, nil
}

// AutoClassifySiteEnvironments infers and persists an environment for every site that
// doesn't already have one, based on its domain. Sites that are already classified
// (manually or by a previous auto-classification) are left untouched. It returns the
// number of sites that were newly classified. sites are SpinupWP sites.
func AutoClassifySiteEnvironments(sites []models.Site) (int, error) {
	existing, err := GetAllSiteEnvironments()
	if err != nil {
		return 0, err
	}

	classified := 0
	for _, site := range sites {
		siteUUID := utils.SpinupWPSiteUUID(site.ID)
		if _, ok := existing[siteUUID]; ok {
			continue
		}

		env := models.InferEnvironmentFromDomain(site.Domain)
		if env == "" {
			continue
		}

		if err := SetSiteEnvironment(siteUUID, string(env), "auto-classifier"); err != nil {
			return classified, err
		}
		classified++
	}

	return classified, nil
}
