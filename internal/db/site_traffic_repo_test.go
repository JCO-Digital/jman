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

func TestUpsertSiteTrafficDaily(t *testing.T) {
	setupTaskRepoTest(t)

	const siteID = 99
	entry := models.TrafficDailyEntry{
		Day:            "2026-01-15",
		RequestsTotal:  50,
		RequestsHuman:  40,
		RequestsBot:    10,
		UniqueVisitors: 12,
		TopPages:       []models.TrafficTopEntry{{Key: "/", Count: 30}},
		TopReferrers:   []models.TrafficTopEntry{{Key: "https://example.org/", Count: 5}},
	}

	if err := UpsertSiteTrafficDaily(siteID, entry); err != nil {
		t.Fatalf("UpsertSiteTrafficDaily() error = %v", err)
	}

	daily, err := GetSiteTraffic(siteID, "daily", 3650)
	if err != nil {
		t.Fatalf("failed to read daily rollup: %v", err)
	}
	if len(daily) != 1 {
		t.Fatalf("expected exactly 1 daily row, got %d", len(daily))
	}
	if daily[0].RequestsTotal != 50 || daily[0].RequestsHuman != 40 || daily[0].RequestsBot != 10 {
		t.Errorf("daily row counts = %+v, want total=50 human=40 bot=10", daily[0])
	}
	if len(daily[0].TopReferrers) != 1 || daily[0].TopReferrers[0].Key != "https://example.org/" {
		t.Errorf("TopReferrers = %+v, want the seeded referrer preserved directly (no hourly source data)", daily[0].TopReferrers)
	}

	// A second upsert for the same day must replace, not add to, the row.
	entry.RequestsTotal = 99
	if err := UpsertSiteTrafficDaily(siteID, entry); err != nil {
		t.Fatalf("UpsertSiteTrafficDaily() second call error = %v", err)
	}
	var count int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM site_traffic_daily WHERE site_id = ?`, siteID).Scan(&count); err != nil {
		t.Fatalf("failed to count daily rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected re-upserting the same day to replace the row, not add another, count = %d", count)
	}
}

func TestGetSiteTrafficMonthly(t *testing.T) {
	setupTaskRepoTest(t)

	const siteID = 5
	now := time.Now().UTC()
	// Anchor to whole-month boundaries (rather than n-days-ago offsets from
	// `now`) so this test's month groupings are correct regardless of which
	// day of the month it happens to run on.
	prevMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	dayA := prevMonthStart.AddDate(0, 0, 10)  // previous month
	dayB := prevMonthStart.AddDate(0, 0, 11)  // same month as dayA
	dayC := prevMonthStart.AddDate(0, -1, 10) // the month before that

	seed := func(day time.Time, entry models.TrafficDailyEntry) {
		t.Helper()
		entry.Day = day.Format("2006-01-02")
		if err := UpsertSiteTrafficDaily(siteID, entry); err != nil {
			t.Fatalf("failed to seed daily entry for %s: %v", entry.Day, err)
		}
	}
	seed(dayA, models.TrafficDailyEntry{
		RequestsTotal: 10, RequestsHuman: 8, RequestsBot: 2, UniqueVisitors: 4,
		TopReferrers: []models.TrafficTopEntry{{Key: "https://a.com/", Count: 3}},
	})
	seed(dayB, models.TrafficDailyEntry{
		RequestsTotal: 5, RequestsHuman: 5, RequestsBot: 0, UniqueVisitors: 2,
		TopReferrers: []models.TrafficTopEntry{{Key: "https://a.com/", Count: 1}, {Key: "https://b.com/", Count: 2}},
	})
	seed(dayC, models.TrafficDailyEntry{RequestsTotal: 20, RequestsHuman: 15, RequestsBot: 5, UniqueVisitors: 6})

	months, err := GetSiteTrafficMonthly(siteID, 100)
	if err != nil {
		t.Fatalf("GetSiteTrafficMonthly() error = %v", err)
	}
	if len(months) != 2 {
		t.Fatalf("expected 2 monthly rows (dayA/dayB share a month), got %d: %+v", len(months), months)
	}

	older, newer := months[0], months[1]
	if want := dayC.Format("2006-01"); older.PeriodStart != want {
		t.Errorf("months[0].PeriodStart = %q, want %q", older.PeriodStart, want)
	}
	if want := dayA.Format("2006-01"); newer.PeriodStart != want {
		t.Errorf("months[1].PeriodStart = %q, want %q", newer.PeriodStart, want)
	}
	if older.RequestsTotal != 20 {
		t.Errorf("older month RequestsTotal = %d, want 20", older.RequestsTotal)
	}
	if newer.RequestsTotal != 15 || newer.RequestsHuman != 13 || newer.RequestsBot != 2 || newer.UniqueVisitors != 6 {
		t.Errorf("newer month aggregate = %+v, want total=15 human=13 bot=2 unique=6 (summed across dayA+dayB)", newer)
	}
	if len(newer.TopReferrers) != 2 || newer.TopReferrers[0].Key != "https://a.com/" || newer.TopReferrers[0].Count != 4 {
		t.Errorf("merged top referrers = %+v, want https://a.com/ count=4 ranked first (merged across dayA+dayB)", newer.TopReferrers)
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
