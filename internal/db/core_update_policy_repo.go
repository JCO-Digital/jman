package db

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// SetSiteCoreUpdatePolicy sets a site's core update policy override. The
// caller checks that the policy is valid.
func SetSiteCoreUpdatePolicy(siteID string, policy models.CoreUpdatePolicy, updatedBy string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`
		INSERT INTO site_core_update_policy (site_id, policy, updated_at, updated_by)
		VALUES (?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(site_id) DO UPDATE SET
			policy = excluded.policy,
			updated_at = CURRENT_TIMESTAMP,
			updated_by = excluded.updated_by`,
		siteID, string(policy), updatedBy)
	if err != nil {
		return fmt.Errorf("failed to set core update policy of site %s: %w", siteID, err)
	}
	return nil
}

// ClearSiteCoreUpdatePolicy removes a site's override, so the default
// applies again.
func ClearSiteCoreUpdatePolicy(siteID string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`DELETE FROM site_core_update_policy WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to clear core update policy of site %s: %w", siteID, err)
	}
	return nil
}

// GetAllSiteCoreUpdatePolicies returns every site's override, by site UUID.
func GetAllSiteCoreUpdatePolicies() (map[string]models.CoreUpdatePolicy, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := dbConn.Query(`SELECT site_id, policy FROM site_core_update_policy`)
	if err != nil {
		return nil, fmt.Errorf("failed to query core update policies: %w", err)
	}
	defer rows.Close()
	policies := make(map[string]models.CoreUpdatePolicy)
	for rows.Next() {
		var siteID, policy string
		if err := rows.Scan(&siteID, &policy); err != nil {
			return nil, fmt.Errorf("failed to scan core update policy: %w", err)
		}
		policies[siteID] = models.CoreUpdatePolicy(policy)
	}
	return policies, rows.Err()
}

// GetEffectiveCoreUpdatePolicies returns the core update policy in effect
// for every inventory site, by site UUID: its override, or the default for
// its environment and tier.
func GetEffectiveCoreUpdatePolicies() (map[string]models.CoreUpdatePolicy, error) {
	sites, err := ListManagedSites()
	if err != nil {
		return nil, err
	}
	environments, err := GetAllSiteEnvironments()
	if err != nil {
		return nil, err
	}
	tiers, err := GetAllSiteTiers()
	if err != nil {
		return nil, err
	}
	overrides, err := GetAllSiteCoreUpdatePolicies()
	if err != nil {
		return nil, err
	}
	out := make(map[string]models.CoreUpdatePolicy, len(sites))
	for _, s := range sites {
		env := s.Environment
		if e, ok := environments[s.ID]; ok {
			env = models.SiteEnvironmentType(e)
		}
		tier := 0
		if env == models.SiteEnvironmentProduction {
			tier = tiers[s.ID]
		}
		out[s.ID] = models.EffectiveCoreUpdatePolicy(overrides[s.ID], env, tier)
	}
	return out, nil
}
