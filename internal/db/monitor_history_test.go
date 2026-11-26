package db

import (
	"testing"
	"time"
)

func TestGetMonitorHistoryByDomain(t *testing.T) {
	setupTestAPIDB(t)
	for _, h := range []struct{ check, domain, status string }{
		{"", "a.example.com", "up"},
		{"", "a.example.com", "down"},
		{"", "b.example.com", "up"},
	} {
		if _, err := GetAPIDB().Exec(`INSERT INTO monitor_history (domain, status, error_code, first_seen, last_seen, count) VALUES (?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1)`, h.domain, h.status); err != nil {
			t.Fatal(err)
		}
	}
	all, err := GetMonitorHistory(48, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("all = %d rows, %v", len(all), err)
	}
	a, err := GetMonitorHistory(48, "a.example.com")
	if err != nil || len(a) != 2 || a[0].Domain != "a.example.com" || a[1].Domain != "a.example.com" {
		t.Fatalf("a.example.com = %+v, %v", a, err)
	}
}

func TestPruneMonitorHistory(t *testing.T) {
	setupTestAPIDB(t)
	for _, ago := range []string{"-200 days", "-100 days", "-1 days"} {
		if _, err := GetAPIDB().Exec(`INSERT INTO monitor_history (domain, status, error_code, first_seen, last_seen, count) VALUES ('a.example.com', 'up', 0, datetime('now', '-300 days'), datetime('now', ?), 1)`, ago); err != nil {
			t.Fatal(err)
		}
	}
	n, err := PruneMonitorHistory(time.Now().Add(-90 * 24 * time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("pruned %d, %v; want 2", n, err)
	}
	// The run that is still going stays, however old its start.
	left, err := GetMonitorHistory(24*365, "")
	if err != nil || len(left) != 1 {
		t.Fatalf("left = %+v, %v", left, err)
	}
}
