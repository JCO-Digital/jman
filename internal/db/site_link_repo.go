package db

import (
	"database/sql"
	"fmt"
)

// GetEffectiveSiteEnvironment returns a site's environment as /api/sites
// reports it: its site_environment classification, or the inventory record's
// default. found is false when the site is in neither.
func GetEffectiveSiteEnvironment(siteID string) (env string, found bool, err error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return "", false, fmt.Errorf("database not initialized")
	}
	// The scalar subqueries make this one row, NULL when neither has the site.
	var v sql.NullString
	err = dbConn.QueryRow(`
		SELECT COALESCE(
			(SELECT environment FROM site_environment WHERE site_id = ?),
			(SELECT environment FROM sites WHERE id = ?)
		)`, siteID, siteID).Scan(&v)
	if err != nil {
		return "", false, fmt.Errorf("failed to get environment of site %s: %w", siteID, err)
	}
	return v.String, v.Valid, nil
}

// SetSiteProductionLink links a staging, development or archived site to
// its production site, replacing any earlier link. The caller checks the
// environments.
func SetSiteProductionLink(siteID, productionSiteID, createdBy string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`
		INSERT INTO site_production_link (site_id, production_site_id, created_at, created_by)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(site_id) DO UPDATE SET
			production_site_id = excluded.production_site_id,
			created_at = CURRENT_TIMESTAMP,
			created_by = excluded.created_by`,
		siteID, productionSiteID, createdBy)
	if err != nil {
		return fmt.Errorf("failed to link site %s to %s: %w", siteID, productionSiteID, err)
	}
	return nil
}

// DeleteSiteProductionLink removes a site's link to its production site.
func DeleteSiteProductionLink(siteID string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`DELETE FROM site_production_link WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to unlink site %s: %w", siteID, err)
	}
	return nil
}

// CountSitesLinkedTo returns how many sites are linked to siteID as their
// production site.
func CountSitesLinkedTo(siteID string) (int, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	var n int
	if err := dbConn.QueryRow(`SELECT COUNT(*) FROM site_production_link WHERE production_site_id = ?`, siteID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count sites linked to %s: %w", siteID, err)
	}
	return n, nil
}

// GetAllSiteProductionLinks returns every linked site's production site, by
// linked site UUID.
func GetAllSiteProductionLinks() (map[string]string, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT site_id, production_site_id FROM site_production_link`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site links: %w", err)
	}
	defer rows.Close()
	links := make(map[string]string)
	for rows.Next() {
		var siteID, productionID string
		if err := rows.Scan(&siteID, &productionID); err != nil {
			return nil, fmt.Errorf("failed to scan site link: %w", err)
		}
		links[siteID] = productionID
	}
	return links, rows.Err()
}

// SetSiteTier sets a site's tier by its id. The caller checks that the site
// is production and the tier exists.
func SetSiteTier(siteID string, tier int, updatedBy string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`
		INSERT INTO site_tier (site_id, tier, updated_at, updated_by)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(site_id) DO UPDATE SET
			tier = excluded.tier,
			updated_at = CURRENT_TIMESTAMP,
			updated_by = excluded.updated_by`,
		siteID, tier, updatedBy)
	if err != nil {
		return fmt.Errorf("failed to set tier of site %s: %w", siteID, err)
	}
	return nil
}

// ClearSiteTier removes a site's tier.
func ClearSiteTier(siteID string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`DELETE FROM site_tier WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to clear tier of site %s: %w", siteID, err)
	}
	return nil
}

// GetAllSiteTiers returns every site's tier id, by site UUID.
func GetAllSiteTiers() (map[string]int, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT site_id, tier FROM site_tier`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site tiers: %w", err)
	}
	defer rows.Close()
	tiers := make(map[string]int)
	for rows.Next() {
		var siteID string
		var tier int
		if err := rows.Scan(&siteID, &tier); err != nil {
			return nil, fmt.Errorf("failed to scan site tier: %w", err)
		}
		tiers[siteID] = tier
	}
	return tiers, rows.Err()
}
