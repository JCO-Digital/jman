package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/JCO-Digital/jman/internal/config"
	_ "modernc.org/sqlite"
)

// jman's data is split across two SQLite databases:
//
//   - inventory.db holds refreshable inventory data (plugin/site/core
//     versions, ignore rules) that both the `jman` CLI and `jman-api` read
//     and write.
//   - api.db holds jman-api's own business data (organizations, billing,
//     tasks, monitor state, agent tokens, traffic, ...) that only jman-api
//     (and, transitionally, a standalone jman-monitor) ever touches.
//
// GetInventoryDB/GetAPIDB are deliberately not aliased to each other, so
// every call site must be explicit about which database it means.
var (
	inventoryDB *sql.DB
	apiDB       *sql.DB
	dbMutex     sync.Mutex
)

// TableDefinition represents the desired state of a database table.
type TableDefinition struct {
	Name       string
	Columns    map[string]string // Column name -> SQL type and constraints
	PrimaryKey []string          // Optional composite primary key
}

// CheckSplitState returns an error if the data directory is in an
// inconsistent, partially-migrated state: the legacy pre-split jman.db file
// coexists with either of the new split database files. Every binary
// (jman, jman-api, jman-monitor) should call this before InitInventory/
// InitAPI so a partial or interrupted migration is never silently ignored.
func CheckSplitState() error {
	legacyPath := filepath.Join(config.RunData.DataDir, "jman.db")
	inventoryPath := filepath.Join(config.RunData.DataDir, "inventory.db")
	apiPath := filepath.Join(config.RunData.DataDir, "api.db")

	if fileExists(legacyPath) && (fileExists(inventoryPath) || fileExists(apiPath)) {
		return fmt.Errorf(
			"both the legacy database %s and a split database file exist in %s — "+
				"this looks like an interrupted migration; run `jman-api migrate-db` to "+
				"resolve it, or restore from backup, before starting any jman binary",
			legacyPath, config.RunData.DataDir,
		)
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// openDB opens a SQLite database file with the pragma set jman relies on
// for concurrency and reliability (shared by both inventory.db and api.db).
func openDB(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Limit to a single connection to avoid "database is locked" errors.
	// SQLite works best with a single connection when performing concurrent writes.
	conn.SetMaxOpenConns(1)

	// Set pragmas for better concurrency and reliability.
	// WAL mode allows multiple readers and one writer simultaneously.
	// Busy timeout ensures it retries before failing with SQLITE_BUSY.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to set pragma %q: %w", p, err)
		}
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return conn, nil
}

// InitInventory initializes the shared SQLite inventory database in the data
// directory, creating it if it doesn't exist. This is the database jman's
// own CLI commands read and write.
func InitInventory() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if inventoryDB != nil {
		return nil
	}

	dbPath := filepath.Join(config.RunData.DataDir, "inventory.db")
	conn, err := openDB(dbPath)
	if err != nil {
		return err
	}

	inventoryDB = conn
	if err := initInventorySchema(); err != nil {
		conn.Close()
		inventoryDB = nil
		return fmt.Errorf("failed to initialize inventory schema: %w", err)
	}

	return nil
}

// InitAPI initializes jman-api's own SQLite database in the data directory,
// creating it if it doesn't exist. Only jman-api (and, transitionally, a
// standalone jman-monitor) needs to call this.
func InitAPI() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if apiDB != nil {
		return nil
	}

	dbPath := filepath.Join(config.RunData.DataDir, "api.db")
	conn, err := openDB(dbPath)
	if err != nil {
		return err
	}

	apiDB = conn
	if err := initAPISchema(); err != nil {
		conn.Close()
		apiDB = nil
		return fmt.Errorf("failed to initialize api schema: %w", err)
	}

	return nil
}

// GetInventoryDB returns the shared inventory database instance.
func GetInventoryDB() *sql.DB {
	return inventoryDB
}

// GetAPIDB returns jman-api's own database instance.
func GetAPIDB() *sql.DB {
	return apiDB
}

// BackupInventory creates a snapshot of the inventory database using VACUUM INTO.
func BackupInventory(destPath string) error {
	return backupDB(inventoryDB, destPath)
}

// BackupAPI creates a snapshot of the api database using VACUUM INTO.
func BackupAPI(destPath string) error {
	return backupDB(apiDB, destPath)
}

func backupDB(conn *sql.DB, destPath string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if conn == nil {
		return fmt.Errorf("database not initialized")
	}

	// SQLite's VACUUM INTO requires the target file to NOT exist.
	// We escape single quotes in the path just in case.
	escapedPath := strings.ReplaceAll(destPath, "'", "''")
	query := fmt.Sprintf("VACUUM INTO '%s'", escapedPath)

	if _, err := conn.Exec(query); err != nil {
		return fmt.Errorf("VACUUM INTO failed: %w", err)
	}

	return nil
}

// Close closes whichever database handles have been opened. Safe to call
// even if only one (or neither) of InitInventory/InitAPI was ever called.
func Close() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	var firstErr error
	if inventoryDB != nil {
		if err := inventoryDB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		inventoryDB = nil
	}
	if apiDB != nil {
		if err := apiDB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		apiDB = nil
	}
	return firstErr
}

// initInventorySchema creates/migrates the tables that live in inventory.db:
// refreshable plugin/site/core inventory data, plus the unified ignore list
// (used by both `jman vuln`, running standalone, and jman-api's schedulers).
func initInventorySchema() error {
	tables := []TableDefinition{
		{
			Name: "plugin_info",
			Columns: map[string]string{
				"slug":           "TEXT PRIMARY KEY",
				"name":           "TEXT",
				"version":        "TEXT",
				"author":         "TEXT",
				"author_profile": "TEXT",
				"requires":       "TEXT",
				"tested":         "TEXT",
				"last_updated":   "TEXT",
				"homepage":       "TEXT",
				"fetched_at":     "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "site_plugins",
			Columns: map[string]string{
				"site_id":          "INTEGER NOT NULL",
				"slug":             "TEXT NOT NULL",
				"status":           "TEXT",
				"version":          "TEXT",
				"update_available": "TEXT",
				"auto_update":      "BOOLEAN",
				"updated_at":       "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
			PrimaryKey: []string{"site_id", "slug"},
		},
		{
			Name: "site_core",
			Columns: map[string]string{
				"site_id":    "INTEGER PRIMARY KEY",
				"version":    "TEXT NOT NULL",
				"updated_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "site_environment",
			Columns: map[string]string{
				"site_id":     "INTEGER PRIMARY KEY",
				"environment": "TEXT NOT NULL",
				"updated_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":  "TEXT",
			},
		},
		{
			Name: "ignore_entries",
			Columns: map[string]string{
				"id":               "INTEGER PRIMARY KEY AUTOINCREMENT",
				"type":             "TEXT NOT NULL",
				"target":           "TEXT NOT NULL",
				"reason":           "TEXT",
				"negated_site_ids": "TEXT",
				"use_for_monitor":  "BOOLEAN DEFAULT 0",
				"use_for_vuln":     "BOOLEAN DEFAULT 0",
				"created_at":       "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":       "TEXT",
				"updated_at":       "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":       "TEXT",
			},
		},
	}

	// Drop old ignore tables if they exist (pre-dates the unified ignore_entries table).
	_, _ = inventoryDB.Exec("DROP TABLE IF EXISTS monitor_ignored_sites")
	_, _ = inventoryDB.Exec("DROP TABLE IF EXISTS monitor_ignored_history")
	_, _ = inventoryDB.Exec("DROP TABLE IF EXISTS vuln_ignored")

	for _, table := range tables {
		if err := migrateTable(inventoryDB, table); err != nil {
			return fmt.Errorf("failed to migrate table %s: %w", table.Name, err)
		}
	}

	return nil
}

// initAPISchema creates/migrates the tables that live in api.db: jman-api's
// own business data, exclusively owned by jman-api (and, transitionally, a
// standalone jman-monitor for the monitor_status/monitor_history tables).
func initAPISchema() error {
	tables := []TableDefinition{
		{
			Name: "slack_messages",
			Columns: map[string]string{
				"hash":      "TEXT PRIMARY KEY",
				"timestamp": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"channel":   "TEXT",
			},
		},
		{
			Name: "monitor_status",
			Columns: map[string]string{
				"domain":                "TEXT PRIMARY KEY COLLATE NOCASE",
				"is_down":               "BOOLEAN DEFAULT 0",
				"failure_count":         "INTEGER DEFAULT 0",
				"consecutive_successes": "INTEGER DEFAULT 0",
				"current_mode":          "TEXT DEFAULT 'normal'",
				"last_alert_time":       "DATETIME",
				"last_checked":          "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"next_check_at":         "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "monitor_history",
			Columns: map[string]string{
				"id":         "INTEGER PRIMARY KEY AUTOINCREMENT",
				"domain":     "TEXT COLLATE NOCASE",
				"status":     "TEXT",
				"error_code": "INTEGER",
				"first_seen": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"last_seen":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"count":      "INTEGER DEFAULT 1",
			},
		},
		{
			Name: "organizations",
			Columns: map[string]string{
				"id":         "INTEGER PRIMARY KEY AUTOINCREMENT",
				"name":       "TEXT NOT NULL",
				"vat_number": "TEXT",
				"info":       "TEXT",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by": "TEXT",
				"updated_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by": "TEXT",
			},
		},
		{
			Name: "contacts",
			Columns: map[string]string{
				"id":              "INTEGER PRIMARY KEY AUTOINCREMENT",
				"organization_id": "INTEGER REFERENCES organizations(id) ON DELETE CASCADE",
				"name":            "TEXT NOT NULL",
				"email":           "TEXT",
				"phone":           "TEXT",
				"type":            "TEXT",
				"created_at":      "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":      "TEXT",
				"updated_at":      "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":      "TEXT",
			},
		},
		{
			Name: "site_organization_map",
			Columns: map[string]string{
				"site_id":         "INTEGER NOT NULL",
				"organization_id": "INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE",
				"created_at":      "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":      "TEXT",
			},
			PrimaryKey: []string{"site_id", "organization_id"},
		},
		{
			Name: "payment_methods",
			Columns: map[string]string{
				"id":          "INTEGER PRIMARY KEY AUTOINCREMENT",
				"name":        "TEXT NOT NULL",
				"type":        "TEXT NOT NULL",
				"expiry_date": "DATETIME",
				"created_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":  "TEXT",
				"updated_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":  "TEXT",
			},
		},
		{
			Name: "assets",
			Columns: map[string]string{
				"id":                 "INTEGER PRIMARY KEY AUTOINCREMENT",
				"type":               "TEXT NOT NULL",
				"identifier":         "TEXT",
				"name":               "TEXT NOT NULL",
				"description":        "TEXT",
				"default_price":      "INTEGER",
				"default_freq":       "TEXT",
				"active":             "BOOLEAN DEFAULT 1",
				"payment_method_id":  "INTEGER REFERENCES payment_methods(id) ON DELETE SET NULL",
				"purchase_price":     "INTEGER DEFAULT 0",
				"quantity":           "INTEGER DEFAULT 1",
				"next_payment":       "DATETIME",
				"management_url":     "TEXT DEFAULT ''",
				"management_account": "TEXT DEFAULT ''",
				"license_key":        "TEXT DEFAULT ''",
				"created_at":         "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":         "TEXT",
				"updated_at":         "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":         "TEXT",
			},
		},
		{
			Name: "organization_assets",
			Columns: map[string]string{
				"id":                "INTEGER PRIMARY KEY AUTOINCREMENT",
				"organization_id":   "INTEGER REFERENCES organizations(id) ON DELETE CASCADE",
				"site_id":           "INTEGER",
				"asset_id":          "INTEGER REFERENCES assets(id) ON DELETE SET NULL",
				"identifier":        "TEXT",
				"price":             "INTEGER",
				"billing_freq":      "TEXT",
				"next_billing":      "DATETIME",
				"status":            "TEXT DEFAULT 'active'",
				"description":       "TEXT",
				"payment_method_id": "INTEGER REFERENCES payment_methods(id) ON DELETE SET NULL",
				"license_key":       "TEXT DEFAULT ''",
				"created_at":        "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":        "TEXT",
				"updated_at":        "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":        "TEXT",
			},
		},
		{
			Name: "asset_payments",
			Columns: map[string]string{
				"id":           "INTEGER PRIMARY KEY AUTOINCREMENT",
				"org_asset_id": "INTEGER REFERENCES organization_assets(id) ON DELETE CASCADE",
				"amount":       "INTEGER",
				"payment_date": "DATETIME",
				"info":         "TEXT",
				"created_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":   "TEXT",
			},
		},
		{
			Name: "notes",
			Columns: map[string]string{
				"id":          "INTEGER PRIMARY KEY AUTOINCREMENT",
				"parent_type": "TEXT",
				"parent_id":   "TEXT",
				"content":     "TEXT",
				"created_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":  "TEXT",
				"updated_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":  "TEXT",
			},
		},
		{
			Name: "settings",
			Columns: map[string]string{
				"user_id":    "TEXT NOT NULL",
				"key":        "TEXT NOT NULL",
				"value":      "TEXT NOT NULL",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
			PrimaryKey: []string{"user_id", "key"},
		},
		{
			Name: "agent_tokens",
			Columns: map[string]string{
				"id":            "INTEGER PRIMARY KEY AUTOINCREMENT",
				"server_id":     "INTEGER NOT NULL",
				"server_name":   "TEXT",
				"token_hash":    "TEXT NOT NULL",
				"token_prefix":  "TEXT NOT NULL",
				"description":   "TEXT",
				"revoked":       "BOOLEAN DEFAULT 0",
				"last_seen_at":  "DATETIME",
				"agent_version": "TEXT",
				"created_at":    "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":    "TEXT",
				// stale_alert_sent_at tracks the current staleness alert
				// episode (see internal/tasks/agent_health.go): set when a
				// "hasn't reported in" Slack alert is sent, cleared once the
				// agent reports again — so the next time it goes stale, a
				// fresh alert fires instead of being silently deduped by
				// slack_messages' permanent exact-text matching.
				"stale_alert_sent_at": "DATETIME",
			},
		},
		{
			Name: "site_disk_usage",
			Columns: map[string]string{
				"site_id":     "INTEGER NOT NULL",
				"bytes_used":  "INTEGER NOT NULL",
				"measured_at": "DATETIME NOT NULL",
				"created_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
			PrimaryKey: []string{"site_id", "measured_at"},
		},
		{
			Name: "site_wp_flags",
			Columns: map[string]string{
				"site_id":            "INTEGER PRIMARY KEY",
				"is_multisite":       "BOOLEAN DEFAULT 0",
				"disallow_file_mods": "BOOLEAN DEFAULT 0",
				"updated_at":         "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "site_traffic_hourly",
			Columns: map[string]string{
				"site_id":         "INTEGER NOT NULL",
				"hour":            "DATETIME NOT NULL",
				"requests_total":  "INTEGER DEFAULT 0",
				"requests_human":  "INTEGER DEFAULT 0",
				"requests_bot":    "INTEGER DEFAULT 0",
				"unique_visitors": "INTEGER DEFAULT 0",
				"top_pages":       "TEXT",
				"top_referrers":   "TEXT",
				// DEFAULT '' (not just nullable TEXT) matters here: the
				// migrateTable recreate-and-copy migration only copies
				// columns common to the old and new schema, so pre-existing
				// rows get this newly-added column filled from its column
				// default — without one they'd be NULL, and every existing
				// row-scan in site_traffic_repo.go scans this column
				// straight into a Go string, which errors on NULL.
				"status_codes": "TEXT DEFAULT ''",
				"updated_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
			PrimaryKey: []string{"site_id", "hour"},
		},
		{
			Name: "site_traffic_daily",
			Columns: map[string]string{
				"site_id":         "INTEGER NOT NULL",
				"day":             "DATE NOT NULL",
				"requests_total":  "INTEGER DEFAULT 0",
				"requests_human":  "INTEGER DEFAULT 0",
				"requests_bot":    "INTEGER DEFAULT 0",
				"unique_visitors": "INTEGER DEFAULT 0",
				"top_pages":       "TEXT",
				"top_referrers":   "TEXT",
				"status_codes":    "TEXT DEFAULT ''", // see site_traffic_hourly's status_codes comment
				// finalized_at is set exactly once, by
				// FinalizeCompletedDailyRollups, the first time this day is
				// recomputed after fully closing (as opposed to the
				// continuous intraday recomputes AgentReportHandler triggers
				// while the day is still in progress, which leave this NULL).
				// RecomputeSiteTrafficDaily's upsert deliberately never
				// touches this column, so it's safe to call from either path
				// without one clobbering the other's signal. Deliberately
				// nullable with no DEFAULT: pre-existing rows (and any day
				// whose hourly source rows are already gone) simply stay
				// NULL, which correctly excludes them from
				// PruneOldSiteTrafficHourly's finalized-only deletion check.
				"finalized_at": "DATETIME",
				"updated_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
			PrimaryKey: []string{"site_id", "day"},
		},
		{
			Name: "tasks",
			Columns: map[string]string{
				"id":               "INTEGER PRIMARY KEY AUTOINCREMENT",
				"type":             "TEXT NOT NULL",
				"status":           "TEXT NOT NULL DEFAULT 'pending'",
				"priority":         "TEXT NOT NULL DEFAULT 'medium'",
				"title":            "TEXT NOT NULL",
				"description":      "TEXT",
				"site_id":          "INTEGER",
				"server_id":        "INTEGER",
				"organization_id":  "INTEGER",
				"plugin_slug":      "TEXT",
				"assigned_to":      "TEXT",
				"metadata":         "TEXT",
				"interval":         "TEXT",
				"due_date":         "DATETIME",
				"reminder_date":    "DATETIME",
				"created_at":       "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"completed_at":     "DATETIME",
				"completed_by":     "TEXT",
				"last_notified_at": "DATETIME",
				"created_by":       "TEXT",
				"updated_at":       "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "site_update_ledger",
			Columns: map[string]string{
				"id":          "INTEGER PRIMARY KEY AUTOINCREMENT",
				"site_id":     "INTEGER NOT NULL",
				"update_type": "TEXT NOT NULL",
				"status":      "TEXT NOT NULL",
				"data_json":   "TEXT",
				"updated_by":  "TEXT",
				"updated_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
	}

	for _, table := range tables {
		if err := migrateTable(apiDB, table); err != nil {
			return fmt.Errorf("failed to migrate table %s: %w", table.Name, err)
		}
	}

	// Manual index management
	_, err := apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_monitor_history_domain ON monitor_history(domain);")
	if err != nil {
		return err
	}

	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_contacts_organization_id ON contacts(organization_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_notes_parent ON notes(parent_type, parent_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_organization_assets_org_id ON organization_assets(organization_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_asset_payments_asset_id ON asset_payments(org_asset_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_assets_payment_method_id ON assets(payment_method_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_organization_assets_payment_method_id ON organization_assets(payment_method_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_tasks_site_id ON tasks(site_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_tasks_completed_by ON tasks(completed_by);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_agent_tokens_server_id ON agent_tokens(server_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_site_disk_usage_site_id ON site_disk_usage(site_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_site_traffic_hourly_site_id ON site_traffic_hourly(site_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_site_traffic_daily_site_id ON site_traffic_daily(site_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_site_update_ledger_site_id ON site_update_ledger(site_id);")
	return err
}

// migrateTable compares the current database table schema with the desired definition.
// SQLite has restrictions on ALTER TABLE (e.g., adding columns with non-constant defaults like CURRENT_TIMESTAMP).
// To be robust, this implementation uses the "recreate and copy" pattern if changes are detected.
func migrateTable(conn *sql.DB, def TableDefinition) error {
	// Disable foreign keys during migration to avoid broken references when renaming tables.
	// PRAGMA foreign_keys must be set outside of a transaction.
	if _, err := conn.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("failed to disable foreign keys: %w", err)
	}
	defer func() {
		_, _ = conn.Exec("PRAGMA foreign_keys=ON")
	}()

	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Check if the table exists
	if !config.IsSafeIdentifier(def.Name) {
		return fmt.Errorf("invalid table name: %s", def.Name)
	}

	var exists bool
	query := fmt.Sprintf("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='%s'", def.Name)
	err = tx.QueryRow(query).Scan(&exists)
	if err != nil {
		return err
	}

	// Prepare column strings for the desired state (sorted for deterministic output)
	colNames := make([]string, 0, len(def.Columns))
	for name := range def.Columns {
		if !config.IsSafeIdentifier(name) {
			return fmt.Errorf("invalid column name: %s", name)
		}
		colNames = append(colNames, name)
	}
	sort.Strings(colNames)

	newColsList := make([]string, 0, len(colNames))
	for _, name := range colNames {
		newColsList = append(newColsList, fmt.Sprintf("%s %s", name, def.Columns[name]))
	}
	if len(def.PrimaryKey) > 0 {
		newColsList = append(newColsList, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(def.PrimaryKey, ", ")))
	}
	newColsSQL := strings.Join(newColsList, ", ")

	if !exists {
		createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", def.Name, newColsSQL)
		if _, err := tx.Exec(createSQL); err != nil {
			return err
		}
		return tx.Commit()
	}

	// 2. Table exists, fetch current columns
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info('%s')", def.Name))
	if err != nil {
		return err
	}
	defer rows.Close()

	currentCols := make(map[string]int) // name -> notnull
	currentPK := make(map[string]int)   // name -> pk index
	for rows.Next() {
		var cid int
		var name, typeName string
		var notnull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &typeName, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		currentCols[name] = notnull
		if pk > 0 {
			currentPK[name] = pk
		}
	}

	// 3. Determine if migration is needed (missing or extra columns, or NOT NULL change)
	needsMigration := false

	// Self-healing: Check if the table definition accidentally refers to temporary migration tables.
	// This can happen if a previous migration was interrupted or performed with foreign keys enabled
	// while target tables were temporarily renamed.
	var currentSQL string
	if err := tx.QueryRow(fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name='%s'", def.Name)).Scan(&currentSQL); err == nil {
		if strings.Contains(currentSQL, "_old") || strings.Contains(currentSQL, "_new") {
			needsMigration = true
		}
	}

	if !needsMigration && len(currentCols) != len(def.Columns) {
		needsMigration = true
	} else if !needsMigration {
		for name, colDef := range def.Columns {
			notnull, exists := currentCols[name]
			if !exists {
				needsMigration = true
				break
			}
			// Check for NOT NULL change. PRIMARY KEY also implies NOT NULL in many SQLite versions.
			expectNotNull := strings.Contains(strings.ToUpper(colDef), "NOT NULL") || strings.Contains(strings.ToUpper(colDef), "PRIMARY KEY")
			if (notnull == 1) != expectNotNull {
				needsMigration = true
				break
			}
		}
	}

	// Check Primary Key change
	if !needsMigration && len(def.PrimaryKey) > 0 {
		if len(def.PrimaryKey) != len(currentPK) {
			needsMigration = true
		} else {
			for i, pkCol := range def.PrimaryKey {
				if currentPK[pkCol] != i+1 {
					needsMigration = true
					break
				}
			}
		}
	}

	if !needsMigration {
		return tx.Commit()
	}

	// 4. Perform robust migration using a temporary table
	// This handles non-constant defaults and dropped columns safely.
	// We create a new table with a temporary name, copy data, drop the old one, and rename.
	// This prevents foreign keys in other tables from being "redirected" to the old table name,
	// which happens in SQLite if we rename the existing table first.
	tempTableName := fmt.Sprintf("_%s_new", def.Name)

	// Drop temp table if it somehow exists
	_, _ = tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tempTableName))

	// Create new table with desired schema under temporary name
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", tempTableName, newColsSQL)
	if _, err := tx.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create new table schema: %w", err)
	}

	// Identify columns that exist in both old and new schemas to copy data
	commonCols := []string{}
	for _, name := range colNames {
		if _, exists := currentCols[name]; exists {
			commonCols = append(commonCols, name)
		}
	}

	if len(commonCols) > 0 {
		colsCSV := strings.Join(commonCols, ", ")
		copySQL := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", tempTableName, colsCSV, colsCSV, def.Name)
		if _, err := tx.Exec(copySQL); err != nil {
			return fmt.Errorf("failed to copy data to new table: %w", err)
		}
	}

	// Drop the old table
	if _, err := tx.Exec(fmt.Sprintf("DROP TABLE %s", def.Name)); err != nil {
		return fmt.Errorf("failed to drop old table: %w", err)
	}

	// Rename the temporary table to the final name
	if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", tempTableName, def.Name)); err != nil {
		return fmt.Errorf("failed to rename temporary table to final name: %w", err)
	}

	return tx.Commit()
}
