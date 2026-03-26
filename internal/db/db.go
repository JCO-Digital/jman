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
	`
	_, err := dbInstance.Exec(query)
	return err
}
