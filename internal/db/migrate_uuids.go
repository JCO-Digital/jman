package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/utils"
)

// uuidMigrationName is the schema_migrations key recording that a database's
// legacy SpinupWP integer site/server IDs have been rewritten to UUIDs.
const uuidMigrationName = "legacy_ids_to_uuids"

// idColumn describes one column holding site or server identifiers.
type idColumn struct {
	table  string
	column string
	toUUID func(int) string
	// where optionally restricts which rows hold an identifier (e.g. only
	// notes attached to sites).
	where string
}

var inventoryIDColumns = []idColumn{
	{table: "site_plugins", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_core", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_admin_user", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_environment", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "ignore_entries", column: "target", toUUID: utils.SpinupWPSiteUUID, where: "type = 'site'"},
	{table: "ignore_entries", column: "target", toUUID: utils.SpinupWPServerUUID, where: "type = 'server'"},
}

var apiIDColumns = []idColumn{
	{table: "site_disk_usage", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_wp_flags", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_traffic_hourly", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_traffic_daily", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_update_ledger", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "site_organization_map", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "organization_assets", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "tasks", column: "site_id", toUUID: utils.SpinupWPSiteUUID},
	{table: "tasks", column: "server_id", toUUID: utils.SpinupWPServerUUID},
	{table: "agent_tokens", column: "server_id", toUUID: utils.SpinupWPServerUUID},
	{table: "notes", column: "parent_id", toUUID: utils.SpinupWPSiteUUID, where: "parent_type = 'Site'"},
}

// MigrateLegacyInventoryIDs rewrites legacy SpinupWP integer site/server IDs
// in inventory.db to their deterministic UUIDs. It runs once per database
// (recorded in schema_migrations); later calls are no-ops.
func MigrateLegacyInventoryIDs(dbConn *sql.DB) error {
	return runOnce(dbConn, uuidMigrationName, func(tx *sql.Tx) error {
		if err := migrateIDColumns(tx, inventoryIDColumns); err != nil {
			return err
		}
		return migrateNegatedSiteIDs(tx)
	})
}

// MigrateLegacyAPIIDs rewrites legacy SpinupWP integer site/server IDs in
// api.db to their deterministic UUIDs. It runs once per database (recorded
// in schema_migrations); later calls are no-ops.
func MigrateLegacyAPIIDs(dbConn *sql.DB) error {
	return runOnce(dbConn, uuidMigrationName, func(tx *sql.Tx) error {
		return migrateIDColumns(tx, apiIDColumns)
	})
}

// runOnce applies fn in a single transaction unless name is already recorded
// in the database's schema_migrations table, and records it on success.
func runOnce(dbConn *sql.DB, name string, fn func(tx *sql.Tx) error) error {
	if dbConn == nil {
		return nil
	}

	if _, err := dbConn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("failed to create schema_migrations: %w", err)
	}

	var applied int
	if err := dbConn.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&applied); err != nil {
		return fmt.Errorf("failed to check migration %s: %w", name, err)
	}
	if applied > 0 {
		return nil
	}

	tx, err := dbConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return fmt.Errorf("migration %s failed: %w", name, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("failed to record migration %s: %w", name, err)
	}
	return tx.Commit()
}

// parseLegacyID interprets a stored identifier as a legacy SpinupWP integer
// ID. Besides plain integers ("123") it accepts the "123.0" form that JSON
// numbers decoded as float64 end up as when written to a TEXT column.
func parseLegacyID(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || utils.IsValidUUID(raw) {
		return 0, false
	}
	if id, err := strconv.Atoi(raw); err == nil {
		if id <= 0 {
			return 0, false
		}
		return id, true
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f <= 0 || f != math.Trunc(f) || f > math.MaxInt32 {
		return 0, false
	}
	return int(f), true
}

func tableExists(tx *sql.Tx, table string) (bool, error) {
	var count int
	err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count)
	return count > 0, err
}

func migrateIDColumns(tx *sql.Tx, columns []idColumn) error {
	for _, col := range columns {
		if err := migrateIDColumn(tx, col); err != nil {
			return fmt.Errorf("%s.%s: %w", col.table, col.column, err)
		}
	}
	return nil
}

// migrateIDColumn rewrites every legacy integer value in one column to its
// UUID. UPDATE OR REPLACE resolves unique-key collisions with a row that
// already carries the UUID in favour of the legacy row: in a database that
// was partially migrated by an earlier release, rows still keyed by the
// integer are the ones written most recently.
func migrateIDColumn(tx *sql.Tx, col idColumn) error {
	exists, err := tableExists(tx, col.table)
	if err != nil || !exists {
		return err
	}

	where := fmt.Sprintf("%s IS NOT NULL", col.column)
	if col.where != "" {
		where += " AND " + col.where
	}

	rows, err := tx.Query(fmt.Sprintf(`SELECT DISTINCT CAST(%s AS TEXT) FROM %s WHERE %s`, col.column, col.table, where))
	if err != nil {
		return err
	}
	var raws []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		raws = append(raws, raw)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// Every id column has TEXT affinity by the time this runs (migrateTable
	// has recreated legacy INTEGER columns as TEXT), so stored values compare
	// equal to their text form and a plain equality can use the column's index.
	update := fmt.Sprintf(`UPDATE OR REPLACE %s SET %s = ? WHERE %s = ? AND %s`, col.table, col.column, col.column, where)
	for _, raw := range raws {
		id, ok := parseLegacyID(raw)
		if !ok {
			continue
		}
		if _, err := tx.Exec(update, col.toUUID(id), raw); err != nil {
			return fmt.Errorf("failed to migrate %q: %w", raw, err)
		}
	}
	return nil
}

// migrateNegatedSiteIDs rewrites ignore_entries.negated_site_ids JSON arrays
// of legacy integer site IDs to arrays of site UUIDs.
func migrateNegatedSiteIDs(tx *sql.Tx) error {
	exists, err := tableExists(tx, "ignore_entries")
	if err != nil || !exists {
		return err
	}

	rows, err := tx.Query(`SELECT id, negated_site_ids FROM ignore_entries
		WHERE negated_site_ids IS NOT NULL AND negated_site_ids NOT IN ('', '[]', 'null')`)
	if err != nil {
		return err
	}
	type entry struct {
		id  int
		raw string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.raw); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, e := range entries {
		// Elements may be numbers (legacy) or strings (UUIDs, or numeric
		// strings), so decode loosely and normalise each one.
		var values []any
		if err := json.Unmarshal([]byte(e.raw), &values); err != nil {
			return fmt.Errorf("ignore entry %d has invalid negated_site_ids %q: %w", e.id, e.raw, err)
		}
		uuids := make([]string, 0, len(values))
		for _, v := range values {
			raw := strings.TrimSpace(fmt.Sprint(v))
			if id, ok := parseLegacyID(raw); ok {
				uuids = append(uuids, utils.SpinupWPSiteUUID(id))
			} else if utils.IsValidUUID(raw) {
				uuids = append(uuids, raw)
			}
		}
		encoded, err := json.Marshal(uuids)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE ignore_entries SET negated_site_ids = ? WHERE id = ?`, string(encoded), e.id); err != nil {
			return fmt.Errorf("failed to migrate negated_site_ids for ignore entry %d: %w", e.id, err)
		}
	}
	return nil
}
