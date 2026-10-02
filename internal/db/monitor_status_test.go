package db

import (
	"testing"
	"time"
)

func TestMonitorStatusRecordRoundTrip(t *testing.T) {
	setupTestAPIDB(t)

	checked := time.Date(2026, 10, 2, 6, 0, 0, 0, time.UTC)
	in := MonitorStatusRecord{
		Domain:       "Example.com",
		IsDown:       true,
		FailureCount: 3,
		CurrentMode:  "alert",
		LastChecked:  checked,
		NextCheckAt:  checked.Add(time.Minute),
		DownSince:    checked.Add(-time.Hour),
		PDTriggered:  true,
	}
	if err := SaveMonitorStatusRecord(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	in.FailureCount = 4 // second save takes the ON CONFLICT update path
	if err := SaveMonitorStatusRecord(in); err != nil {
		t.Fatalf("update: %v", err)
	}

	records, err := GetAllMonitorStatusRecords()
	if err != nil || len(records) != 1 {
		t.Fatalf("got %d records, err %v; want 1", len(records), err)
	}
	got := records[0]
	if got.Domain != "example.com" || got.FailureCount != 4 || !got.IsDown || !got.PDTriggered || got.CurrentMode != "alert" {
		t.Errorf("unexpected record %+v", got)
	}
	if !got.LastChecked.Equal(in.LastChecked) || !got.DownSince.Equal(in.DownSince) {
		t.Errorf("times did not round-trip: %+v", got)
	}
	if !got.LastAlertTime.IsZero() {
		t.Errorf("zero LastAlertTime came back as %v", got.LastAlertTime)
	}

	if err := ResetMonitorStatus("EXAMPLE.com"); err != nil {
		t.Fatal(err)
	}
	records, _ = GetAllMonitorStatusRecords()
	if r := records[0]; r.IsDown || r.CurrentMode != "normal" || !r.DownSince.IsZero() || r.PDTriggered {
		t.Errorf("reset left %+v", r)
	}

	if err := DeleteMonitorStatus("Example.COM"); err != nil {
		t.Fatal(err)
	}
	if records, _ = GetAllMonitorStatusRecords(); len(records) != 0 {
		t.Errorf("delete left %d records", len(records))
	}
}

func TestRecordMonitorHistory(t *testing.T) {
	setupTestAPIDB(t)

	for _, s := range []string{"UP", "UP", "HTTP 500", "UP"} {
		if err := RecordMonitorHistory("Example.com", s, 0); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := GetAPIDB().Query(`SELECT domain, status, count FROM monitor_history ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type entry struct {
		domain, status string
		count          int
	}
	var got []entry
	for rows.Next() {
		var e entry
		rows.Scan(&e.domain, &e.status, &e.count)
		got = append(got, e)
	}
	want := []entry{{"example.com", "UP", 2}, {"example.com", "HTTP 500", 1}, {"example.com", "UP", 1}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %v, want %v", i, got[i], want[i])
		}
	}
}
