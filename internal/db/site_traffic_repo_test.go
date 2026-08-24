package db

import (
	"database/sql"
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
		StatusCodes:    []models.TrafficTopEntry{{Key: "200", Count: 4}, {Key: "404", Count: 1}},
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

	// Mirrors production's pruneSiteTraffic(): finalize completed days'
	// daily rollups (the old hour's day is already fully in the past) before
	// pruning touches their source hourly rows.
	if err := FinalizeCompletedDailyRollups(); err != nil {
		t.Fatalf("FinalizeCompletedDailyRollups() error = %v", err)
	}
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
			if len(d.StatusCodes) != 2 || d.StatusCodes[0].Key != "200" || d.StatusCodes[0].Count != 4 {
				t.Errorf("daily rollup StatusCodes = %+v, want the pruned hour's status codes preserved", d.StatusCodes)
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
		StatusCodes:    []models.TrafficTopEntry{{Key: "200", Count: 45}, {Key: "404", Count: 5}},
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
	if len(daily[0].StatusCodes) != 2 || daily[0].StatusCodes[0].Key != "200" || daily[0].StatusCodes[0].Count != 45 {
		t.Errorf("StatusCodes = %+v, want the seeded status codes preserved directly", daily[0].StatusCodes)
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

func TestGetSiteTrafficDailyRange(t *testing.T) {
	setupTaskRepoTest(t)

	seed := func(siteID int, day string, total int) {
		t.Helper()
		entry := models.TrafficDailyEntry{Day: day, RequestsTotal: total, RequestsHuman: total}
		if err := UpsertSiteTrafficDaily(siteID, entry); err != nil {
			t.Fatalf("failed to seed daily entry for site %d day %s: %v", siteID, day, err)
		}
	}

	seed(1, "2026-01-01", 10)
	seed(1, "2026-01-02", 20)
	seed(2, "2026-01-02", 30)
	seed(1, "2025-12-31", 99) // out of range, must be excluded

	rows, err := GetSiteTrafficDailyRange("2026-01-01", "2026-01-02")
	if err != nil {
		t.Fatalf("GetSiteTrafficDailyRange() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows in range, got %d: %+v", len(rows), rows)
	}

	// Ordered by site_id ASC, day ASC.
	if rows[0].SiteID != 1 || rows[0].Day != "2026-01-01" || rows[0].RequestsTotal != 10 {
		t.Errorf("rows[0] = %+v, want site 1, day 2026-01-01, total 10", rows[0])
	}
	if rows[1].SiteID != 1 || rows[1].Day != "2026-01-02" || rows[1].RequestsTotal != 20 {
		t.Errorf("rows[1] = %+v, want site 1, day 2026-01-02, total 20", rows[1])
	}
	if rows[2].SiteID != 2 || rows[2].Day != "2026-01-02" || rows[2].RequestsTotal != 30 {
		t.Errorf("rows[2] = %+v, want site 2, day 2026-01-02, total 30", rows[2])
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
		StatusCodes:  []models.TrafficTopEntry{{Key: "200", Count: 9}, {Key: "404", Count: 1}},
	})
	seed(dayB, models.TrafficDailyEntry{
		RequestsTotal: 5, RequestsHuman: 5, RequestsBot: 0, UniqueVisitors: 2,
		TopReferrers: []models.TrafficTopEntry{{Key: "https://a.com/", Count: 1}, {Key: "https://b.com/", Count: 2}},
		StatusCodes:  []models.TrafficTopEntry{{Key: "200", Count: 5}},
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
	if len(newer.StatusCodes) != 2 || newer.StatusCodes[0].Key != "200" || newer.StatusCodes[0].Count != 14 {
		t.Errorf("merged status codes = %+v, want 200 count=14 ranked first (merged across dayA+dayB)", newer.StatusCodes)
	}
	if len(older.StatusCodes) != 0 {
		t.Errorf("older month StatusCodes = %+v, want empty (dayC had none seeded)", older.StatusCodes)
	}
}

// TestGetSiteTraffic_HandlesRowsWithoutStatusCodesColumn simulates a row
// written before the status_codes column existed. migrateTable's
// recreate-and-copy migration omits any column not present in the old
// schema from its copy INSERT, so such a row relies on the column's
// DEFAULT ” (see internal/db/db.go's site_traffic_hourly/site_traffic_daily
// definitions) rather than getting NULL — without that default, scanning
// this column straight into a Go string would error.
func TestGetSiteTraffic_HandlesRowsWithoutStatusCodesColumn(t *testing.T) {
	setupTaskRepoTest(t)

	const siteID = 11
	hour := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Hour).Format(time.RFC3339)
	day := time.Now().UTC().Format("2006-01-02")

	// top_pages/top_referrers are populated here (as any genuinely
	// pre-existing row would already have them) — status_codes is the only
	// column omitted, since it's the one that's actually new.
	if _, err := GetDB().Exec(
		`INSERT INTO site_traffic_hourly (site_id, hour, requests_total, top_pages, top_referrers) VALUES (?, ?, ?, '[]', '[]')`,
		siteID, hour, 1,
	); err != nil {
		t.Fatalf("failed to seed hourly row without status_codes: %v", err)
	}
	if _, err := GetDB().Exec(
		`INSERT INTO site_traffic_daily (site_id, day, requests_total, top_pages, top_referrers) VALUES (?, date(?), ?, '[]', '[]')`,
		siteID, day, 1,
	); err != nil {
		t.Fatalf("failed to seed daily row without status_codes: %v", err)
	}

	hourly, err := GetSiteTraffic(siteID, "hourly", 7)
	if err != nil {
		t.Fatalf("GetSiteTraffic(hourly) error = %v", err)
	}
	if len(hourly) != 1 || len(hourly[0].StatusCodes) != 0 {
		t.Errorf("hourly = %+v, want 1 row with empty StatusCodes", hourly)
	}

	daily, err := GetSiteTraffic(siteID, "daily", 7)
	if err != nil {
		t.Fatalf("GetSiteTraffic(daily) error = %v", err)
	}
	if len(daily) != 1 || len(daily[0].StatusCodes) != 0 {
		t.Errorf("daily = %+v, want 1 row with empty StatusCodes", daily)
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

// TestFinalizeCompletedDailyRollups checks that only fully-elapsed days
// (strictly before today, UTC) get their site_traffic_daily rollup computed
// — today's in-progress day is left alone (it's kept live by the ingest-time
// recompute in AgentReportHandler instead), and finalizing a day never
// deletes its source hourly rows.
func TestFinalizeCompletedDailyRollups(t *testing.T) {
	setupTaskRepoTest(t)

	const siteID = 13
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1)

	seedHour := func(hour time.Time, total int) {
		entry := models.TrafficHourlyEntry{Hour: hour.Truncate(time.Hour).Format(time.RFC3339), RequestsTotal: total}
		if err := UpsertSiteTrafficHourly(siteID, entry); err != nil {
			t.Fatalf("failed to seed hourly entry for %s: %v", entry.Hour, err)
		}
	}
	seedHour(yesterday.Add(10*time.Hour), 10)
	seedHour(yesterday.Add(11*time.Hour), 20)
	seedHour(now, 99) // today — must not be finalized yet

	if err := FinalizeCompletedDailyRollups(); err != nil {
		t.Fatalf("FinalizeCompletedDailyRollups() error = %v", err)
	}

	daily, err := GetSiteTraffic(siteID, "daily", 3650)
	if err != nil {
		t.Fatalf("failed to read daily rollup: %v", err)
	}
	yesterdayStr := yesterday.Format("2006-01-02")
	todayStr := now.Format("2006-01-02")
	var gotYesterday, gotToday bool
	for _, d := range daily {
		if strings.HasPrefix(d.PeriodStart, yesterdayStr) {
			gotYesterday = true
			if d.RequestsTotal != 30 {
				t.Errorf("yesterday's daily total = %d, want 30 (10+20)", d.RequestsTotal)
			}
		}
		if strings.HasPrefix(d.PeriodStart, todayStr) {
			gotToday = true
		}
	}
	if !gotYesterday {
		t.Errorf("expected a finalized daily rollup for yesterday (%s)", yesterdayStr)
	}
	if gotToday {
		t.Errorf("today (%s) should not be finalized yet — it's still in progress", todayStr)
	}

	var hourlyCount int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM site_traffic_hourly WHERE site_id = ?`, siteID).Scan(&hourlyCount); err != nil {
		t.Fatalf("failed to count hourly rows: %v", err)
	}
	if hourlyCount != 3 {
		t.Errorf("FinalizeCompletedDailyRollups must not delete hourly rows, got %d remaining, want 3", hourlyCount)
	}

	var finalizedAt sql.NullString
	if err := GetDB().QueryRow(
		`SELECT finalized_at FROM site_traffic_daily WHERE site_id = ? AND day = ?`, siteID, yesterdayStr,
	).Scan(&finalizedAt); err != nil {
		t.Fatalf("failed to read finalized_at: %v", err)
	}
	if !finalizedAt.Valid {
		t.Fatalf("expected finalized_at to be set for yesterday after finalization")
	}

	// A late-arriving hourly row for the same day (e.g. AgentReportHandler's
	// ingest-time recompute picking up backlog) must not clear finalized_at —
	// otherwise PruneOldSiteTrafficHourly's finalized-only deletion check
	// (and re-finalization) would race against it again.
	seedHour(yesterday.Add(12*time.Hour), 5)
	if err := RecomputeSiteTrafficDaily(siteID, yesterdayStr); err != nil {
		t.Fatalf("RecomputeSiteTrafficDaily() error = %v", err)
	}
	var finalizedAtAfter sql.NullString
	if err := GetDB().QueryRow(
		`SELECT finalized_at FROM site_traffic_daily WHERE site_id = ? AND day = ?`, siteID, yesterdayStr,
	).Scan(&finalizedAtAfter); err != nil {
		t.Fatalf("failed to re-read finalized_at: %v", err)
	}
	if !finalizedAtAfter.Valid || finalizedAtAfter.String != finalizedAt.String {
		t.Errorf("RecomputeSiteTrafficDaily must preserve finalized_at, got %v, want unchanged %v", finalizedAtAfter, finalizedAt)
	}
}

// TestPruneOldSiteTrafficHourly_MultiTickDoesNotCorruptDailyTotal reproduces
// the scenario that used to corrupt daily rollups: a single calendar day's
// hourly rows being deleted incrementally across many scheduler ticks (the
// cutoff sliding forward one hour at a time) rather than all at once. The
// previous PruneOldSiteTrafficHourly recomputed the daily rollup from
// whatever hourly rows happened to still exist on every tick, so the stored
// total shrank a little each time a tick deleted a few more of the day's
// hours out from under the next recompute. With daily finalization decoupled
// from pruning, the total must stay correct throughout.
func TestPruneOldSiteTrafficHourly_MultiTickDoesNotCorruptDailyTotal(t *testing.T) {
	setupTaskRepoTest(t)

	const siteID = 21
	day := time.Now().UTC().AddDate(0, 0, -3).Truncate(24 * time.Hour)

	hours := []time.Time{
		day.Add(0 * time.Hour),
		day.Add(1 * time.Hour),
		day.Add(2 * time.Hour),
		day.Add(3 * time.Hour),
		day.Add(4 * time.Hour),
	}
	const perHourTotal = 10
	wantDailyTotal := perHourTotal * len(hours)

	for _, h := range hours {
		entry := models.TrafficHourlyEntry{Hour: h.Format(time.RFC3339), RequestsTotal: perHourTotal}
		if err := UpsertSiteTrafficHourly(siteID, entry); err != nil {
			t.Fatalf("failed to seed hourly entry for %s: %v", entry.Hour, err)
		}
	}

	// Simulate one scheduler tick per hourly row, each advancing the prune
	// cutoff just past one more of the day's hours — exactly how
	// pruneSiteTraffic's hourly ticker chips away at a day in production.
	for i, h := range hours {
		if err := FinalizeCompletedDailyRollups(); err != nil {
			t.Fatalf("tick %d: FinalizeCompletedDailyRollups() error = %v", i, err)
		}
		cutoff := h.Add(1 * time.Hour)
		if err := PruneOldSiteTrafficHourly(cutoff); err != nil {
			t.Fatalf("tick %d: PruneOldSiteTrafficHourly() error = %v", i, err)
		}

		daily, err := GetSiteTraffic(siteID, "daily", 3650)
		if err != nil {
			t.Fatalf("tick %d: failed to read daily rollup: %v", i, err)
		}
		dayStr := day.Format("2006-01-02")
		found := false
		for _, d := range daily {
			if strings.HasPrefix(d.PeriodStart, dayStr) {
				found = true
				if d.RequestsTotal != wantDailyTotal {
					t.Errorf("tick %d: daily total = %d, want %d (must not shrink as hourly rows are incrementally pruned)", i, d.RequestsTotal, wantDailyTotal)
				}
			}
		}
		if !found {
			t.Errorf("tick %d: expected a daily rollup for %s", i, dayStr)
		}
	}

	var hourlyCount int
	if err := GetDB().QueryRow(`SELECT COUNT(*) FROM site_traffic_hourly WHERE site_id = ?`, siteID).Scan(&hourlyCount); err != nil {
		t.Fatalf("failed to count hourly rows: %v", err)
	}
	if hourlyCount != 0 {
		t.Errorf("expected all of the day's hourly rows to be pruned by the final tick, got %d remaining", hourlyCount)
	}
}
