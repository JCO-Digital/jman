package db

import "testing"

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
