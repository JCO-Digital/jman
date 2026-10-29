package db

import "fmt"

// VulnAlertsSeededSettingKey is the system setting marking that vuln_alerts
// has been seeded with the vulnerabilities that existed when new-vulnerability
// alerting was first enabled, so they aren't all alerted at once.
const VulnAlertsSeededSettingKey = "vuln_alerts_seeded"

// GetAlertedVulnUUIDs returns the set of vulnerability UUIDs that have
// already had a new-vulnerability alert sent (or were seeded).
func GetAlertedVulnUUIDs() (map[string]bool, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(`SELECT uuid FROM vuln_alerts`)
	if err != nil {
		return nil, fmt.Errorf("failed to query vuln alerts: %w", err)
	}
	defer rows.Close()

	uuids := make(map[string]bool)
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, err
		}
		uuids[uuid] = true
	}
	return uuids, rows.Err()
}

// RecordVulnAlert remembers that a vulnerability's alert has been sent;
// recording the same UUID again is a no-op.
func RecordVulnAlert(uuid, pluginSlug string, cvss float64) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`INSERT INTO vuln_alerts (uuid, plugin_slug, cvss) VALUES (?, ?, ?) ON CONFLICT (uuid) DO NOTHING`, uuid, pluginSlug, cvss)
	return err
}

// VulnAlertSitesSeededSettingKey is the system setting marking that
// vuln_alert_sites has been seeded with the exposures that existed when
// per-site alerting was first enabled.
const VulnAlertSitesSeededSettingKey = "vuln_alert_sites_seeded"

// VulnAlertSite is one vulnerability alerted (or seeded) for a plugin (or
// core) on a site.
type VulnAlertSite struct {
	Kind, Slug, SiteID, UUID string
}

// ListVulnAlertSites returns every recorded per-site vulnerability alert.
func ListVulnAlertSites() (map[VulnAlertSite]bool, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(`SELECT kind, slug, site_id, uuid FROM vuln_alert_sites`)
	if err != nil {
		return nil, fmt.Errorf("failed to query vuln alert sites: %w", err)
	}
	defer rows.Close()
	sites := map[VulnAlertSite]bool{}
	for rows.Next() {
		var a VulnAlertSite
		if err := rows.Scan(&a.Kind, &a.Slug, &a.SiteID, &a.UUID); err != nil {
			return nil, err
		}
		sites[a] = true
	}
	return sites, rows.Err()
}

// RecordVulnAlertSites remembers that the given per-site alerts have been
// sent (or seeded); recording one again is a no-op.
func RecordVulnAlertSites(sites []VulnAlertSite) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	for _, a := range sites {
		if _, err := db.Exec(`INSERT INTO vuln_alert_sites (kind, slug, site_id, uuid) VALUES (?, ?, ?, ?)
			ON CONFLICT DO NOTHING`, a.Kind, a.Slug, a.SiteID, a.UUID); err != nil {
			return fmt.Errorf("failed to record vuln alert site: %w", err)
		}
	}
	return nil
}

// DeleteVulnAlertSites forgets the per-site alerts of a plugin (or core) on
// a site, so its vulnerabilities are alerted again if it becomes affected
// again (reinstalled or downgraded).
func DeleteVulnAlertSites(kind, slug, siteID string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`DELETE FROM vuln_alert_sites WHERE kind = ? AND slug = ? AND site_id = ?`, kind, slug, siteID)
	return err
}
