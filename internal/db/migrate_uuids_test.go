package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/utils"
)

// createLegacyDB writes a database file using the pre-UUID schema (INTEGER
// site/server id columns), the way an install from before the host-agnostic
// refactor looks on disk.
func createLegacyDB(t *testing.T, path string, statements ...string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("failed to open legacy db %s: %v", path, err)
	}
	defer conn.Close()
	for _, stmt := range statements {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("legacy db setup failed on %q: %v", stmt, err)
		}
	}
}

// setupLegacyDataDir points the package at a fresh data dir containing legacy
// inventory.db/api.db files, without initialising them.
func setupLegacyDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = dir
	t.Cleanup(func() {
		Close()
		config.RunData.DataDir = oldDataDir
	})

	createLegacyDB(t, filepath.Join(dir, "inventory.db"),
		`CREATE TABLE site_core (site_id INTEGER PRIMARY KEY, version TEXT NOT NULL, minor_update TEXT, major_update TEXT, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE site_plugins (site_id INTEGER NOT NULL, slug TEXT NOT NULL, status TEXT, version TEXT, update_available TEXT, auto_update BOOLEAN, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (site_id, slug))`,
		`CREATE TABLE ignore_entries (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL, target TEXT NOT NULL, reason TEXT, negated_site_ids TEXT, use_for_monitor BOOLEAN DEFAULT 0, use_for_vuln BOOLEAN DEFAULT 0, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, created_by TEXT, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_by TEXT)`,
		`INSERT INTO site_core (site_id, version) VALUES (42, '6.5.0')`,
		`INSERT INTO site_plugins (site_id, slug, version) VALUES (42, 'akismet', '5.1'), (42, 'yoast', '22.0'), (43, 'akismet', '5.0')`,
		`INSERT INTO ignore_entries (type, target, negated_site_ids) VALUES ('site', '42', '[]'), ('server', '12', '[42, 99]'), ('plugin', '123', '[]')`,
	)
	createLegacyDB(t, filepath.Join(dir, "api.db"),
		`CREATE TABLE tasks (id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending', priority TEXT NOT NULL DEFAULT 'medium', title TEXT NOT NULL, site_id INTEGER, server_id INTEGER)`,
		`CREATE TABLE agent_tokens (id INTEGER PRIMARY KEY AUTOINCREMENT, server_id INTEGER NOT NULL, server_name TEXT, token_hash TEXT NOT NULL, token_prefix TEXT NOT NULL)`,
		`CREATE TABLE site_disk_usage (site_id INTEGER NOT NULL, bytes_used INTEGER NOT NULL, measured_at DATETIME NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (site_id, measured_at))`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY AUTOINCREMENT, parent_type TEXT, parent_id TEXT, content TEXT)`,
		`INSERT INTO tasks (type, title, site_id, server_id) VALUES ('one-time', 'site task', 55, NULL), ('one-time', 'server task', NULL, 12), ('one-time', 'unlinked', NULL, NULL)`,
		`INSERT INTO agent_tokens (server_id, token_hash, token_prefix) VALUES (12, 'sha256:x', 'abcdefgh')`,
		`INSERT INTO site_disk_usage (site_id, bytes_used, measured_at) VALUES (55, 100, '2026-01-01T00:00:00Z'), (55, 200, '2026-01-02T00:00:00Z')`,
		`INSERT INTO notes (parent_type, parent_id, content) VALUES ('Site', '55', 'site note'), ('Organization', '3', 'org note')`,
	)
	return dir
}

func queryString(t *testing.T, conn *sql.DB, query string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := conn.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("query %q failed: %v", query, err)
	}
	return v.String
}

func queryInt(t *testing.T, conn *sql.DB, query string, args ...any) int {
	t.Helper()
	var v int
	if err := conn.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("query %q failed: %v", query, err)
	}
	return v
}

func TestMigrateLegacyIDs_FromPreUUIDSchema(t *testing.T) {
	setupLegacyDataDir(t)

	if err := InitInventory(); err != nil {
		t.Fatalf("InitInventory failed: %v", err)
	}
	if err := InitAPI(); err != nil {
		t.Fatalf("InitAPI failed: %v", err)
	}
	inv, api := GetInventoryDB(), GetAPIDB().DB

	site42, site43, site55, site99 := utils.SpinupWPSiteUUID(42), utils.SpinupWPSiteUUID(43), utils.SpinupWPSiteUUID(55), utils.SpinupWPSiteUUID(99)
	server12 := utils.SpinupWPServerUUID(12)

	if got := queryString(t, inv, `SELECT site_id FROM site_core`); got != site42 {
		t.Errorf("site_core.site_id = %q, want %q", got, site42)
	}
	if got := queryInt(t, inv, `SELECT COUNT(*) FROM site_plugins WHERE site_id = ?`, site42); got != 2 {
		t.Errorf("site_plugins rows for site 42 = %d, want 2", got)
	}
	if got := queryInt(t, inv, `SELECT COUNT(*) FROM site_plugins WHERE site_id = ?`, site43); got != 1 {
		t.Errorf("site_plugins rows for site 43 = %d, want 1", got)
	}

	if got := queryString(t, inv, `SELECT target FROM ignore_entries WHERE type = 'site'`); got != site42 {
		t.Errorf("site ignore target = %q, want %q", got, site42)
	}
	if got := queryString(t, inv, `SELECT target FROM ignore_entries WHERE type = 'server'`); got != server12 {
		t.Errorf("server ignore target = %q, want %q", got, server12)
	}
	if got := queryString(t, inv, `SELECT target FROM ignore_entries WHERE type = 'plugin'`); got != "123" {
		t.Errorf("plugin ignore target must be left alone, got %q", got)
	}
	var negated []string
	if err := json.Unmarshal([]byte(queryString(t, inv, `SELECT negated_site_ids FROM ignore_entries WHERE type = 'server'`)), &negated); err != nil {
		t.Fatalf("negated_site_ids is not a JSON string array: %v", err)
	}
	if len(negated) != 2 || negated[0] != site42 || negated[1] != site99 {
		t.Errorf("negated_site_ids = %v, want [%s %s]", negated, site42, site99)
	}

	if got := queryString(t, api, `SELECT site_id FROM tasks WHERE title = 'site task'`); got != site55 {
		t.Errorf("task site_id = %q, want %q", got, site55)
	}
	if got := queryString(t, api, `SELECT server_id FROM tasks WHERE title = 'server task'`); got != server12 {
		t.Errorf("task server_id = %q, want %q", got, server12)
	}
	if got := queryInt(t, api, `SELECT COUNT(*) FROM tasks WHERE title = 'unlinked' AND site_id IS NULL AND server_id IS NULL`); got != 1 {
		t.Errorf("unlinked task must keep NULL links")
	}
	if got := queryString(t, api, `SELECT server_id FROM agent_tokens`); got != server12 {
		t.Errorf("agent_tokens.server_id = %q, want %q", got, server12)
	}
	if got := queryInt(t, api, `SELECT COUNT(*) FROM site_disk_usage WHERE site_id = ?`, site55); got != 2 {
		t.Errorf("site_disk_usage rows for site 55 = %d, want 2", got)
	}
	if got := queryString(t, api, `SELECT parent_id FROM notes WHERE parent_type = 'Site'`); got != site55 {
		t.Errorf("site note parent_id = %q, want %q", got, site55)
	}
	if got := queryString(t, api, `SELECT parent_id FROM notes WHERE parent_type = 'Organization'`); got != "3" {
		t.Errorf("organization note parent_id must be left alone, got %q", got)
	}

	// The migrated values are readable through the typed repositories.
	claims, err := ListAgentTokens()
	if err != nil || len(claims) != 1 || claims[0].ServerID != server12 {
		t.Errorf("ListAgentTokens() = %+v, %v; want one token for %s", claims, err, server12)
	}

	// The migration is recorded, and a restart leaves the data unchanged.
	for name, conn := range map[string]*sql.DB{"inventory": inv, "api": api} {
		if got := queryInt(t, conn, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, uuidMigrationName); got != 1 {
			t.Errorf("%s: migration not recorded", name)
		}
	}
	Close()
	if err := InitInventory(); err != nil {
		t.Fatalf("second InitInventory failed: %v", err)
	}
	if err := InitAPI(); err != nil {
		t.Fatalf("second InitAPI failed: %v", err)
	}
	if got := queryInt(t, GetInventoryDB(), `SELECT COUNT(*) FROM site_plugins`); got != 3 {
		t.Errorf("site_plugins rows after restart = %d, want 3", got)
	}
	if got := queryString(t, GetAPIDB().DB, `SELECT site_id FROM tasks WHERE title = 'site task'`); got != site55 {
		t.Errorf("task site_id after restart = %q, want %q", got, site55)
	}
}

// TestMigrateLegacyIDs_PartiallyMigrated covers a database written by the
// broken first UUID release: current (TEXT) schema, a mix of UUID and legacy
// rows, and "123.0" values from JSON numbers decoded as float64.
func TestMigrateLegacyIDs_PartiallyMigrated(t *testing.T) {
	setupTaskRepoTest(t)
	inv, api := GetInventoryDB(), GetAPIDB().DB

	for _, conn := range []*sql.DB{inv, api} {
		if _, err := conn.Exec(`DELETE FROM schema_migrations`); err != nil {
			t.Fatalf("failed to reset schema_migrations: %v", err)
		}
	}

	site7 := utils.SpinupWPSiteUUID(7)
	mustExec := func(conn *sql.DB, q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(q, args...); err != nil {
			t.Fatalf("exec %q failed: %v", q, err)
		}
	}
	// An older, already-migrated row and a newer legacy row for the same site.
	mustExec(inv, `INSERT INTO site_core (site_id, version) VALUES (?, '6.0.0')`, site7)
	mustExec(inv, `INSERT INTO site_core (site_id, version) VALUES ('7', '6.5.0')`)
	mustExec(inv, `INSERT INTO ignore_entries (type, target, negated_site_ids) VALUES ('server', '3', ?)`, `["7", 8, "`+site7+`"]`)
	mustExec(api, `INSERT INTO tasks (type, title, site_id) VALUES ('one-time', 'float id', '7.0')`)
	mustExec(api, `INSERT INTO tasks (type, title, site_id) VALUES ('one-time', 'already uuid', ?)`, site7)

	if err := MigrateLegacyInventoryIDs(inv); err != nil {
		t.Fatalf("MigrateLegacyInventoryIDs failed: %v", err)
	}
	if err := MigrateLegacyAPIIDs(api); err != nil {
		t.Fatalf("MigrateLegacyAPIIDs failed: %v", err)
	}

	if got := queryInt(t, inv, `SELECT COUNT(*) FROM site_core WHERE site_id = ?`, site7); got != 1 {
		t.Fatalf("site_core rows for site 7 = %d, want 1", got)
	}
	if got := queryString(t, inv, `SELECT version FROM site_core WHERE site_id = ?`, site7); got != "6.5.0" {
		t.Errorf("site_core collision kept version %q, want the legacy (newer) 6.5.0", got)
	}
	var negated []string
	if err := json.Unmarshal([]byte(queryString(t, inv, `SELECT negated_site_ids FROM ignore_entries`)), &negated); err != nil {
		t.Fatalf("negated_site_ids is not a JSON string array: %v", err)
	}
	if len(negated) != 3 || negated[0] != site7 || negated[1] != utils.SpinupWPSiteUUID(8) || negated[2] != site7 {
		t.Errorf("negated_site_ids = %v", negated)
	}
	if got := queryInt(t, api, `SELECT COUNT(*) FROM tasks WHERE site_id = ?`, site7); got != 2 {
		t.Errorf("tasks linked to site 7 = %d, want 2 (\"7.0\" migrated, UUID kept)", got)
	}
}

func TestParseLegacyID(t *testing.T) {
	tests := []struct {
		raw  string
		want int
		ok   bool
	}{
		{"123", 123, true},
		{" 123 ", 123, true},
		{"123.0", 123, true},
		{"1.23e+06", 1230000, true},
		{"123.5", 0, false},
		{"0", 0, false},
		{"-4", 0, false},
		{"", 0, false},
		{"abc", 0, false},
		{utils.SpinupWPSiteUUID(1), 0, false},
	}
	for _, tt := range tests {
		got, ok := parseLegacyID(tt.raw)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseLegacyID(%q) = %d, %v; want %d, %v", tt.raw, got, ok, tt.want, tt.ok)
		}
	}
}
