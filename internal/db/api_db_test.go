package db

import (
	"database/sql"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestAPIDBStoresTimesAsCanonicalUTC(t *testing.T) {
	setupTestAPIDB(t)
	api := GetAPIDB()

	helsinki := time.FixedZone("EEST", 3*3600)
	local := time.Date(2026, 10, 2, 8, 54, 39, 0, helsinki)
	ptr := time.Date(2026, 10, 2, 9, 0, 0, 0, helsinki)

	if _, err := api.Exec(`INSERT INTO organizations (name, created_at, updated_at) VALUES ('t', ?, ?)`, local, &ptr); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var createdRaw, updatedRaw string
	// CAST stops the driver from parsing the DATETIME column back into a time.Time.
	if err := api.QueryRow(`SELECT CAST(created_at AS TEXT), CAST(updated_at AS TEXT) FROM organizations WHERE name = 't'`).Scan(&createdRaw, &updatedRaw); err != nil {
		t.Fatalf("select: %v", err)
	}
	if want := "2026-10-02 05:54:39+00:00"; createdRaw != want {
		t.Errorf("created_at stored as %q, want %q", createdRaw, want)
	}
	if want := "2026-10-02 06:00:00+00:00"; updatedRaw != want {
		t.Errorf("updated_at stored as %q, want %q", updatedRaw, want)
	}

	// A comparison with a time in a different zone must be chronological.
	cutoff := time.Date(2026, 10, 2, 5, 55, 0, 0, time.UTC)
	var n int
	if err := api.QueryRow(`SELECT COUNT(*) FROM organizations WHERE created_at < ?`, cutoff).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("created_at < %v matched %d rows, want 1", cutoff, n)
	}

	var back time.Time
	if err := api.QueryRow(`SELECT created_at FROM organizations WHERE name = 't'`).Scan(&back); err != nil {
		t.Fatalf("scan back: %v", err)
	}
	if !back.Equal(local) {
		t.Errorf("round trip gave %v, want %v", back, local)
	}
}

func TestNormalizeArgs(t *testing.T) {
	zone := time.FixedZone("X", -5*3600)
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, zone)
	var nilTime *time.Time
	args := []any{"a", ts, &ts, nilTime, sql.NullTime{Time: ts, Valid: true}, sql.NullTime{}, 42}

	out := normalizeArgs(args)

	if out[1].(time.Time).Location() != time.UTC {
		t.Errorf("time.Time not converted to UTC")
	}
	if out[2].(*time.Time).Location() != time.UTC {
		t.Errorf("*time.Time not converted to UTC")
	}
	if ts.Location() != zone {
		t.Errorf("caller's time was mutated")
	}
	if out[3].(*time.Time) != nil {
		t.Errorf("nil *time.Time changed")
	}
	if out[4].(sql.NullTime).Time.Location() != time.UTC {
		t.Errorf("sql.NullTime not converted to UTC")
	}
	if args[1].(time.Time).Location() != zone {
		t.Errorf("input slice was modified")
	}

	plain := []any{"a", 1}
	if got := normalizeArgs(plain); &got[0] != &plain[0] {
		t.Errorf("slice without times was copied")
	}
}

// TestInsertsReturnIDs covers the INSERT ... RETURNING id paths that
// replaced LastInsertId: each saved row gets the id the database assigned.
func TestInsertsReturnIDs(t *testing.T) {
	setupTestAPIDB(t)

	var ids []int
	for _, name := range []string{"first", "second"} {
		org := models.Organization{Name: name}
		if err := SaveOrganization(&org, "test"); err != nil {
			t.Fatalf("SaveOrganization: %v", err)
		}
		got, err := GetOrganization(org.ID)
		if err != nil || got == nil || got.Name != name {
			t.Fatalf("organization %d: got %+v, err %v; want name %q", org.ID, got, err, name)
		}
		ids = append(ids, org.ID)
	}
	if ids[0] == 0 || ids[0] == ids[1] {
		t.Errorf("organization ids %v are not distinct non-zero values", ids)
	}

	inc, err := CreateIncident("example.com", "down", 500, time.Now())
	if err != nil {
		t.Fatalf("CreateIncident: %v", err)
	}
	stored, err := GetIncidentByID(inc.ID)
	if err != nil || stored == nil || stored.Domain != "example.com" {
		t.Fatalf("incident %d: got %+v, err %v", inc.ID, stored, err)
	}
}

// TestCaseInsensitiveLookups pins behavior that used to come from SQLite's
// case-insensitive LIKE and COLLATE NOCASE, so it survives a database
// without them.
func TestCaseInsensitiveLookups(t *testing.T) {
	setupTestAPIDB(t)

	org := models.Organization{Name: "Acme Oy"}
	if err := SaveOrganization(&org, "test"); err != nil {
		t.Fatal(err)
	}
	found, err := GetAllOrganizations("ACME")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Errorf("search ACME found %d organizations, want 1", len(found))
	}

	inc, err := CreateIncident("Shop.Example.COM", "down", 500, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if inc.Domain != "shop.example.com" {
		t.Errorf("incident domain stored as %q, want lowercase", inc.Domain)
	}
	active, err := GetActiveIncidentByDomain("SHOP.example.com")
	if err != nil || active == nil || active.ID != inc.ID {
		t.Errorf("mixed-case lookup found %+v, err %v; want incident %d", active, err, inc.ID)
	}

	if _, err := GetAPIDB().Exec(`INSERT INTO monitor_status (domain) VALUES ('shop.example.com')`); err != nil {
		t.Fatal(err)
	}
	status, err := GetMonitorStatus("Shop.Example.com")
	if err != nil || status == nil {
		t.Errorf("GetMonitorStatus with mixed case: %+v, %v", status, err)
	}
}
