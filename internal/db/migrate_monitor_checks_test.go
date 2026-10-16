package db

import "testing"

func TestMigrateMonitorChecks(t *testing.T) {
	setupTestDBDir(t)
	if err := InitInventory(); err != nil {
		t.Fatal(err)
	}
	if err := InitAPI(); err != nil {
		t.Fatal(err)
	}
	inv := GetInventoryDB()
	api := GetAPIDB().DB

	// Seed the pre-checks state: has_monitoring, monitor ignore rules and
	// domain-keyed monitor data.
	for _, q := range []string{
		`INSERT INTO sites (id, server_id, provider, domain, has_monitoring) VALUES
			('site-a', 'srv-1', 'spinupwp', 'a.example', 1),
			('site-b', 'srv-1', 'manual', 'b.example', 1),
			('site-c', 'srv-2', 'manual', 'c.example', 1),
			('site-d', 'srv-2', 'manual', 'd.example', 1),
			('site-e', NULL, 'manual', 'e.example', 0)`,
		// b: site rule shared with vuln scanning; c: covered by a
		// monitor-only server rule that negates d.
		`INSERT INTO ignore_entries (type, target, negated_site_ids, use_for_monitor, use_for_vuln) VALUES
			('site', 'site-b', '[]', 1, 1),
			('server', 'srv-2', '["site-d"]', 1, 0)`,
	} {
		if _, err := inv.Exec(q); err != nil {
			t.Fatalf("seed inventory %q: %v", q, err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE monitor_status (domain TEXT PRIMARY KEY COLLATE NOCASE, is_down BOOLEAN DEFAULT 0,
			failure_count INTEGER DEFAULT 0, consecutive_successes INTEGER DEFAULT 0, current_mode TEXT DEFAULT 'normal',
			last_alert_time DATETIME, last_checked DATETIME, next_check_at DATETIME, down_since DATETIME,
			pd_triggered BOOLEAN DEFAULT 0, pd_escalated BOOLEAN DEFAULT 0)`,
		`INSERT INTO monitor_status (domain, is_down, failure_count, current_mode) VALUES ('A.Example', 1, 3, 'alert')`,
		`INSERT INTO monitor_history (domain, status) VALUES ('a.example', 'UP'), ('gone.example', 'UP')`,
		`INSERT INTO incidents (domain, down_since) VALUES ('b.example', '2026-10-01 00:00:00'), ('gone.example', '2026-10-01 00:00:00')`,
		`DELETE FROM schema_migrations WHERE name = 'monitor_checks'`,
	} {
		if _, err := api.Exec(q); err != nil {
			t.Fatalf("seed api %q: %v", q, err)
		}
	}
	if _, err := inv.Exec(`DELETE FROM schema_migrations WHERE name = 'monitor_ignores_to_checks'`); err != nil {
		t.Fatal(err)
	}

	if err := MigrateMonitorChecks(api, inv); err != nil {
		t.Fatalf("MigrateMonitorChecks: %v", err)
	}

	checks, err := ListMonitorChecks()
	if err != nil {
		t.Fatal(err)
	}
	checkFor := map[string]string{}
	wantEnabled := map[string]bool{"site-a": true, "site-b": false, "site-c": false, "site-d": true, "site-e": false}
	for _, c := range checks {
		if !c.IsDefault || c.Path != "/" || c.Expect != "html" || c.Type != "http" {
			t.Errorf("unexpected default check %+v", c)
		}
		if want, ok := wantEnabled[c.SiteID]; !ok || c.Enabled != want {
			t.Errorf("check for %s: enabled = %v, want %v", c.SiteID, c.Enabled, want)
		}
		checkFor[c.SiteID] = c.ID
	}
	if len(checks) != len(wantEnabled) {
		t.Errorf("got %d checks, want %d", len(checks), len(wantEnabled))
	}

	records, err := GetAllMonitorStatusRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].CheckID != checkFor["site-a"] || records[0].Domain != "a.example" ||
		!records[0].IsDown || records[0].FailureCount != 3 || records[0].CurrentMode != "alert" {
		t.Errorf("monitor status not carried over: %+v", records)
	}
	if exists := storedText(t, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'monitor_status'`); exists != "0" {
		t.Errorf("monitor_status still exists")
	}

	if got := storedText(t, `SELECT group_concat(check_id || '=' || domain) FROM monitor_history`); got != checkFor["site-a"]+"=a.example" {
		t.Errorf("monitor_history = %q, want only a.example re-keyed", got)
	}
	if got := storedText(t, `SELECT group_concat(check_id || '=' || domain) FROM incidents`); got != checkFor["site-b"]+"=b.example" {
		t.Errorf("incidents = %q, want only b.example re-keyed", got)
	}

	var entries []string
	rows, err := inv.Query(`SELECT type || ':' || use_for_monitor || use_for_vuln FROM ignore_entries ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, e)
	}
	if len(entries) != 1 || entries[0] != "site:01" {
		t.Errorf("ignore entries after migration = %v, want only the vuln site rule with the monitor flag cleared", entries)
	}

	// Recorded: a second run changes nothing.
	if err := MigrateMonitorChecks(api, inv); err != nil {
		t.Fatal(err)
	}
	if again, _ := ListMonitorChecks(); len(again) != len(checks) {
		t.Errorf("second run created checks: %d, want %d", len(again), len(checks))
	}
}
