package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

const (
	// monitorChecksMigrationName is the api.db schema_migrations key
	// recording that monitoring moved from domains to monitor_checks.
	monitorChecksMigrationName = "monitor_checks"
	// monitorIgnoresMigrationName is the inventory.db key recording that
	// the monitor ignore rules were removed after becoming paused checks.
	monitorIgnoresMigrationName = "monitor_ignores_to_checks"
)

// MigrateMonitorChecks moves uptime monitoring from domains to
// monitor_checks. Every site in inventory.db gets a default check, disabled
// when the site had monitoring turned off or a monitor ignore rule matched
// it, so the default-check reconcile never sees it as missing. The monitor
// state, history and incidents are then re-keyed from domain to check ID;
// rows whose domain matches no site are dropped.
//
// It writes only api.db, in one transaction. Once that is recorded, the
// monitor ignore rules are removed from inventory.db in a second,
// separately recorded step. Without inventory.db it does nothing, so the
// migration runs on the next start that has both databases.
func MigrateMonitorChecks(apiConn, inventoryConn *sql.DB) error {
	if apiConn == nil || inventoryConn == nil {
		return nil
	}

	err := runOnce(apiConn, monitorChecksMigrationName, func(tx *sql.Tx) error {
		sites, err := loadMigrationSites(inventoryConn)
		if err != nil {
			return err
		}
		ignores, err := loadMonitorIgnores(inventoryConn)
		if err != nil {
			return err
		}

		checkByDomain := make(map[string]string, len(sites))
		for _, s := range sites {
			id := utils.NewV7UUID()
			enabled := s.hasMonitoring && !ignores.isIgnored(s.id, s.serverID)
			if _, err := tx.Exec(`
				INSERT INTO monitor_checks (id, site_id, is_default, type, path, expect, enabled, created_by, updated_by)
				VALUES (?, ?, 1, ?, '/', ?, ?, 'system', 'system')
			`, id, s.id, models.MonitorCheckTypeHTTP, models.MonitorExpectHTML, enabled); err != nil {
				return fmt.Errorf("failed to create default check for %s: %w", s.domain, err)
			}
			// Sites are ordered SpinupWP first, so a domain that exists twice
			// keeps its history on the SpinupWP site's check.
			domain := strings.ToLower(s.domain)
			if _, ok := checkByDomain[domain]; !ok {
				checkByDomain[domain] = id
			}
		}

		return rekeyMonitorData(tx, checkByDomain)
	})
	if err != nil {
		return err
	}

	return runOnce(inventoryConn, monitorIgnoresMigrationName, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM ignore_entries WHERE use_for_monitor = 1 AND use_for_vuln = 0`); err != nil {
			return fmt.Errorf("failed to delete monitor ignore entries: %w", err)
		}
		if _, err := tx.Exec(`UPDATE ignore_entries SET use_for_monitor = 0 WHERE use_for_monitor = 1`); err != nil {
			return fmt.Errorf("failed to clear monitor ignore flags: %w", err)
		}
		return nil
	})
}

type migrationSite struct {
	id, serverID, domain string
	hasMonitoring        bool
}

func loadMigrationSites(inventoryConn *sql.DB) ([]migrationSite, error) {
	rows, err := inventoryConn.Query(`
		SELECT id, COALESCE(server_id, ''), domain, has_monitoring
		FROM sites
		ORDER BY provider = 'spinupwp' DESC, domain ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to load sites: %w", err)
	}
	defer rows.Close()

	var sites []migrationSite
	for rows.Next() {
		var s migrationSite
		if err := rows.Scan(&s.id, &s.serverID, &s.domain, &s.hasMonitoring); err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

func loadMonitorIgnores(inventoryConn *sql.DB) (siteServerIgnores, error) {
	ignores := newSiteServerIgnores()
	rows, err := inventoryConn.Query(`
		SELECT type, target, COALESCE(negated_site_ids, '')
		FROM ignore_entries
		WHERE use_for_monitor = 1 AND type IN ('site', 'server')
	`)
	if err != nil {
		return ignores, fmt.Errorf("failed to load monitor ignore entries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e models.IgnoreEntry
		var negatedJSON string
		if err := rows.Scan(&e.Type, &e.Target, &negatedJSON); err != nil {
			return ignores, fmt.Errorf("failed to scan monitor ignore entry: %w", err)
		}
		if negatedJSON != "" {
			_ = json.Unmarshal([]byte(negatedJSON), &e.NegatedSiteIDs)
		}
		ignores.add(e)
	}
	return ignores, rows.Err()
}

// rekeyMonitorData points monitor state, history and incidents at the
// default checks, by domain, and drops monitor_status, which
// monitor_check_status replaces.
func rekeyMonitorData(tx *sql.Tx, checkByDomain map[string]string) error {
	hasStatus, err := tableExists(tx, "monitor_status")
	if err != nil {
		return err
	}

	for domain, checkID := range checkByDomain {
		if hasStatus {
			if _, err := tx.Exec(`
				INSERT INTO monitor_check_status (
					check_id, domain, is_down, failure_count, consecutive_successes, current_mode,
					last_alert_time, last_checked, next_check_at, down_since, pd_triggered, pd_escalated
				)
				SELECT ?, lower(domain), is_down, failure_count, consecutive_successes, current_mode,
					last_alert_time, last_checked, next_check_at, down_since, pd_triggered, pd_escalated
				FROM monitor_status WHERE domain = ?
			`, checkID, domain); err != nil {
				return fmt.Errorf("failed to copy monitor status for %s: %w", domain, err)
			}
		}
		for _, table := range []string{"monitor_history", "incidents"} {
			if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET check_id = ? WHERE domain = ?`, table), checkID, domain); err != nil {
				return fmt.Errorf("failed to re-key %s for %s: %w", table, domain, err)
			}
		}
	}

	for _, table := range []string{"monitor_history", "incidents"} {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE check_id IS NULL`, table)); err != nil {
			return fmt.Errorf("failed to drop unmatched %s rows: %w", table, err)
		}
	}

	if hasStatus {
		if _, err := tx.Exec(`DROP TABLE monitor_status`); err != nil {
			return fmt.Errorf("failed to drop monitor_status: %w", err)
		}
	}
	return nil
}
