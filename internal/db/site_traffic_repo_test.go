package db

import (
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestPruneOldSiteTrafficHourly(t *testing.T) {
	setupTaskRepoTest(t)

	const siteID = 42
	now := time.Now().UTC()
	oldHour := now.Add(-72 * time.Hour).Truncate(time.Hour)
	recentHour := now.Add(-1 * time.Hour).Truncate(time.Hour)

	oldEntry := models.TrafficHourlyEntry{
		Hour:           oldHour.Format(time.RFC3339),
		RequestsTotal:  5,
		RequestsHuman:  4,
		RequestsBot:    1,
		UniqueVisitors: 3,
		TopPages:       []models.TrafficTopEntry{{Key: "/", Count: 5}},
		TopReferrers:   []models.TrafficTopEntry{{Key: "https://google.com/", Count: 2}},
	}
	recentEntry := models.TrafficHourlyEntry{
		Hour:           recentHour.Format(time.RFC3339),
		RequestsTotal:  7,
		RequestsHuman:  6,
		RequestsBot:    1,
		UniqueVisitors: 4,
	}

	if err := UpsertSiteTrafficHourly(siteID, oldEntry); err != nil {
		t.Fatalf("failed to seed old hourly entry: %v", err)
	}
	if err := UpsertSiteTrafficHourly(siteID, recentEntry); err != nil {
		t.Fatalf("failed to seed recent hourly entry: %v", err)
	}

	// Note: no manual RecomputeSiteTrafficDaily call here — the point of
	// this test is that PruneOldSiteTrafficHourly itself guarantees the
	// daily rollup is up to date before it deletes the source hourly row.
	if err := PruneOldSiteTrafficHourly(now.Add(-48 * time.Hour)); err != nil {
		t.Fatalf("PruneOldSiteTrafficHourly() error = %v", err)
	}

	var hourlyCount int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM site_traffic_hourly WHERE site_id = ?`, siteID).Scan(&hourlyCount); err != nil {
		t.Fatalf("failed to count hourly rows: %v", err)
	}
	if hourlyCount != 1 {
		t.Fatalf("expected 1 hourly row left (the recent one), got %d", hourlyCount)
	}

	var remainingHour string
	if err := GetDB().QueryRow(`SELECT hour FROM site_traffic_hourly WHERE site_id = ?`, siteID).Scan(&remainingHour); err != nil {
		t.Fatalf("failed to read remaining hourly row: %v", err)
	}
	if remainingHour != recentEntry.Hour {
		t.Errorf("remaining hourly row = %q, want %q", remainingHour, recentEntry.Hour)
	}

	daily, err := GetSiteTraffic(siteID, "daily", 3650)
	if err != nil {
		t.Fatalf("failed to read daily rollup: %v", err)
	}
	oldDay := oldHour.Format("2006-01-02")
	found := false
	for _, d := range daily {
		// PeriodStart's exact formatting (e.g. a bare date vs. a full
		// midnight timestamp) depends on how the sqlite driver round-trips
		// the DATE column; only the date portion is guaranteed here.
		if strings.HasPrefix(d.PeriodStart, oldDay) {
			found = true
			if d.RequestsTotal != 5 {
				t.Errorf("daily rollup RequestsTotal = %d, want 5 (must survive pruning of the source hourly row)", d.RequestsTotal)
			}
			if len(d.TopReferrers) != 1 || d.TopReferrers[0].Key != "https://google.com/" {
				t.Errorf("daily rollup TopReferrers = %+v, want the pruned hour's referrer preserved", d.TopReferrers)
			}
		}
	}
	if !found {
		t.Errorf("expected the pruned day (%s) to still have a site_traffic_daily rollup", oldDay)
	}
}

func TestPruneOldSiteTrafficHourly_KeepsRowsWithinRetention(t *testing.T) {
	setupTaskRepoTest(t)

	const siteID = 7
	recentHour := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Hour)
	entry := models.TrafficHourlyEntry{Hour: recentHour.Format(time.RFC3339), RequestsTotal: 1}

	if err := UpsertSiteTrafficHourly(siteID, entry); err != nil {
		t.Fatalf("failed to seed hourly entry: %v", err)
	}

	if err := PruneOldSiteTrafficHourly(time.Now().Add(-48 * time.Hour)); err != nil {
		t.Fatalf("PruneOldSiteTrafficHourly() error = %v", err)
	}

	var count int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM site_traffic_hourly WHERE site_id = ?`, siteID).Scan(&count); err != nil {
		t.Fatalf("failed to count hourly rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected the within-retention row to survive pruning, count = %d", count)
	}
}
