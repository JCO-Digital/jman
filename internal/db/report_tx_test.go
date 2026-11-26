package db

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestRecordSiteTrafficSkipsBadRows(t *testing.T) {
	setupTestAPIDB(t)
	var errs []error
	stored, err := RecordSiteTraffic("s1",
		[]models.TrafficHourlyEntry{
			{Hour: "2026-11-01T10:00:00Z", RequestsTotal: 5},
			{Hour: "not a time", RequestsTotal: 1},
			{Hour: "2026-11-01T11:00:00Z", RequestsTotal: 7},
		},
		[]models.TrafficDailyEntry{{Day: "2026-10-01", RequestsTotal: 99}},
		func(err error) { errs = append(errs, err) })
	if err != nil || len(stored) != 2 || len(errs) != 1 {
		t.Fatalf("stored %d, %d errors, %v", len(stored), len(errs), err)
	}
	var hours, days int
	if err := GetAPIDB().QueryRow(`SELECT COUNT(*) FROM site_traffic_hourly WHERE site_id = 's1'`).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if err := GetAPIDB().QueryRow(`SELECT COUNT(*) FROM site_traffic_daily WHERE site_id = 's1'`).Scan(&days); err != nil {
		t.Fatal(err)
	}
	if hours != 2 || days != 1 {
		t.Errorf("rows = %d hourly, %d daily; want 2 and 1", hours, days)
	}
}

func TestUpsertScanFindingsInOneTransaction(t *testing.T) {
	setupTestAPIDB(t)
	// A non-UTC time must be stored as UTC, as outside a transaction.
	seen := time.Date(2026, 11, 1, 12, 0, 0, 0, time.FixedZone("EET", 2*3600))
	findings := []models.ScanFinding{
		{Engine: "agent", Rule: "uploads-php", Severity: "high", Path: "wp-content/uploads/a.php", SHA256: "aa"},
		{Engine: "agent", Rule: "uploads-php", Severity: "high", Path: "wp-content/uploads/b.php", SHA256: "bb"},
	}
	var errs []error
	if err := UpsertScanFindings("s1", findings, "1", seen, func(err error) { errs = append(errs, err) }); err != nil || len(errs) != 0 {
		t.Fatalf("err %v, row errors %v", err, errs)
	}
	var n int
	var lastSeen string
	if err := GetAPIDB().QueryRow(`SELECT COUNT(*), MAX(last_seen_at) FROM scan_findings WHERE site_id = 's1'`).Scan(&n, &lastSeen); err != nil {
		t.Fatal(err)
	}
	if n != 2 || !strings.HasPrefix(lastSeen, "2026-11-01 10:00:00") || !strings.HasSuffix(lastSeen, "+00:00") {
		t.Errorf("%d findings, last_seen_at %q; want 2 at 10:00 UTC", n, lastSeen)
	}
}

func TestInAPITxRollsBack(t *testing.T) {
	setupTestAPIDB(t)
	boom := errors.New("boom")
	err := inAPITx(func(ex execer) error {
		if _, err := ex.Exec(`INSERT INTO monitor_history (domain, status, error_code) VALUES ('x', 'up', 0)`); err != nil {
			return err
		}
		return boom
	})
	var n int
	_ = GetAPIDB().QueryRow(`SELECT COUNT(*) FROM monitor_history`).Scan(&n)
	if !errors.Is(err, boom) || n != 0 {
		t.Errorf("err %v, %d rows left", err, n)
	}
}
