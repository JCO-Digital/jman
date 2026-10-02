package db

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// rerunTimestampMigration forgets that the migration ran and runs it
// again, after a test has seeded legacy-format rows through the raw
// (unwrapped) handle.
func rerunTimestampMigration(t *testing.T) {
	t.Helper()
	raw := GetAPIDB().DB
	if _, err := raw.Exec(`DELETE FROM schema_migrations WHERE name = ?`, timestampMigrationName); err != nil {
		t.Fatalf("reset migration: %v", err)
	}
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if err := initAPISchema(); err != nil {
		t.Fatalf("initAPISchema: %v", err)
	}
}

func storedText(t *testing.T, query string, args ...any) string {
	t.Helper()
	var s string
	if err := GetAPIDB().DB.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return s
}

func TestParseStoredTime(t *testing.T) {
	want := time.Date(2026, 10, 2, 5, 54, 39, 400000000, time.UTC)
	for _, in := range []string{
		"2026-10-02 08:54:39.4 +0300 EEST m=+156.950703675",
		"2026-10-02 08:54:39.4 +0300 EEST",
		"2026-10-02 05:54:39.4 +0000 UTC m=+4700.87",
		"2026-10-02T05:54:39.4Z",
		"2026-10-02T08:54:39.4+03:00",
		"2026-10-02 05:54:39.4+00:00",
		"2026-10-02 05:54:39.4",
	} {
		got, ok := parseStoredTime(in)
		if !ok || !got.Equal(want) {
			t.Errorf("parseStoredTime(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := parseStoredTime("not a time"); ok {
		t.Errorf("parseStoredTime accepted garbage")
	}
}

func TestNormalizeAPITimestamps(t *testing.T) {
	setupTestAPIDB(t)
	raw := GetAPIDB().DB

	// Seed rows the way older versions wrote them, bypassing APIDB.
	seed := []string{
		`INSERT INTO organizations (id, name, created_at, updated_at) VALUES
			(1, 'local', '2026-10-02 08:54:39.4 +0300 EEST m=+156.95', '2026-03-26 16:57:01'),
			(2, 'utc',   '2026-05-03 20:40:39.299090079 +0000 UTC m=+4700.87', 'garbage')`,
		`INSERT INTO site_disk_usage (site_id, bytes_used, measured_at) VALUES
			('s1', 10, '2026-09-28T13:30:40Z'),
			('s1', 11, '2026-09-28 13:30:40+00:00')`,
		`INSERT INTO monitor_status (domain) VALUES ('Example.COM')`,
		`INSERT INTO incidents (domain, error_message, error_code, down_since, status) VALUES ('Mixed.Example', 'x', 500, '2026-10-02T05:00:00Z', 'open')`,
	}
	for _, q := range seed {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	rerunTimestampMigration(t)

	checks := []struct{ query, want string }{
		{`SELECT CAST(created_at AS TEXT) FROM organizations WHERE id = 1`, "2026-10-02 05:54:39.4+00:00"},
		{`SELECT CAST(created_at AS TEXT) FROM organizations WHERE id = 2`, "2026-05-03 20:40:39.299090079+00:00"},
		// CURRENT_TIMESTAMP text is already consistent and left alone.
		{`SELECT CAST(updated_at AS TEXT) FROM organizations WHERE id = 1`, "2026-03-26 16:57:01"},
		// Unparseable values are left untouched rather than failing startup.
		{`SELECT CAST(updated_at AS TEXT) FROM organizations WHERE id = 2`, "garbage"},
		{`SELECT CAST(down_since AS TEXT) FROM incidents`, "2026-10-02 05:00:00+00:00"},
		{`SELECT domain FROM monitor_status`, "example.com"},
		{`SELECT domain FROM incidents`, "mixed.example"},
	}
	for _, c := range checks {
		if got := storedText(t, c.query); got != c.want {
			t.Errorf("%s = %q, want %q", c.query, got, c.want)
		}
	}

	// The RFC3339 row collided with an already-canonical row for the same
	// instant; the canonical row wins.
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM site_disk_usage WHERE site_id = 's1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("site_disk_usage has %d rows for s1, want 1 after dropping the duplicate", n)
	}
	if got := storedText(t, `SELECT bytes_used FROM site_disk_usage WHERE site_id = 's1'`); got != "11" {
		t.Errorf("kept bytes_used %s, want the canonical row's 11", got)
	}

	// Recorded as applied: a second startup leaves new legacy text alone.
	if _, err := raw.Exec(`UPDATE organizations SET created_at = '2026-01-01T00:00:00Z' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	dbMutex.Lock()
	err := initAPISchema()
	dbMutex.Unlock()
	if err != nil {
		t.Fatalf("second initAPISchema: %v", err)
	}
	if got := storedText(t, `SELECT CAST(created_at AS TEXT) FROM organizations WHERE id = 1`); got != "2026-01-01T00:00:00Z" {
		t.Errorf("migration ran twice: created_at = %q", got)
	}
}

// TestBeforeFilterAcrossOffsets is the bug the canonical encoding fixes:
// a next_billing written from a UTC+3 time used to be stored with its
// local wall-clock time and compared as text against the cutoff.
func TestBeforeFilterAcrossOffsets(t *testing.T) {
	setupTestAPIDB(t)
	api := GetAPIDB()

	if _, err := api.Exec(`INSERT INTO organizations (id, name) VALUES (1, 'org')`); err != nil {
		t.Fatal(err)
	}
	eest := time.FixedZone("EEST", 3*3600)
	// 2026-11-01 01:00 EEST is 2026-10-31 22:00 UTC: due on Oct 31 in UTC.
	due := time.Date(2026, 11, 1, 1, 0, 0, 0, eest)
	oa := models.OrganizationAsset{OrganizationID: 1, Identifier: "a", BillingFreq: models.BillingFrequencyYearly, NextBilling: &due, Status: models.AssetStatusActive}
	if err := SaveOrganizationAsset(&oa, "test"); err != nil {
		t.Fatal(err)
	}

	endOct, err := EndOfDayUTC("2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetAllOrganizationAssets("", "", &endOct)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("before=2026-10-31 matched %d assets, want 1", len(got))
	}

	endOct30, _ := EndOfDayUTC("2026-10-30")
	if got, _ := GetAllOrganizationAssets("", "", &endOct30); len(got) != 0 {
		t.Fatalf("before=2026-10-30 matched %d assets, want 0", len(got))
	}
}
