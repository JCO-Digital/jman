package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/verb"
	_ "modernc.org/sqlite"
)

// jman's data is split across two SQLite databases:
//
//   - inventory.db holds refreshable inventory data (plugin/site/core
//     versions, ignore rules) that both the `jman` CLI and `jman-api` read
//     and write.
//   - api.db holds jman-api's own business data (organizations, billing,
//     tasks, monitor state, agent tokens, traffic, ...) that only jman-api
//     ever touches.
//
// GetInventoryDB/GetAPIDB are deliberately not aliased to each other, so
// every call site must be explicit about which database it means.
var (
	inventoryDB *sql.DB
	apiDB       *APIDB
	dbMutex     sync.Mutex
)

// TableDefinition represents the desired state of a database table.
type TableDefinition struct {
	Name       string
	Columns    map[string]string // Column name -> SQL type and constraints
	PrimaryKey []string          // Optional composite primary key
}

// CheckSplitState returns an error if the legacy pre-split jman.db file
// still exists. Binaries that do not auto-migrate (such as jman-api) call
// this before InitInventory/InitAPI, so:
//
//   - An un-migrated install (only jman.db exists) is never allowed to
//     silently proceed — without this check, InitInventory/InitAPI would
//     just create brand-new *empty* inventory.db/api.db files next to the
//     untouched jman.db, and the process would run against empty data
//     with no error at all.
//   - A partially-migrated install (jman.db plus one or both split files,
//     from an interrupted `migrate-db` run) is also caught, rather than
//     starting up against incomplete data.
//
// In both cases the fix is the same: run `jman` or `jman migrate-db` (or
// `jman-api migrate-db`).
func CheckSplitState() error {
	legacyPath := filepath.Join(config.RunData.DataDir, "jman.db")

	if fileExists(legacyPath) {
		return fmt.Errorf(
			"legacy database %s still exists — run `jman migrate-db` or `jman-api migrate-db` to split it "+
				"into inventory.db and api.db before starting any jman binary",
			legacyPath,
		)
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// maxOpenConns caps each database's connection pool.
const maxOpenConns = 8

// openDB opens a SQLite database file with the pragma set jman relies on
// for concurrency and reliability (shared by both inventory.db and api.db).
func openDB(path string) (*sql.DB, error) {
	return openDBWithParams(path, nil)
}

// openAPIDB opens api.db. On top of openDB's settings it makes the driver
// write time.Time values in SQLite's sortable text format instead of Go's
// time.String() (see APIDB for the UTC half of that). inventory.db keeps
// the driver default, as it is also written by laptop CLIs running older
// jman versions.
func openAPIDB(path string) (*APIDB, error) {
	conn, err := openDBWithParams(path, url.Values{"_time_format": {"sqlite"}})
	if err != nil {
		return nil, err
	}
	return &APIDB{DB: conn}, nil
}

func openDBWithParams(path string, extra url.Values) (*sql.DB, error) {
	// Pragmas are connection-scoped, so they go in the DSN: the driver
	// applies them to every connection the pool opens, not just the first.
	//   - WAL lets readers run concurrently with the single writer.
	//   - busy_timeout makes a writer wait for the write lock rather than
	//     failing immediately with SQLITE_BUSY.
	//   - _txlock=immediate makes explicit transactions take the write lock
	//     at BEGIN, so a transaction never has to upgrade a read lock to a
	//     write lock mid-way (an SQLITE_BUSY that busy_timeout can't resolve).
	// The path is deliberately not given a "file:" prefix: the driver then
	// strips the query itself and opens the path verbatim, with no URI
	// escaping concerns.
	params := url.Values{
		"_pragma": {
			"journal_mode(WAL)",
			"synchronous(NORMAL)",
			"busy_timeout(10000)",
			"foreign_keys(1)",
		},
		"_txlock": {"immediate"},
	}
	for k, v := range extra {
		params[k] = v
	}
	dsn := path + "?" + params.Encode()
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Allow several connections so a slow query (or the hourly VACUUM INTO
	// backup) doesn't stall every other request. SQLite still serializes
	// writers on its own lock; reads proceed in parallel under WAL.
	conn.SetMaxOpenConns(maxOpenConns)
	conn.SetMaxIdleConns(maxOpenConns)

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
// creating it if it doesn't exist. Only jman-api needs to call this.
func InitAPI() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if apiDB != nil {
		return nil
	}

	dbPath := filepath.Join(config.RunData.DataDir, "api.db")
	conn, err := openAPIDB(dbPath)
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

// GetAPIDB returns jman-api's own database instance, or nil if InitAPI
// hasn't been called.
func GetAPIDB() *APIDB {
	return apiDB
}

// BackupInventory creates a snapshot of the inventory database using VACUUM INTO.
func BackupInventory(destPath string) error {
	return backupDB(inventoryDB, destPath)
}

// BackupAPI creates a snapshot of the api database using VACUUM INTO.
func BackupAPI(destPath string) error {
	return backupDB(apiDB.sqlDB(), destPath)
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
				"site_id":          "TEXT NOT NULL",
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
				"site_id":      "TEXT PRIMARY KEY",
				"version":      "TEXT NOT NULL",
				"minor_update": "TEXT",
				"major_update": "TEXT",
				"updated_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "site_users",
			Columns: map[string]string{
				"site_id":      "TEXT NOT NULL",
				"user_id":      "INTEGER NOT NULL",
				"login":        "TEXT NOT NULL DEFAULT ''",
				"email":        "TEXT NOT NULL DEFAULT ''",
				"display_name": "TEXT NOT NULL DEFAULT ''",
				"roles":        "TEXT NOT NULL DEFAULT ''",
				"registered":   "TEXT NOT NULL DEFAULT ''",
				// NULL when the posts weren't counted.
				"post_count": "INTEGER",
				// The user's application passwords as JSON, without hashes
				// ('' when none or not read; see app_passwords_collected).
				"app_passwords": "TEXT NOT NULL DEFAULT ''",
			},
			PrimaryKey: []string{"site_id", "user_id"},
		},
		{
			// One row per site whose users were read, also when it has
			// none to list: the excluded roles' counts and when it was read.
			Name: "site_user_summary",
			Columns: map[string]string{
				"site_id":         "TEXT PRIMARY KEY",
				"excluded_counts": "TEXT NOT NULL DEFAULT '{}'",
				"truncated":       "BOOLEAN NOT NULL DEFAULT 0",
				// Whether the users' application passwords were read.
				"app_passwords_collected": "BOOLEAN NOT NULL DEFAULT 0",
				"updated_at":              "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "site_admin_user",
			Columns: map[string]string{
				"site_id":    "TEXT PRIMARY KEY",
				"user_id":    "INTEGER NOT NULL",
				"updated_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "site_environment",
			Columns: map[string]string{
				"site_id":     "TEXT PRIMARY KEY",
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
				// Legacy: monitor ignores became paused monitor checks. Kept
				// only so the monitor_checks migration can read it.
				"use_for_monitor": "BOOLEAN DEFAULT 0",
				"use_for_vuln":    "BOOLEAN DEFAULT 0",
				"created_at":      "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":      "TEXT",
				"updated_at":      "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":      "TEXT",
			},
		},
		{
			// vuln_backports records vulnerabilities fixed by backport
			// releases the vulnerability database doesn't know about.
			Name: "vuln_backports",
			Columns: map[string]string{
				"id":          "INTEGER PRIMARY KEY AUTOINCREMENT",
				"type":        "TEXT NOT NULL",
				"slug":        "TEXT NOT NULL DEFAULT ''",
				"vuln_uuid":   "TEXT NOT NULL",
				"branch":      "TEXT NOT NULL",
				"min_version": "TEXT NOT NULL",
				"reason":      "TEXT NOT NULL DEFAULT ''",
				"created_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":  "TEXT",
				"updated_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":  "TEXT",
			},
		},
		{
			Name: "servers",
			Columns: map[string]string{
				"id":                 "TEXT PRIMARY KEY",
				"provider":           "TEXT NOT NULL DEFAULT 'manual'",
				"provider_server_id": "TEXT DEFAULT ''",
				"name":               "TEXT NOT NULL",
				"is_logical":         "BOOLEAN DEFAULT 0",
				"ip_address":         "TEXT DEFAULT ''",
				"ssh_port":           "INTEGER DEFAULT 22",
				"created_at":         "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_at":         "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "sites",
			Columns: map[string]string{
				"id":               "TEXT PRIMARY KEY",
				"server_id":        "TEXT",
				"provider":         "TEXT NOT NULL DEFAULT 'manual'",
				"provider_site_id": "TEXT DEFAULT ''",
				"domain":           "TEXT NOT NULL",
				"environment":      "TEXT NOT NULL DEFAULT 'production'",
				"is_wordpress":     "BOOLEAN DEFAULT 1",
				"php_version":      "TEXT DEFAULT ''",
				"connection_type":  "TEXT DEFAULT 'ssh'",
				"ssh_host":         "TEXT NOT NULL DEFAULT ''",
				"ssh_port":         "INTEGER DEFAULT 22",
				"ssh_user":         "TEXT NOT NULL DEFAULT ''",
				"site_path":        "TEXT DEFAULT 'files'",
				"can_wp_cli":       "BOOLEAN DEFAULT 1",
				"has_agent":        "BOOLEAN DEFAULT 0",
				// Legacy: replaced by monitor_checks.enabled. Kept only so the
				// monitor_checks migration can read it; no code writes it.
				"has_monitoring": "BOOLEAN DEFAULT 1",
				"status":         "TEXT DEFAULT 'active'",
				"created_at":     "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_at":     "DATETIME DEFAULT CURRENT_TIMESTAMP",
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

	_, _ = inventoryDB.Exec("CREATE INDEX IF NOT EXISTS idx_sites_domain ON sites(domain);")
	_, _ = inventoryDB.Exec("CREATE INDEX IF NOT EXISTS idx_sites_provider_site_id ON sites(provider, provider_site_id);")
	_, _ = inventoryDB.Exec("CREATE INDEX IF NOT EXISTS idx_servers_provider_server_id ON servers(provider, provider_server_id);")
	// Enforces one rule per vulnerability and branch; the CLI and jman-api
	// both write here, so this must not rely on a check-then-insert.
	if _, err := inventoryDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vuln_backports_uuid_branch ON vuln_backports(vuln_uuid, branch);"); err != nil {
		return fmt.Errorf("failed to create vuln_backports unique index: %w", err)
	}

	if err := MigrateLegacyInventoryIDs(inventoryDB); err != nil {
		return fmt.Errorf("failed to migrate legacy inventory IDs to UUIDs: %w", err)
	}

	return nil
}

// initAPISchema creates/migrates the tables that live in api.db: jman-api's
// own business data, exclusively owned by jman-api.
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
			// vuln_alerts records which vulnerability UUIDs have already had
			// their one-off "new vulnerability" Slack alert sent (or were
			// seeded as pre-existing when alerting was first enabled).
			Name: "vuln_alerts",
			Columns: map[string]string{
				"uuid":        "TEXT PRIMARY KEY",
				"plugin_slug": "TEXT",
				"cvss":        "REAL",
				"sent_at":     "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			// vuln_exposures has one row per site running a vulnerable
			// version of a plugin (or of core), open until the site no
			// longer runs an affected version. site_id is a soft reference
			// into inventory.db. See docs/vuln-workflow.md.
			Name: "vuln_exposures",
			Columns: map[string]string{
				"id":                   "INTEGER PRIMARY KEY AUTOINCREMENT",
				"kind":                 "TEXT NOT NULL",
				"slug":                 "TEXT NOT NULL",
				"site_id":              "TEXT NOT NULL",
				"version":              "TEXT NOT NULL DEFAULT ''",
				"vuln_uuids":           "TEXT NOT NULL DEFAULT '[]'",
				"max_cvss":             "REAL",
				"fix_state":            "TEXT NOT NULL",
				"fixed_in":             "TEXT NOT NULL DEFAULT ''",
				"possible_fix_version": "TEXT NOT NULL DEFAULT ''",
				"fix_available_since":  "DATETIME",
				"first_seen_at":        "DATETIME NOT NULL",
				"last_seen_at":         "DATETIME NOT NULL",
				"resolved_at":          "DATETIME",
			},
		},
		{
			// monitor_checks holds everything the uptime monitor checks. A
			// default check belongs to a site (in inventory.db, so site_id
			// is a soft reference) and takes its host from the site.
			Name: "monitor_checks",
			Columns: map[string]string{
				"id":         "TEXT PRIMARY KEY",
				"site_id":    "TEXT",
				"is_default": "BOOLEAN NOT NULL DEFAULT 0",
				"type":       "TEXT NOT NULL DEFAULT 'http'",
				"name":       "TEXT NOT NULL DEFAULT ''",
				"host":       "TEXT NOT NULL DEFAULT ''",
				"path":       "TEXT NOT NULL DEFAULT '/'",
				"expect":     "TEXT NOT NULL DEFAULT 'html'",
				// One optional assertion on a JSON body: a dotted path, an
				// operator ('equals' or 'exists') and, for equals, the
				// JSON-encoded expected value.
				"assert_path":  "TEXT NOT NULL DEFAULT ''",
				"assert_op":    "TEXT NOT NULL DEFAULT ''",
				"assert_value": "TEXT NOT NULL DEFAULT ''",
				// NULL follows the global monitorCacheBypass setting.
				"cache_bypass": "BOOLEAN",
				// Heartbeat checks: expected check-in interval, grace on top
				// of it, and failed check-ins in a row before alerting.
				"interval_seconds": "INTEGER NOT NULL DEFAULT 0",
				"grace_seconds":    "INTEGER NOT NULL DEFAULT 0",
				"fail_threshold":   "INTEGER NOT NULL DEFAULT 1",
				"enabled":          "BOOLEAN NOT NULL DEFAULT 1",
				// The monitor_check_rules row that created the check paused.
				"paused_by_rule_id": "TEXT",
				"orphaned_at":       "DATETIME",
				"created_at":        "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":        "TEXT",
				"updated_at":        "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":        "TEXT",
			},
		},
		{
			// monitor_check_rules decide how new default checks are
			// created, e.g. paused for every site on a staging server.
			Name: "monitor_check_rules",
			Columns: map[string]string{
				"id":         "TEXT PRIMARY KEY",
				"match_type": "TEXT NOT NULL",
				"value":      "TEXT NOT NULL",
				"action":     "TEXT NOT NULL DEFAULT 'start_paused'",
				"note":       "TEXT NOT NULL DEFAULT ''",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by": "TEXT",
			},
		},
		{
			// monitor_check_secrets holds header values sent with a check's
			// requests. The API only ever writes them, never returns them.
			Name: "monitor_check_secrets",
			Columns: map[string]string{
				"check_id":    "TEXT NOT NULL",
				"header_name": "TEXT NOT NULL",
				"value":       "TEXT NOT NULL",
				"updated_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":  "TEXT",
			},
			PrimaryKey: []string{"check_id", "header_name"},
		},
		{
			// monitor_check_tokens holds the hash of each heartbeat check's
			// check-in token. The token itself is shown once, when issued.
			Name: "monitor_check_tokens",
			Columns: map[string]string{
				"check_id":     "TEXT PRIMARY KEY",
				"token_hash":   "TEXT NOT NULL",
				"token_prefix": "TEXT NOT NULL",
				"created_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":   "TEXT",
			},
		},
		{
			// monitor_check_status is the monitor's persisted per-check
			// state; domain is the checked host, kept for display.
			Name: "monitor_check_status",
			Columns: map[string]string{
				"check_id":              "TEXT PRIMARY KEY",
				"domain":                "TEXT NOT NULL DEFAULT '' COLLATE NOCASE",
				"is_down":               "BOOLEAN DEFAULT 0",
				"failure_count":         "INTEGER DEFAULT 0",
				"consecutive_successes": "INTEGER DEFAULT 0",
				"current_mode":          "TEXT DEFAULT 'normal'",
				"last_alert_time":       "DATETIME",
				"last_checked":          "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"next_check_at":         "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"down_since":            "DATETIME",
				"pd_triggered":          "BOOLEAN DEFAULT 0",
				"pd_escalated":          "BOOLEAN DEFAULT 0",
				"last_started_at":       "DATETIME",
			},
		},
		{
			Name: "monitor_history",
			Columns: map[string]string{
				"id":         "INTEGER PRIMARY KEY AUTOINCREMENT",
				"check_id":   "TEXT",
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
				"site_id":         "TEXT NOT NULL",
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
				"site_id":           "TEXT",
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
				"server_id":     "TEXT NOT NULL",
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
			// refresh_tokens holds web/CLI login sessions. Only a SHA-256
			// hash of each token is stored. A login starts a family; every
			// refresh rotates the token within it (see refresh_token_repo.go).
			Name: "refresh_tokens",
			Columns: map[string]string{
				"id":            "INTEGER PRIMARY KEY AUTOINCREMENT",
				"family_id":     "TEXT NOT NULL",
				"username":      "TEXT NOT NULL",
				"token_hash":    "TEXT NOT NULL",
				"token_version": "INTEGER NOT NULL",
				"expires_at":    "DATETIME NOT NULL",
				"rotated_at":    "DATETIME",
				"created_at":    "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			// cli_tokens are the jman CLI's per-device API tokens, issued
			// through the browser login. Only a SHA-256 hash of each token
			// is stored. A token expires after a period without use (see
			// cli_token_repo.go).
			Name: "cli_tokens",
			Columns: map[string]string{
				"id":            "INTEGER PRIMARY KEY AUTOINCREMENT",
				"username":      "TEXT NOT NULL",
				"token_hash":    "TEXT NOT NULL",
				"token_prefix":  "TEXT NOT NULL",
				"device_name":   "TEXT NOT NULL",
				"token_version": "INTEGER NOT NULL",
				"created_at":    "DATETIME NOT NULL",
				"last_used_at":  "DATETIME NOT NULL",
			},
		},
		{
			Name: "site_disk_usage",
			Columns: map[string]string{
				"site_id":     "TEXT NOT NULL",
				"bytes_used":  "INTEGER NOT NULL",
				"measured_at": "DATETIME NOT NULL",
				"created_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
			PrimaryKey: []string{"site_id", "measured_at"},
		},
		{
			Name: "site_wp_flags",
			Columns: map[string]string{
				"site_id":            "TEXT PRIMARY KEY",
				"is_multisite":       "BOOLEAN DEFAULT 0",
				"disallow_file_mods": "BOOLEAN DEFAULT 0",
				// The WP_AUTO_UPDATE_CORE setting the agent found ("true",
				// "false", "minor", "disabled" for AUTOMATIC_UPDATER_DISABLED,
				// or "default" when neither is set). NULL means unknown: the
				// agent is too old to report it.
				"auto_update_core": "TEXT",
				"updated_at":       "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			// site_scans holds the latest file scan jman-agent reported for
			// each site. fingerprint is the agent's baseline after that
			// scan, which the next scan's prev_fingerprint must match.
			Name: "site_scans",
			Columns: map[string]string{
				"site_id":          "TEXT PRIMARY KEY",
				"scanned_at":       "DATETIME NOT NULL",
				"rule_set_version": "TEXT NOT NULL DEFAULT ''",
				"fingerprint":      "TEXT NOT NULL DEFAULT ''",
				"entries":          "INTEGER NOT NULL DEFAULT 0",
				"omitted":          "INTEGER NOT NULL DEFAULT 0",
				"duration_ms":      "INTEGER NOT NULL DEFAULT 0",
				"error":            "TEXT NOT NULL DEFAULT ''",
				"rebaselined_at":   "DATETIME",
				"updated_at":       "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			// scan_findings holds what file scans flagged. The same site,
			// rule, path and hash is one finding (see
			// idx_scan_findings_identity), however often it's reported.
			Name: "scan_findings",
			Columns: map[string]string{
				"id":               "INTEGER PRIMARY KEY AUTOINCREMENT",
				"site_id":          "TEXT NOT NULL",
				"engine":           "TEXT NOT NULL",
				"rule":             "TEXT NOT NULL",
				"severity":         "TEXT NOT NULL",
				"path":             "TEXT NOT NULL DEFAULT ''",
				"sha256":           "TEXT NOT NULL DEFAULT ''",
				"snippet":          "TEXT NOT NULL DEFAULT ''",
				"detail":           "TEXT NOT NULL DEFAULT ''",
				"rule_set_version": "TEXT NOT NULL DEFAULT ''",
				"first_seen_at":    "DATETIME NOT NULL",
				"last_seen_at":     "DATETIME NOT NULL",
				// The security incident the finding belongs to. Low findings
				// and allowlisted or acknowledged ones don't get one.
				"incident_id": "INTEGER",
				// open, closed (dismissed, or its incident closed),
				// allowlisted (hash) or acknowledged (path); status_by and
				// status_at say who changed it last and when.
				"status":    "TEXT NOT NULL DEFAULT 'open'",
				"status_by": "TEXT NOT NULL DEFAULT ''",
				"status_at": "DATETIME",
				// When the agent found the finding no longer holds (the file
				// was removed, replaced, chowned, …), and what it found.
				// Reporting the finding again clears these.
				"cleared_at":     "DATETIME",
				"cleared_detail": "TEXT NOT NULL DEFAULT ''",
				// When the snippet and detail were removed (they quote the
				// file, which can hold personal data), and by whom
				// ("jman-api" when they expired).
				"redacted_at": "DATETIME",
				"redacted_by": "TEXT NOT NULL DEFAULT ''",
			},
		},
		{
			// release_checksums caches wordpress.org's checksums of WordPress
			// (kind "core", name the locale; outside wp-content) and plugin
			// (kind "plugin", name the slug) releases for jman-agent's scans,
			// as JSON. Empty data means wordpress.org had none at fetched_at.
			Name: "release_checksums",
			Columns: map[string]string{
				"kind":       "TEXT NOT NULL",
				"name":       "TEXT NOT NULL",
				"version":    "TEXT NOT NULL",
				"data":       "TEXT NOT NULL DEFAULT ''",
				"fetched_at": "DATETIME NOT NULL",
			},
			PrimaryKey: []string{"kind", "name", "version"},
		},
		{
			// scan_allowlist holds file hashes trusted on every site.
			// site_id, rule, path and snippet describe the finding an entry
			// was made from. Removing an entry only sets removed, so
			// updated_by and updated_at record who removed it.
			Name: "scan_allowlist",
			Columns: map[string]string{
				"id":         "INTEGER PRIMARY KEY AUTOINCREMENT",
				"sha256":     "TEXT NOT NULL",
				"reason":     "TEXT NOT NULL",
				"site_id":    "TEXT NOT NULL DEFAULT ''",
				"rule":       "TEXT NOT NULL DEFAULT ''",
				"path":       "TEXT NOT NULL DEFAULT ''",
				"snippet":    "TEXT NOT NULL DEFAULT ''",
				"removed":    "BOOLEAN NOT NULL DEFAULT 0",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by": "TEXT NOT NULL DEFAULT ''",
				"updated_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by": "TEXT NOT NULL DEFAULT ''",
			},
		},
		{
			// scan_path_acks acknowledges a state rule's findings (a stray
			// or foreign-owned file) for one path on one site, whatever
			// the file's content. Removal works as in scan_allowlist.
			Name: "scan_path_acks",
			Columns: map[string]string{
				"id":         "INTEGER PRIMARY KEY AUTOINCREMENT",
				"site_id":    "TEXT NOT NULL",
				"rule":       "TEXT NOT NULL",
				"path":       "TEXT NOT NULL",
				"reason":     "TEXT NOT NULL",
				"removed":    "BOOLEAN NOT NULL DEFAULT 0",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by": "TEXT NOT NULL DEFAULT ''",
				"updated_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by": "TEXT NOT NULL DEFAULT ''",
			},
		},
		{
			Name: "site_traffic_hourly",
			Columns: map[string]string{
				"site_id":         "TEXT NOT NULL",
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
				"site_id":         "TEXT NOT NULL",
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
				"site_id":          "TEXT",
				"server_id":        "TEXT",
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
				"site_id":     "TEXT NOT NULL",
				"update_type": "TEXT NOT NULL",
				"status":      "TEXT NOT NULL",
				"data_json":   "TEXT",
				"updated_by":  "TEXT",
				"updated_at":  "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			// Background jobs on one site: plugin updates (kind "plugins"),
			// WordPress core updates (kind "core"), plugin activate,
			// deactivate, delete and uninstall, plugin installs, and
			// WordPress user create and delete. payload holds the kind's
			// JSON payload (models.*Payload), results a JSON array, core the
			// refreshed SiteCore after a core job. upload_path is an
			// uploaded plugin ZIP, kept out of the payload as it's local to
			// jman-api.
			Name: "plugin_update_jobs",
			Columns: map[string]string{
				"id":          "INTEGER PRIMARY KEY AUTOINCREMENT",
				"kind":        "TEXT NOT NULL DEFAULT 'plugins'",
				"site_id":     "TEXT NOT NULL",
				"payload":     "TEXT NOT NULL DEFAULT '{}'",
				"core":        "TEXT",
				"upload_path": "TEXT",
				"status":      "TEXT NOT NULL",
				"results":     "TEXT",
				"error":       "TEXT",
				"created_by":  "TEXT",
				"created_at":  "DATETIME NOT NULL",
				"started_at":  "DATETIME",
				"finished_at": "DATETIME",
			},
		},
		{
			// jman-agent's WordPress data collection per site: when it last
			// collected successfully (NULL until it first has; until then
			// the periodic SSH refresh keeps covering the site) and the
			// latest error, if the last attempt failed.
			Name: "site_agent_wp_status",
			Columns: map[string]string{
				"site_id":      "TEXT PRIMARY KEY",
				"collected_at": "DATETIME",
				"error":        "TEXT",
				"error_at":     "DATETIME",
				"updated_at":   "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			// Open "agent is stale" alerts for sites (kind "site"), so
			// each is sent to Slack once (repeated daily while it lasts),
			// with a recovery message when it clears. Whole agents keep
			// theirs on agent_tokens.
			Name: "agent_stale_alerts",
			Columns: map[string]string{
				"kind":       "TEXT NOT NULL",
				"target_id":  "TEXT NOT NULL",
				"alerted_at": "DATETIME NOT NULL",
			},
			PrimaryKey: []string{"kind", "target_id"},
		},
		{
			// Update locks restrict a site (plugin = '') or one plugin on
			// a site to fix-release updates. See models.UpdateLock.
			Name: "update_locks",
			Columns: map[string]string{
				"id":         "INTEGER PRIMARY KEY AUTOINCREMENT",
				"site_id":    "TEXT NOT NULL",
				"plugin":     "TEXT NOT NULL DEFAULT ''",
				"comment":    "TEXT",
				"created_by": "TEXT",
				"created_at": "DATETIME NOT NULL",
			},
		},
		{
			Name: "incidents",
			Columns: map[string]string{
				"id":              "INTEGER PRIMARY KEY AUTOINCREMENT",
				"check_id":        "TEXT",
				"domain":          "TEXT NOT NULL COLLATE NOCASE",
				"status":          "TEXT NOT NULL DEFAULT 'open'",
				"error_message":   "TEXT",
				"error_code":      "INTEGER DEFAULT 0",
				"down_since":      "DATETIME NOT NULL",
				"acknowledged_by": "TEXT",
				"acknowledged_at": "DATETIME",
				"resolved_by":     "TEXT",
				"resolved_at":     "DATETIME",
				"pd_triggered":    "BOOLEAN DEFAULT 0",
				// uptime or security. A security incident groups a site's
				// scan findings (scan_findings.incident_id); site_id and
				// severity (its worst finding's) are only set for those.
				"type":     "TEXT NOT NULL DEFAULT 'uptime'",
				"site_id":  "TEXT",
				"severity": "TEXT NOT NULL DEFAULT ''",
				// Why a security incident is critical though no single
				// finding is: its findings together (see
				// models.CorrelateScanFindings).
				"escalation": "TEXT NOT NULL DEFAULT ''",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
	}

	// Before migrateTable drops the old job columns.
	if err := MigrateUpdateJobPayloads(apiDB.DB); err != nil {
		return fmt.Errorf("failed to migrate update job payloads: %w", err)
	}

	for _, table := range tables {
		if err := migrateTable(apiDB.DB, table); err != nil {
			return fmt.Errorf("failed to migrate table %s: %w", table.Name, err)
		}
	}

	// Manual index management
	_, err := apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_monitor_history_domain ON monitor_history(domain);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_monitor_history_check_id ON monitor_history(check_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_monitor_checks_site_id ON monitor_checks(site_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_monitor_check_tokens_hash ON monitor_check_tokens(token_hash);")
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
	_, err = apiDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family_id ON refresh_tokens(family_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_cli_tokens_token_hash ON cli_tokens(token_hash);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_cli_tokens_username ON cli_tokens(username);")
	if err != nil {
		return err
	}
	// Redundant with the (site_id, measured_at) primary key's index, which
	// already serves site_id lookups; dropped to save space and write cost.
	_, err = apiDB.Exec("DROP INDEX IF EXISTS idx_site_disk_usage_site_id;")
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
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_plugin_update_jobs_status ON plugin_update_jobs(status);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_update_locks_site_plugin ON update_locks(site_id, plugin);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_scan_findings_identity ON scan_findings(site_id, rule, path, sha256);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_scan_findings_last_seen_at ON scan_findings(last_seen_at);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_scan_findings_incident_id ON scan_findings(incident_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_scan_findings_sha256 ON scan_findings(sha256);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_scan_allowlist_sha256 ON scan_allowlist(sha256);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_scan_path_acks_site_id ON scan_path_acks(site_id);")
	if err != nil {
		return err
	}
	// At most one open exposure per plugin (or core) and site.
	_, err = apiDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_vuln_exposures_open ON vuln_exposures(kind, slug, site_id) WHERE resolved_at IS NULL;")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_vuln_exposures_resolved_at ON vuln_exposures(resolved_at);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_incidents_site_id ON incidents(site_id);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_incidents_domain ON incidents(domain);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(status);")
	if err != nil {
		return err
	}
	_, err = apiDB.Exec("CREATE INDEX IF NOT EXISTS idx_incidents_check_id ON incidents(check_id);")
	if err != nil {
		return err
	}

	if err := MigrateLegacyAPIIDs(apiDB.DB); err != nil {
		return fmt.Errorf("failed to migrate legacy API IDs to UUIDs: %w", err)
	}

	if err := NormalizeAPITimestamps(apiDB.DB, tables); err != nil {
		return fmt.Errorf("failed to normalize API timestamps: %w", err)
	}

	if err := MigrateMonitorChecks(apiDB.DB, inventoryDB); err != nil {
		return fmt.Errorf("failed to migrate monitoring to checks: %w", err)
	}

	if err := SkipAutoVulnTasks(apiDB.DB); err != nil {
		return fmt.Errorf("failed to skip auto-generated vulnerability tasks: %w", err)
	}

	return nil
}

// migrateTable compares the current database table schema with the desired definition.
// SQLite has restrictions on ALTER TABLE (e.g., adding columns with non-constant defaults like CURRENT_TIMESTAMP).
// To be robust, this implementation uses the "recreate and copy" pattern if changes are detected.
func migrateTable(pool *sql.DB, def TableDefinition) (retErr error) {
	// PRAGMA foreign_keys is connection-scoped, so pin a single connection
	// from the pool for the pragma and the transaction that relies on it.
	ctx := context.Background()
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Disable foreign keys during migration to avoid broken references when renaming tables.
	// PRAGMA foreign_keys must be set outside of a transaction.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("failed to disable foreign keys: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys=ON")
	}()

	tx, err := conn.BeginTx(ctx, nil)
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

	currentCols := make(map[string]int)        // name -> notnull
	currentColTypes := make(map[string]string) // name -> type
	currentPK := make(map[string]int)          // name -> pk index
	for rows.Next() {
		var cid int
		var name, typeName string
		var notnull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &typeName, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		currentCols[name] = notnull
		currentColTypes[name] = strings.ToUpper(typeName)
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
			// Check for NOT NULL change. Only an explicit NOT NULL counts:
			// PRAGMA table_info reports notnull=0 for a column-level
			// PRIMARY KEY without one, so treating PRIMARY KEY as NOT NULL
			// here made every such table look changed and get rebuilt on
			// every start (which also reset AUTOINCREMENT sequences).
			expectNotNull := strings.Contains(strings.ToUpper(colDef), "NOT NULL")
			if (notnull == 1) != expectNotNull {
				needsMigration = true
				break
			}
			// Check for type change (e.g. INTEGER to TEXT)
			parts := strings.Fields(strings.ToUpper(colDef))
			if len(parts) > 0 {
				expectedType := parts[0]
				currType := currentColTypes[name]
				if currType != "" && expectedType != "" && currType != expectedType {
					needsMigration = true
					break
				}
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

	// Rebuilding a large table can take a while; say so, so startup doesn't
	// look hung.
	verb.LogPrintf(verb.Normal, "Upgrading schema of table %s...", def.Name)
	started := time.Now()
	defer func() {
		if retErr == nil {
			verb.LogPrintf(verb.Normal, "Upgraded table %s in %s", def.Name, time.Since(started).Round(time.Millisecond))
		}
	}()

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
