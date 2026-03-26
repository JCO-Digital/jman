package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/JCO-Digital/jman/internal/config"
	_ "modernc.org/sqlite"
)

var (
	dbInstance *sql.DB
	once       sync.Once
	dbMutex    sync.Mutex
)

// Init initializes the SQLite database.
// It creates the database file in the data directory if it doesn't exist.
func Init() error {
	var err error
	once.Do(func() {
		dbPath := filepath.Join(config.RunData.DataDir, "jman.db")

		// Open the database connection.
		// Use _pragma=foreign_keys=ON if we need foreign key support later.
		db, openErr := sql.Open("sqlite", dbPath)
		if openErr != nil {
			err = fmt.Errorf("failed to open database: %w", openErr)
			return
		}

		// Check connection
		if pingErr := db.Ping(); pingErr != nil {
			err = fmt.Errorf("failed to ping database: %w", pingErr)
			return
		}

		dbInstance = db

		// Initialize schemas
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

// initSchema creates the necessary tables if they don't exist.
func initSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS plugin_info (
		slug TEXT PRIMARY KEY,
		name TEXT,
		version TEXT,
		author TEXT,
		author_profile TEXT,
		requires TEXT,
		tested TEXT,
		last_updated TEXT,
		homepage TEXT,
		fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS slack_messages (
		hash TEXT PRIMARY KEY,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		channel TEXT
	);

	CREATE TABLE IF NOT EXISTS monitor_status (
		domain TEXT PRIMARY KEY,
		is_down BOOLEAN DEFAULT 0,
		failure_count INTEGER DEFAULT 0,
		last_alert_time DATETIME,
		last_checked DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS monitor_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT,
		status TEXT,
		error_code INTEGER,
		first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
		count INTEGER DEFAULT 1
	);

	CREATE INDEX IF NOT EXISTS idx_monitor_history_domain ON monitor_history(domain);
	`
	if _, err := dbInstance.Exec(query); err != nil {
		return err
	}

	// Check if channel column exists in slack_messages (for migration from older versions)
	var hasChannel bool
	err := dbInstance.QueryRow("SELECT count(*) FROM pragma_table_info('slack_messages') WHERE name='channel'").Scan(&hasChannel)
	if err == nil && !hasChannel {
		_, _ = dbInstance.Exec("ALTER TABLE slack_messages ADD COLUMN channel TEXT")
	}

	return nil
}
