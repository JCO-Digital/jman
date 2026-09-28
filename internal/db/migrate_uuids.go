package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/utils"
)

// MigrateLegacyInventoryIDs converts any legacy integer site_ids in inventory.db
// to deterministic SpinupWP UUIDs. It is idempotent and safe to run multiple times.
func MigrateLegacyInventoryIDs(dbConn *sql.DB) error {
	if dbConn == nil {
		return nil
	}

	siteTables := []string{
		"site_plugins",
		"site_core",
		"site_admin_user",
		"site_environment",
	}

	for _, table := range siteTables {
		if err := migrateTableSiteIDs(dbConn, table); err != nil {
			return fmt.Errorf("failed to migrate site_ids in %s: %w", table, err)
		}
	}

	// Migrate ignore_entries
	if err := migrateIgnoreEntries(dbConn); err != nil {
		return fmt.Errorf("failed to migrate ignore_entries: %w", err)
	}

	return nil
}

// MigrateLegacyAPIIDs converts any legacy integer site_ids and server_ids in api.db
// to deterministic SpinupWP UUIDs. It is idempotent and safe to run multiple times.
func MigrateLegacyAPIIDs(dbConn *sql.DB) error {
	if dbConn == nil {
		return nil
	}

	siteTables := []string{
		"site_disk_usage",
		"site_wp_flags",
		"site_traffic_hourly",
		"site_traffic_daily",
		"site_update_ledger",
		"site_organization_map",
		"organization_assets",
	}

	for _, table := range siteTables {
		if err := migrateTableSiteIDs(dbConn, table); err != nil {
			return fmt.Errorf("failed to migrate site_ids in %s: %w", table, err)
		}
	}

	// Migrate tasks (both site_id and server_id)
	if err := migrateTasksIDs(dbConn); err != nil {
		return fmt.Errorf("failed to migrate tasks: %w", err)
	}

	// Migrate agent_tokens (server_id)
	if err := migrateAgentTokensServerIDs(dbConn); err != nil {
		return fmt.Errorf("failed to migrate agent_tokens: %w", err)
	}

	return nil
}

// migrateTableSiteIDs finds any non-UUID site_ids in the given table and converts them.
func migrateTableSiteIDs(dbConn *sql.DB, tableName string) error {
	// Check if table exists
	var count int
	err := dbConn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	// Select all distinct site_ids that are not 36-character UUIDs
	rows, err := dbConn.Query(fmt.Sprintf(
		`SELECT DISTINCT site_id FROM %s WHERE site_id IS NOT NULL AND length(site_id) < 30`,
		tableName,
	))
	if err != nil {
		return err
	}
	defer rows.Close()

	var legacyIDs []int
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		if id, err := strconv.Atoi(raw); err == nil && id > 0 {
			legacyIDs = append(legacyIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range legacyIDs {
		uuidStr := utils.SpinupWPSiteUUID(id)
		_, err := dbConn.Exec(
			fmt.Sprintf(`UPDATE OR IGNORE %s SET site_id = ? WHERE site_id = ?`, tableName),
			uuidStr, id,
		)
		if err != nil {
			return fmt.Errorf("failed to update site_id %d in %s: %w", id, tableName, err)
		}
		// Clean up any remaining legacy rows that were ignored due to unique conflict
		// with an already-migrated UUID row.
		_, _ = dbConn.Exec(
			fmt.Sprintf(`DELETE FROM %s WHERE site_id = ?`, tableName),
			id,
		)
	}

	return nil
}

func migrateIgnoreEntries(dbConn *sql.DB) error {
	var count int
	err := dbConn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ignore_entries'").Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	// 1. Target sites
	rows, err := dbConn.Query(`SELECT id, target FROM ignore_entries WHERE type = 'site' AND length(target) < 30`)
	if err == nil {
		type entry struct {
			id     int
			target string
		}
		var entries []entry
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.id, &e.target); err == nil {
				entries = append(entries, e)
			}
		}
		rows.Close()

		for _, e := range entries {
			if numID, err := strconv.Atoi(e.target); err == nil && numID > 0 {
				newUUID := utils.SpinupWPSiteUUID(numID)
				_, _ = dbConn.Exec(`UPDATE ignore_entries SET target = ? WHERE id = ?`, newUUID, e.id)
			}
		}
	}

	// 2. Target servers
	rows, err = dbConn.Query(`SELECT id, target FROM ignore_entries WHERE type = 'server' AND length(target) < 30`)
	if err == nil {
		type entry struct {
			id     int
			target string
		}
		var entries []entry
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.id, &e.target); err == nil {
				entries = append(entries, e)
			}
		}
		rows.Close()

		for _, e := range entries {
			if numID, err := strconv.Atoi(e.target); err == nil && numID > 0 {
				newUUID := utils.SpinupWPServerUUID(numID)
				_, _ = dbConn.Exec(`UPDATE ignore_entries SET target = ? WHERE id = ?`, newUUID, e.id)
			}
		}
	}

	// 3. Negated site IDs (JSON array)
	rows, err = dbConn.Query(`SELECT id, negated_site_ids FROM ignore_entries WHERE negated_site_ids IS NOT NULL AND negated_site_ids != '' AND negated_site_ids != '[]'`)
	if err == nil {
		type negEntry struct {
			id     int
			rawIDs string
		}
		var entries []negEntry
		for rows.Next() {
			var e negEntry
			if err := rows.Scan(&e.id, &e.rawIDs); err == nil {
				entries = append(entries, e)
			}
		}
		rows.Close()

		for _, e := range entries {
			var intIDs []int
			if err := json.Unmarshal([]byte(e.rawIDs), &intIDs); err == nil && len(intIDs) > 0 {
				uuidStrings := make([]string, len(intIDs))
				for i, id := range intIDs {
					uuidStrings[i] = utils.SpinupWPSiteUUID(id)
				}
				newJSON, _ := json.Marshal(uuidStrings)
				_, _ = dbConn.Exec(`UPDATE ignore_entries SET negated_site_ids = ? WHERE id = ?`, string(newJSON), e.id)
			}
		}
	}

	return nil
}

func migrateTasksIDs(dbConn *sql.DB) error {
	var count int
	err := dbConn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	// Site IDs in tasks
	rows, err := dbConn.Query(`SELECT DISTINCT site_id FROM tasks WHERE site_id IS NOT NULL AND length(site_id) < 30`)
	if err == nil {
		var siteIDs []int
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err == nil {
				if id, err := strconv.Atoi(raw); err == nil && id > 0 {
					siteIDs = append(siteIDs, id)
				}
			}
		}
		rows.Close()

		for _, id := range siteIDs {
			newUUID := utils.SpinupWPSiteUUID(id)
			_, _ = dbConn.Exec(`UPDATE tasks SET site_id = ? WHERE site_id = ?`, newUUID, id)
		}
	}

	// Server IDs in tasks
	rows, err = dbConn.Query(`SELECT DISTINCT server_id FROM tasks WHERE server_id IS NOT NULL AND length(server_id) < 30`)
	if err == nil {
		var serverIDs []int
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err == nil {
				if id, err := strconv.Atoi(raw); err == nil && id > 0 {
					serverIDs = append(serverIDs, id)
				}
			}
		}
		rows.Close()

		for _, id := range serverIDs {
			newUUID := utils.SpinupWPServerUUID(id)
			_, _ = dbConn.Exec(`UPDATE tasks SET server_id = ? WHERE server_id = ?`, newUUID, id)
		}
	}

	return nil
}

func migrateAgentTokensServerIDs(dbConn *sql.DB) error {
	var count int
	err := dbConn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='agent_tokens'").Scan(&count)
	if err != nil || count == 0 {
		return nil
	}

	rows, err := dbConn.Query(`SELECT DISTINCT server_id FROM agent_tokens WHERE server_id IS NOT NULL AND length(server_id) < 30`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var serverIDs []int
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err == nil {
			if id, err := strconv.Atoi(raw); err == nil && id > 0 {
				serverIDs = append(serverIDs, id)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range serverIDs {
		newUUID := utils.SpinupWPServerUUID(id)
		_, _ = dbConn.Exec(`UPDATE agent_tokens SET server_id = ? WHERE server_id = ?`, newUUID, id)
	}

	return nil
}
