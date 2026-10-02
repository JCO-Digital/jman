package db

import (
	"testing"
	"time"
)

func TestDownsampleOldSiteDiskUsage(t *testing.T) {
	setupTestAPIDB(t)

	const siteA = "11111111-1111-1111-1111-111111111111"
	const siteB = "22222222-2222-2222-2222-222222222222"

	// Old days get several measurements each; recent ones must survive untouched.
	records := []struct {
		site, at string
		bytes    int64
	}{
		{siteA, "2026-08-01T01:00:00Z", 100},
		{siteA, "2026-08-01T12:00:00Z", 110},
		{siteA, "2026-08-01T23:50:00Z", 120}, // kept: latest of 08-01
		{siteA, "2026-08-02T08:00:00Z", 130}, // kept: only one on 08-02
		{siteA, "2026-09-20T10:00:00Z", 200}, // after cutoff: kept
		{siteA, "2026-09-20T10:10:00Z", 210}, // after cutoff: kept
		{siteB, "2026-08-01T05:00:00Z", 300},
		{siteB, "2026-08-01T06:00:00Z", 310}, // kept: latest of 08-01
	}
	for _, r := range records {
		if err := RecordSiteDiskUsage(r.site, r.bytes, r.at); err != nil {
			t.Fatalf("RecordSiteDiskUsage: %v", err)
		}
	}

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := DownsampleOldSiteDiskUsage(cutoff); err != nil {
		t.Fatalf("DownsampleOldSiteDiskUsage: %v", err)
	}

	rows, err := GetAPIDB().Query(`SELECT site_id, measured_at, bytes_used FROM site_disk_usage ORDER BY site_id, measured_at`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	type row struct {
		site, at string
		bytes    int64
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.site, &r.at, &r.bytes); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}

	want := []row{
		{siteA, "2026-08-01T23:50:00Z", 120},
		{siteA, "2026-08-02T08:00:00Z", 130},
		{siteA, "2026-09-20T10:00:00Z", 200},
		{siteA, "2026-09-20T10:10:00Z", 210},
		{siteB, "2026-08-01T06:00:00Z", 310},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows %v, want %d rows %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: got %v, want %v", i, got[i], want[i])
		}
	}

	latest, err := GetLatestSiteDiskUsage()
	if err != nil {
		t.Fatalf("GetLatestSiteDiskUsage: %v", err)
	}
	if latest[siteA].BytesUsed != 210 || latest[siteB].BytesUsed != 310 {
		t.Errorf("unexpected latest usage after downsampling: %+v", latest)
	}
}
