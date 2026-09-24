package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	_ "modernc.org/sqlite"
)

func setupTestDBDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	config.RunData = config.Runtime{
		DataDir:   dir,
		ConfigDir: dir,
		CacheDir:  dir,
		BackupDir: filepath.Join(dir, "backups"),
	}
	t.Cleanup(func() {
		_ = Close()
	})
	return dir
}

func TestHasLegacyDB(t *testing.T) {
	dir := setupTestDBDir(t)
	legacyPath := filepath.Join(dir, "jman.db")

	if HasLegacyDB() {
		t.Fatal("expected HasLegacyDB to be false when jman.db does not exist")
	}

	if err := os.WriteFile(legacyPath, []byte("fake sqlite"), 0644); err != nil {
		t.Fatalf("failed to create fake jman.db: %v", err)
	}

	if !HasLegacyDB() {
		t.Fatal("expected HasLegacyDB to be true when jman.db exists")
	}
}

func TestMigrateSplitDB_FullMigration(t *testing.T) {
	dir := setupTestDBDir(t)
	legacyPath := filepath.Join(dir, "jman.db")

	// Create legacy database with sample inventory and api tables/rows
	legacyConn, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatalf("failed to open legacy db: %v", err)
	}

	schema := `
	CREATE TABLE plugin_info (
		slug TEXT PRIMARY KEY,
		name TEXT,
		version TEXT
	);
	INSERT INTO plugin_info (slug, name, version) VALUES ('akismet', 'Akismet', '5.0.0'), ('yoast-seo', 'Yoast SEO', '20.0');

	CREATE TABLE site_plugins (
		site_id INTEGER NOT NULL,
		slug TEXT NOT NULL,
		status TEXT,
		PRIMARY KEY (site_id, slug)
	);
	INSERT INTO site_plugins (site_id, slug, status) VALUES (1, 'akismet', 'active');

	CREATE TABLE slack_messages (
		hash TEXT PRIMARY KEY,
		channel TEXT
	);
	INSERT INTO slack_messages (hash, channel) VALUES ('hash1', '#general');

	CREATE TABLE organizations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	);
	INSERT INTO organizations (id, name) VALUES (10, 'Acme Corp');
	`
	if _, err := legacyConn.Exec(schema); err != nil {
		legacyConn.Close()
		t.Fatalf("failed to populate legacy db: %v", err)
	}
	legacyConn.Close()

	// Execute migration
	if err := MigrateSplitDB(); err != nil {
		t.Fatalf("MigrateSplitDB failed: %v", err)
	}

	// Verify legacy db was renamed to .pre-split-backup
	if fileExists(legacyPath) {
		t.Errorf("legacyPath %s still exists", legacyPath)
	}
	backupPath := legacyPath + ".pre-split-backup"
	if !fileExists(backupPath) {
		t.Errorf("backupPath %s does not exist", backupPath)
	}

	// Verify inventory.db data
	inventoryPath := filepath.Join(dir, "inventory.db")
	if !fileExists(inventoryPath) {
		t.Fatalf("inventory.db was not created")
	}
	invConn, err := sql.Open("sqlite", inventoryPath)
	if err != nil {
		t.Fatalf("failed to open inventory.db: %v", err)
	}
	defer invConn.Close()

	var pluginCount int
	if err := invConn.QueryRow("SELECT COUNT(*) FROM plugin_info").Scan(&pluginCount); err != nil {
		t.Fatalf("failed to query plugin_info: %v", err)
	}
	if pluginCount != 2 {
		t.Errorf("expected 2 plugins, got %d", pluginCount)
	}

	var sitePluginCount int
	if err := invConn.QueryRow("SELECT COUNT(*) FROM site_plugins").Scan(&sitePluginCount); err != nil {
		t.Fatalf("failed to query site_plugins: %v", err)
	}
	if sitePluginCount != 1 {
		t.Errorf("expected 1 site plugin, got %d", sitePluginCount)
	}

	// Verify api.db data
	apiPath := filepath.Join(dir, "api.db")
	if !fileExists(apiPath) {
		t.Fatalf("api.db was not created")
	}
	apiConn, err := sql.Open("sqlite", apiPath)
	if err != nil {
		t.Fatalf("failed to open api.db: %v", err)
	}
	defer apiConn.Close()

	var msgCount int
	if err := apiConn.QueryRow("SELECT COUNT(*) FROM slack_messages").Scan(&msgCount); err != nil {
		t.Fatalf("failed to query slack_messages: %v", err)
	}
	if msgCount != 1 {
		t.Errorf("expected 1 slack message, got %d", msgCount)
	}

	var orgName string
	if err := apiConn.QueryRow("SELECT name FROM organizations WHERE id = 10").Scan(&orgName); err != nil {
		t.Fatalf("failed to query organizations: %v", err)
	}
	if orgName != "Acme Corp" {
		t.Errorf("expected 'Acme Corp', got %q", orgName)
	}

	// Running MigrateSplitDB again should report already migrated without error
	if err := MigrateSplitDB(); err != nil {
		t.Errorf("subsequent MigrateSplitDB call failed: %v", err)
	}
}

func TestMigrateSplitDB_PartialResume(t *testing.T) {
	dir := setupTestDBDir(t)
	legacyPath := filepath.Join(dir, "jman.db")
	inventoryPath := filepath.Join(dir, "inventory.db")

	// Pre-create inventory.db to simulate interrupted run
	if err := os.WriteFile(inventoryPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create dummy inventory.db: %v", err)
	}

	// Create legacy db with API data
	legacyConn, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatalf("failed to open legacy db: %v", err)
	}
	if _, err := legacyConn.Exec(`
		CREATE TABLE settings (user_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY(user_id, key));
		INSERT INTO settings (user_id, key, value) VALUES ('u1', 'theme', 'dark');
	`); err != nil {
		legacyConn.Close()
		t.Fatalf("failed to create table in legacy db: %v", err)
	}
	legacyConn.Close()

	if err := MigrateSplitDB(); err != nil {
		t.Fatalf("MigrateSplitDB failed on resume: %v", err)
	}

	apiPath := filepath.Join(dir, "api.db")
	apiConn, err := sql.Open("sqlite", apiPath)
	if err != nil {
		t.Fatalf("failed to open api.db: %v", err)
	}
	defer apiConn.Close()

	var val string
	if err := apiConn.QueryRow("SELECT value FROM settings WHERE user_id = 'u1' AND key = 'theme'").Scan(&val); err != nil {
		t.Fatalf("failed to query settings: %v", err)
	}
	if val != "dark" {
		t.Errorf("expected 'dark', got %q", val)
	}
}

func TestMigrateSplitDB_MissingLegacyAndSplit(t *testing.T) {
	setupTestDBDir(t)
	err := MigrateSplitDB()
	if err == nil {
		t.Fatal("expected error when no databases exist")
	}
}
