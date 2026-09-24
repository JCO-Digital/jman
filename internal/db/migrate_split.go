package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JCO-Digital/jman/internal/config"
	_ "modernc.org/sqlite"
)

// InventoryTables/APITables mirror the table-to-database assignment in
// internal/db/db.go's initInventorySchema/initAPISchema.
var (
	InventoryTables = []string{
		"plugin_info",
		"site_plugins",
		"site_core",
		"site_admin_user",
		"site_environment",
		"ignore_entries",
	}
	APITables = []string{
		"slack_messages",
		"monitor_status",
		"monitor_history",
		"organizations",
		"contacts",
		"site_organization_map",
		"payment_methods",
		"assets",
		"organization_assets",
		"asset_payments",
		"notes",
		"settings",
		"agent_tokens",
		"site_disk_usage",
		"site_wp_flags",
		"site_traffic_hourly",
		"site_traffic_daily",
		"tasks",
		"site_update_ledger",
		"incidents",
	}
)

// HasLegacyDB returns true if the legacy pre-split jman.db file exists.
func HasLegacyDB() bool {
	legacyPath := filepath.Join(config.RunData.DataDir, "jman.db")
	return fileExists(legacyPath)
}

// MigrateSplitDB splits the legacy single jman.db into inventory.db and api.db.
// It copies data out of the legacy jman.db file into the new split databases,
// verifies row counts match, then renames jman.db to jman.db.pre-split-backup.
//
// Safe to re-run if interrupted partway through: it skips any split database
// that already exists and only renames the legacy file once both are complete.
func MigrateSplitDB() error {
	legacyPath := filepath.Join(config.RunData.DataDir, "jman.db")
	inventoryPath := filepath.Join(config.RunData.DataDir, "inventory.db")
	apiPath := filepath.Join(config.RunData.DataDir, "api.db")

	legacyExists := fileExists(legacyPath)
	inventoryExists := fileExists(inventoryPath)
	apiExists := fileExists(apiPath)

	if !legacyExists {
		if inventoryExists && apiExists {
			fmt.Println("Already migrated: inventory.db and api.db exist, no legacy jman.db found.")
			return nil
		}
		return fmt.Errorf("no legacy jman.db found at %s, and the split databases are missing or incomplete — nothing to migrate", legacyPath)
	}

	fmt.Printf("Migrating %s into inventory.db and api.db...\n", legacyPath)

	// Checkpoint the legacy database's WAL into its main file first, so the
	// ATTACH-based copy below sees a complete, consistent snapshot rather
	// than missing whatever's still sitting in -wal.
	if err := checkpointLegacy(legacyPath); err != nil {
		return fmt.Errorf("failed to checkpoint legacy database: %w", err)
	}

	defer Close()

	if !inventoryExists {
		if err := migrateInto(legacyPath, InitInventory, GetInventoryDB, InventoryTables); err != nil {
			return fmt.Errorf("failed to migrate inventory data: %w", err)
		}
		fmt.Println("inventory.db created and populated.")
	} else {
		fmt.Println("inventory.db already exists — skipping (resuming a previous partial migration).")
	}

	if !apiExists {
		if err := migrateInto(legacyPath, InitAPI, GetAPIDB, APITables); err != nil {
			return fmt.Errorf("failed to migrate api data: %w", err)
		}
		fmt.Println("api.db created and populated.")
	} else {
		fmt.Println("api.db already exists — skipping (resuming a previous partial migration).")
	}

	if err := Close(); err != nil {
		return fmt.Errorf("failed to close databases before renaming: %w", err)
	}

	backupPath := legacyPath + ".pre-split-backup"
	_ = os.Remove(backupPath)
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(backupPath + suffix)
	}

	if err := os.Rename(legacyPath, backupPath); err != nil {
		return fmt.Errorf("failed to rename legacy database to %s: %w", backupPath, err)
	}
	// Move any WAL/SHM sidecar files out of the way too, so a stray one
	// can never be reopened alongside the renamed backup file.
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Rename(legacyPath+suffix, backupPath+suffix)
	}

	fmt.Printf("Migration complete. Legacy database preserved at %s (safe to delete once you've verified the split databases).\n", backupPath)
	return nil
}

func checkpointLegacy(path string) error {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("failed to open legacy database: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return err
	}
	return nil
}

// migrateInto creates and schema-initializes the new database (via initFn,
// which also opens it through the shared db package) and copies the given
// tables into it from the legacy database at legacyPath, verifying row
// counts as it goes.
func migrateInto(legacyPath string, initFn func() error, getConn func() *sql.DB, tables []string) error {
	if err := initFn(); err != nil {
		return err
	}

	conn := getConn()
	if conn == nil {
		return fmt.Errorf("database was not initialized")
	}

	escapedLegacy := strings.ReplaceAll(legacyPath, "'", "''")
	if _, err := conn.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS old", escapedLegacy)); err != nil {
		return fmt.Errorf("failed to attach legacy database: %w", err)
	}
	defer func() { _, _ = conn.Exec("DETACH DATABASE old") }()

	for _, table := range tables {
		if err := copyTable(conn, table); err != nil {
			return fmt.Errorf("table %s: %w", table, err)
		}
	}

	return nil
}

// copyTable copies every row of one table from the attached "old" schema
// into the corresponding table in conn's main database, using an explicit
// column list matching columns present in both schemas rather than SELECT *
// to avoid depending on column ordering matching between the two schemas.
// It verifies the destination row count matches the source afterward.
func copyTable(conn *sql.DB, table string) error {
	if !config.IsSafeIdentifier(table) {
		return fmt.Errorf("invalid table name: %s", table)
	}

	// If table doesn't exist in the legacy database, skip it.
	var tableExists int
	if err := conn.QueryRow("SELECT COUNT(*) FROM old.sqlite_master WHERE type='table' AND name = ?", table).Scan(&tableExists); err != nil {
		return fmt.Errorf("failed to check source table %s: %w", table, err)
	}
	if tableExists == 0 {
		return nil
	}

	var sourceCount int
	if err := conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM old.%s", table)).Scan(&sourceCount); err != nil {
		return fmt.Errorf("failed to count source rows: %w", err)
	}

	cols, err := commonColumnNames(conn, table)
	if err != nil {
		return fmt.Errorf("failed to read columns for %s: %w", table, err)
	}
	colsCSV := strings.Join(cols, ", ")

	// Idempotent: clear anything already copied (e.g. a partial copy from
	// an interrupted prior run before the destination file was considered
	// "existing" and thus skipped on resume) before re-inserting.
	if _, err := conn.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
		return fmt.Errorf("failed to clear destination table before copy: %w", err)
	}

	if sourceCount > 0 {
		copySQL := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM old.%s", table, colsCSV, colsCSV, table)
		if _, err := conn.Exec(copySQL); err != nil {
			return fmt.Errorf("failed to copy rows: %w", err)
		}
	}

	var destCount int
	if err := conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&destCount); err != nil {
		return fmt.Errorf("failed to count destination rows: %w", err)
	}

	if destCount != sourceCount {
		return fmt.Errorf("row count mismatch after copy: source had %d, destination has %d", sourceCount, destCount)
	}

	fmt.Printf("  %s: copied %d rows\n", table, destCount)
	return nil
}

// commonColumnNames returns the column names present in both old.table and main.table.
func commonColumnNames(conn *sql.DB, table string) ([]string, error) {
	oldCols, err := columnNames(conn, "old", table)
	if err != nil {
		return nil, fmt.Errorf("failed to read source columns: %w", err)
	}
	newCols, err := columnNames(conn, "main", table)
	if err != nil {
		return nil, fmt.Errorf("failed to read destination columns: %w", err)
	}
	newColSet := make(map[string]bool, len(newCols))
	for _, c := range newCols {
		newColSet[c] = true
	}
	var common []string
	for _, c := range oldCols {
		if newColSet[c] {
			common = append(common, c)
		}
	}
	if len(common) == 0 {
		return nil, fmt.Errorf("table %s has no overlapping columns between legacy and new schema", table)
	}
	return common, nil
}

// columnNames returns a table's column names, in schema-declared order, by
// querying the given attached schema (e.g. "old" or "main") via PRAGMA table_info.
func columnNames(conn *sql.DB, schema, table string) ([]string, error) {
	rows, err := conn.Query(fmt.Sprintf("PRAGMA %s.table_info(%s)", schema, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var cid int
		var name, typeName string
		var notnull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &typeName, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table has no columns")
	}
	return cols, rows.Err()
}
