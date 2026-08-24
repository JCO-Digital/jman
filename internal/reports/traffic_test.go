package reports

import (
	"net/url"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestTrafficReport_Run(t *testing.T) {
	setupReportsTest(t)

	// Two days for site 1 (must be summed into one row) plus one day for
	// site 2, to check both aggregation and grouping by site.
	if err := db.UpsertSiteTrafficDaily(1, models.TrafficDailyEntry{
		Day: "2026-01-05", RequestsTotal: 100, RequestsHuman: 80, RequestsBot: 20, UniqueVisitors: 15,
	}); err != nil {
		t.Fatalf("failed to seed daily traffic: %v", err)
	}
	if err := db.UpsertSiteTrafficDaily(1, models.TrafficDailyEntry{
		Day: "2026-01-06", RequestsTotal: 50, RequestsHuman: 40, RequestsBot: 10, UniqueVisitors: 8,
	}); err != nil {
		t.Fatalf("failed to seed daily traffic: %v", err)
	}
	if err := db.UpsertSiteTrafficDaily(2, models.TrafficDailyEntry{
		Day: "2026-01-05", RequestsTotal: 30, RequestsHuman: 25, RequestsBot: 5, UniqueVisitors: 6,
	}); err != nil {
		t.Fatalf("failed to seed daily traffic: %v", err)
	}

	r := &trafficReport{}
	result, err := r.Run(url.Values{"start": {"2026-01-01"}, "end": {"2026-01-31"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows (one per site), got %d: %+v", len(result.Rows), result.Rows)
	}

	// Ordered by site ID.
	site1, site2 := result.Rows[0], result.Rows[1]
	// No sites.json cache seeded, so the site falls back to a "Site N" label.
	if site1["site"] != "Site 1" {
		t.Errorf("rows[0][site] = %v, want fallback label \"Site 1\"", site1["site"])
	}
	if site1["requests_total"] != 150 || site1["requests_human"] != 120 || site1["requests_bot"] != 30 || site1["unique_visitors"] != 23 {
		t.Errorf("site 1 totals = %+v, want total=150 human=120 bot=30 unique=23 (summed across both days)", site1)
	}
	if site2["site"] != "Site 2" {
		t.Errorf("rows[1][site] = %v, want fallback label \"Site 2\"", site2["site"])
	}
	if site2["requests_total"] != 30 {
		t.Errorf("site 2 requests_total = %v, want 30", site2["requests_total"])
	}
}

func TestTrafficReport_Run_EmptyRange(t *testing.T) {
	setupReportsTest(t)

	r := &trafficReport{}
	result, err := r.Run(url.Values{"start": {"2026-01-01"}, "end": {"2026-01-31"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows for a range with no data, got %d", len(result.Rows))
	}
	if len(result.Columns) == 0 {
		t.Error("expected columns to still be populated even with no rows")
	}
}

func TestTrafficReport_Run_InvalidDateIsBadRequest(t *testing.T) {
	setupReportsTest(t)

	r := &trafficReport{}
	if _, err := r.Run(url.Values{"start": {"nope"}}); err == nil {
		t.Error("Run() with an invalid start date should error")
	}
}
