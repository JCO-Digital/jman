package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/JCO-Digital/jman/internal/config"
	_ "modernc.org/sqlite"
)

var (
	dbInstance *sql.DB
	once       sync.Once
	dbMutex    sync.Mutex
)

// TableDefinition represents the desired state of a database table.
type TableDefinition struct {
	Name       string
	Columns    map[string]string // Column name -> SQL type and constraints
	PrimaryKey []string          // Optional composite primary key
}

// Init initializes the SQLite database.
// It creates the database file in the data directory if it doesn't exist.
func Init() error {
	var err error
	once.Do(func() {
		dbPath := filepath.Join(config.RunData.DataDir, "jman.db")

		// Open the database connection.
		db, openErr := sql.Open("sqlite", dbPath)
		if openErr != nil {
			err = fmt.Errorf("failed to open database: %w", openErr)
			return
		}

		// Limit to a single connection to avoid "database is locked" errors.
		// SQLite works best with a single connection when performing concurrent writes.
		db.SetMaxOpenConns(1)

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
			if _, pragmaErr := db.Exec(p); pragmaErr != nil {
				err = fmt.Errorf("failed to set pragma %q: %w", p, pragmaErr)
				return
			}
		}

		// Check connection
		if pingErr := db.Ping(); pingErr != nil {
			err = fmt.Errorf("failed to ping database: %w", pingErr)
			return
		}

		dbInstance = db

		// Initialize schemas with migration support
		if schemaErr := initSchema(); schemaErr != nil {
			err = fmt.Errorf("failed to initialize schema: %w", schemaErr)
			return
		}
	})

	return err
}

// GetDB returns the global database instance.
func GetDB() *sql.DB {
	return dbInstance
}

// Backup creates a snapshot of the database using VACUUM INTO.
func Backup(destPath string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if dbInstance == nil {
		return fmt.Errorf("database not initialized")
	}

	// SQLite's VACUUM INTO requires the target file to NOT exist.
	// We escape single quotes in the path just in case.
	escapedPath := strings.ReplaceAll(destPath, "'", "''")
	query := fmt.Sprintf("VACUUM INTO '%s'", escapedPath)

	if _, err := dbInstance.Exec(query); err != nil {
		return fmt.Errorf("VACUUM INTO failed: %w", err)
	}

	return nil
}

// Close closes the database connection.
func Close() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if dbInstance != nil {
		err := dbInstance.Close()
		dbInstance = nil
		return err
	}
	return nil
}

// initSchema creates the necessary tables and migrates them if they've changed.
func initSchema() error {
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
			Name: "monitor_ignored_sites",
			Columns: map[string]string{
				"domain":     "TEXT PRIMARY KEY COLLATE NOCASE",
				"reason":     "TEXT",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "monitor_ignored_history",
			Columns: map[string]string{
				"id":         "INTEGER PRIMARY KEY AUTOINCREMENT",
				"domain":     "TEXT COLLATE NOCASE",
				"action":     "TEXT",
				"reason":     "TEXT",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
			},
		},
		{
			Name: "vuln_ignored",
			Columns: map[string]string{
				"uuid":       "TEXT PRIMARY KEY",
				"reason":     "TEXT",
				"created_at": "DATETIME DEFAULT CURRENT_TIMESTAMP",
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
			Name: "assets",
			Columns: map[string]string{
				"id":            "INTEGER PRIMARY KEY AUTOINCREMENT",
				"type":          "TEXT NOT NULL",
				"identifier":    "TEXT",
				"name":          "TEXT NOT NULL",
				"description":   "TEXT",
				"default_price": "INTEGER",
				"default_freq":  "TEXT",
				"active":        "BOOLEAN DEFAULT 1",
				"created_at":    "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":    "TEXT",
				"updated_at":    "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":    "TEXT",
			},
		},
		{
			Name: "organization_assets",
			Columns: map[string]string{
				"id":              "INTEGER PRIMARY KEY AUTOINCREMENT",
				"organization_id": "INTEGER REFERENCES organizations(id) ON DELETE CASCADE",
				"site_id":         "INTEGER",
				"asset_id":        "INTEGER REFERENCES assets(id) ON DELETE SET NULL",
				"identifier":      "TEXT",
				"price":           "INTEGER",
				"billing_freq":    "TEXT",
				"next_billing":    "DATETIME",
				"status":          "TEXT DEFAULT 'active'",
				"description":     "TEXT",
				"created_at":      "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"created_by":      "TEXT",
				"updated_at":      "DATETIME DEFAULT CURRENT_TIMESTAMP",
				"updated_by":      "TEXT",
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
				"parent_id":   "INTEGER",
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
	}

	for _, table := range tables {
		if err := migrateTable(table); err != nil {
			return fmt.Errorf("failed to migrate table %s: %w", table.Name, err)
		}
	}

	// Manual index management
	_, err := dbInstance.Exec("CREATE INDEX IF NOT EXISTS idx_monitor_history_domain ON monitor_history(domain);")
	if err != nil {
		return err
	}

	_, err = dbInstance.Exec("CREATE INDEX IF NOT EXISTS idx_contacts_organization_id ON contacts(organization_id);")
	if err != nil {
		return err
	}
	_, err = dbInstance.Exec("CREATE INDEX IF NOT EXISTS idx_notes_parent ON notes(parent_type, parent_id);")
	if err != nil {
		return err
	}
	_, err = dbInstance.Exec("CREATE INDEX IF NOT EXISTS idx_organization_assets_org_id ON organization_assets(organization_id);")
	if err != nil {
		return err
	}
	_, err = dbInstance.Exec("CREATE INDEX IF NOT EXISTS idx_asset_payments_asset_id ON asset_payments(org_asset_id);")
	return err
}

// migrateTable compares the current database table schema with the desired definition.
// SQLite has restrictions on ALTER TABLE (e.g., adding columns with non-constant defaults like CURRENT_TIMESTAMP).
// To be robust, this implementation uses the "recreate and copy" pattern if changes are detected.
func migrateTable(def TableDefinition) error {
	// Disable foreign keys during migration to avoid broken references when renaming tables.
	// PRAGMA foreign_keys must be set outside of a transaction.
	if _, err := dbInstance.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("failed to disable foreign keys: %w", err)
	}
	defer func() {
		_, _ = dbInstance.Exec("PRAGMA foreign_keys=ON")
	}()

	tx, err := dbInstance.Begin()
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
	if len(currentCols) != len(def.Columns) {
		needsMigration = true
	} else {
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
	tempTableName := fmt.Sprintf("_%s_old", def.Name)

	// Drop temp table if it somehow exists
	_, _ = tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tempTableName))

	// Rename current table to temp
	if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", def.Name, tempTableName)); err != nil {
		return fmt.Errorf("failed to rename table for migration: %w", err)
	}

	// Create new table with desired schema
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", def.Name, newColsSQL)
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
		copySQL := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", def.Name, colsCSV, colsCSV, tempTableName)
		if _, err := tx.Exec(copySQL); err != nil {
			return fmt.Errorf("failed to copy data to new table: %w", err)
		}
	}

	// Drop the temporary table
	if _, err := tx.Exec(fmt.Sprintf("DROP TABLE %s", tempTableName)); err != nil {
		return fmt.Errorf("failed to drop temporary table: %w", err)
	}

	return tx.Commit()
}
