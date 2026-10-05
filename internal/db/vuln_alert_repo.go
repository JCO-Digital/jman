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
