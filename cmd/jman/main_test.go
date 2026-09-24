package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/JCO-Digital/jman/internal/commands"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/adrg/xdg"
	_ "modernc.org/sqlite"
)

func TestRun_AutoMigratesLegacyDB(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	dataDir := filepath.Join(dir, "data")
	cacheDir := filepath.Join(dir, "cache")

	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	t.Setenv("JMAN_TOKENSPINUP", "dummy-token")
	xdg.Reload()
	t.Cleanup(func() {
		xdg.Reload()
	})

	legacyPath := filepath.Join(dataDir, "jman", "jman.db")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}

	legacyConn, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacyConn.Exec(`
		CREATE TABLE plugin_info (slug TEXT PRIMARY KEY, name TEXT);
		INSERT INTO plugin_info (slug, name) VALUES ('test-plugin', 'Test Plugin');
	`)
	if err != nil {
		legacyConn.Close()
		t.Fatal(err)
	}
	legacyConn.Close()

	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		_ = db.Close()
		commands.MigratedJustNow = false
	}()
	os.Args = []string{"jman", "--help"}

	exitCode := run()
	if exitCode != 0 {
		t.Fatalf("expected run() to return 0, got %d", exitCode)
	}

	// Legacy db should be renamed
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy db still exists at %s", legacyPath)
	}

	backupPath := legacyPath + ".pre-split-backup"
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup db does not exist at %s: %v", backupPath, err)
	}

	// inventory.db should exist and have the migrated row
	invPath := filepath.Join(dataDir, "jman", "inventory.db")
	invConn, err := sql.Open("sqlite", invPath)
	if err != nil {
		t.Fatal(err)
	}
	defer invConn.Close()

	var count int
	if err := invConn.QueryRow("SELECT COUNT(*) FROM plugin_info WHERE slug = 'test-plugin'").Scan(&count); err != nil {
		t.Fatalf("failed to query inventory.db: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 plugin in inventory.db, got %d", count)
	}
}

func TestRun_MigrateDbCommand(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	dataDir := filepath.Join(dir, "data")
	cacheDir := filepath.Join(dir, "cache")

	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	t.Setenv("JMAN_TOKENSPINUP", "dummy-token")
	xdg.Reload()
	t.Cleanup(func() {
		xdg.Reload()
	})

	legacyPath := filepath.Join(dataDir, "jman", "jman.db")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}

	legacyConn, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacyConn.Exec(`
		CREATE TABLE plugin_info (slug TEXT PRIMARY KEY, name TEXT);
		INSERT INTO plugin_info (slug, name) VALUES ('test-plugin-2', 'Test Plugin 2');
	`)
	if err != nil {
		legacyConn.Close()
		t.Fatal(err)
	}
	legacyConn.Close()

	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		_ = db.Close()
		commands.MigratedJustNow = false
	}()
	os.Args = []string{"jman", "migrate-db"}

	exitCode := run()
	if exitCode != 0 {
		t.Fatalf("expected run() to return 0, got %d", exitCode)
	}

	// Legacy db should be renamed
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy db still exists at %s", legacyPath)
	}

	// Running migrate-db again when already migrated should succeed and report already migrated
	commands.MigratedJustNow = false
	exitCodeAgain := run()
	if exitCodeAgain != 0 {
		t.Fatalf("expected run() to return 0 on already migrated db, got %d", exitCodeAgain)
	}
}
