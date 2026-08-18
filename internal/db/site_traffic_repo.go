package db

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// topEntryLimit bounds how many ranked pages/referrers are kept per period,
// matching the cap jman-agent already applies before sending.
const topEntryLimit = 20

// UpsertSiteTrafficHourly stores one fully-elapsed hour's traffic for a
// site. jman-agent only ever sends a given hour once it's closed, so this
// is a plain replace-style upsert — no incremental merging needed.
func UpsertSiteTrafficHourly(siteID int, entry models.TrafficHourlyEntry) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	topPages, err := json.Marshal(entry.TopPages)
	if err != nil {
		return fmt.Errorf("failed to encode top pages: %w", err)
	}
	topReferrers, err := json.Marshal(entry.TopReferrers)
	if err != nil {
		return fmt.Errorf("failed to encode top referrers: %w", err)
	}

	query := `
	INSERT INTO site_traffic_hourly
		(site_id, hour, requests_total, requests_human, requests_bot, unique_visitors, top_pages, top_referrers, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id, hour) DO UPDATE SET
		requests_total = excluded.requests_total,
		requests_human = excluded.requests_human,
		requests_bot = excluded.requests_bot,
		unique_visitors = excluded.unique_visitors,
		top_pages = excluded.top_pages,
		top_referrers = excluded.top_referrers,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err = dbConn.Exec(query, siteID, entry.Hour, entry.RequestsTotal, entry.RequestsHuman, entry.RequestsBot, entry.UniqueVisitors, string(topPages), string(topReferrers))
	if err != nil {
		return fmt.Errorf("failed to upsert hourly traffic for site %d: %w", siteID, err)
	}
	return nil
}

// UpsertSiteTrafficDaily stores one already-aggregated day of traffic
// directly into site_traffic_daily. Used for backlog jman-agent aggregated
// client-side because it was older than its hourly retention window (see
// models.TrafficDailyEntry) — unlike RecomputeSiteTrafficDaily, this does
// NOT read from site_traffic_hourly; there is deliberately no hourly source
// data behind these days, so callers must not also add them to whatever
// day-recompute set they're tracking for hourly writes in the same report.
func UpsertSiteTrafficDaily(siteID int, entry models.TrafficDailyEntry) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	topPages, err := json.Marshal(entry.TopPages)
	if err != nil {
		return fmt.Errorf("failed to encode top pages: %w", err)
	}
	topReferrers, err := json.Marshal(entry.TopReferrers)
	if err != nil {
		return fmt.Errorf("failed to encode top referrers: %w", err)
	}

	query := `
	INSERT INTO site_traffic_daily
		(site_id, day, requests_total, requests_human, requests_bot, unique_visitors, top_pages, top_referrers, updated_at)
	VALUES (?, date(?), ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id, day) DO UPDATE SET
		requests_total = excluded.requests_total,
		requests_human = excluded.requests_human,
		requests_bot = excluded.requests_bot,
		unique_visitors = excluded.unique_visitors,
		top_pages = excluded.top_pages,
		top_referrers = excluded.top_referrers,
		updated_at = CURRENT_TIMESTAMP;
	`
	_, err = dbConn.Exec(query, siteID, entry.Day, entry.RequestsTotal, entry.RequestsHuman, entry.RequestsBot, entry.UniqueVisitors, string(topPages), string(topReferrers))
	if err != nil {
		return fmt.Errorf("failed to upsert daily traffic for site %d: %w", siteID, err)
	}
	return nil
}

// RecomputeSiteTrafficDaily rebuilds the daily rollup for a site/day from
// its hourly rows. Safe to call repeatedly (e.g. once per hourly write) —
// it's a pure re-aggregation, not an incremental update.
//
// unique_visitors is the sum of each hour's unique count, which
// over-counts a visitor active across multiple hours in the same day (true
// daily-distinct would require retaining raw IPs, which jman deliberately
// doesn't). top_pages/top_referrers are similarly derived by merging each
// hour's already-truncated top-N lists, not the full day's raw counts — an
// accepted approximation, not the exact daily top-N.
func RecomputeSiteTrafficDaily(siteID int, day string) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	rows, err := dbConn.Query(
		`SELECT requests_total, requests_human, requests_bot, unique_visitors, top_pages, top_referrers
		 FROM site_traffic_hourly WHERE site_id = ? AND date(hour) = date(?)`,
		siteID, day,
	)
	if err != nil {
		return fmt.Errorf("failed to load hourly traffic for site %d day %s: %w", siteID, day, err)
	}
	defer rows.Close()

	var total, human, bot, unique int
	pageCounts := map[string]int{}
	referrerCounts := map[string]int{}

	for rows.Next() {
		var t, h, b, u int
		var pagesJSON, referrersJSON string
		if err := rows.Scan(&t, &h, &b, &u, &pagesJSON, &referrersJSON); err != nil {
			return fmt.Errorf("failed to scan hourly traffic row: %w", err)
		}
		total += t
		human += h
		bot += b
		unique += u
		mergeTopEntries(pageCounts, pagesJSON)
		mergeTopEntries(referrerCounts, referrersJSON)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating hourly traffic: %w", err)
	}

	topPages, err := json.Marshal(topNFromCounts(pageCounts))
	if err != nil {
		return fmt.Errorf("failed to encode daily top pages: %w", err)
	}
	topReferrers, err := json.Marshal(topNFromCounts(referrerCounts))
	if err != nil {
		return fmt.Errorf("failed to encode daily top referrers: %w", err)
	}

	query := `
	INSERT INTO site_traffic_daily
		(site_id, day, requests_total, requests_human, requests_bot, unique_visitors, top_pages, top_referrers, updated_at)
	VALUES (?, date(?), ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id, day) DO UPDATE SET
		requests_total = excluded.requests_total,
		requests_human = excluded.requests_human,
		requests_bot = excluded.requests_bot,
		unique_visitors = excluded.unique_visitors,
		top_pages = excluded.top_pages,
		top_referrers = excluded.top_referrers,
		updated_at = CURRENT_TIMESTAMP;
	`
	if _, err := dbConn.Exec(query, siteID, day, total, human, bot, unique, string(topPages), string(topReferrers)); err != nil {
		return fmt.Errorf("failed to upsert daily traffic for site %d day %s: %w", siteID, day, err)
	}
	return nil
}

// PruneOldSiteTrafficHourly deletes site_traffic_hourly rows older than
// cutoff. Before deleting each affected site/day's rows, it recomputes that
// day's site_traffic_daily rollup so the daily rollup is guaranteed to
// reflect every hourly row currently on disk (including any late-arriving
// backlog hour) before the source data is removed — daily rollups are cheap
// to keep indefinitely, so hourly detail is the only thing pruned.
func PruneOldSiteTrafficHourly(cutoff time.Time) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	cutoffStr := cutoff.UTC().Format(time.RFC3339)

	rows, err := dbConn.Query(
		`SELECT DISTINCT site_id, date(hour) FROM site_traffic_hourly WHERE hour < ?`,
		cutoffStr,
	)
	if err != nil {
		return fmt.Errorf("failed to find stale hourly traffic: %w", err)
	}
	type sitedDay struct {
		siteID int
		day    string
	}
	var stale []sitedDay
	for rows.Next() {
		var s sitedDay
		if err := rows.Scan(&s.siteID, &s.day); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan stale hourly traffic row: %w", err)
		}
		stale = append(stale, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("error iterating stale hourly traffic: %w", err)
	}
	rows.Close()

	for _, s := range stale {
		if err := RecomputeSiteTrafficDaily(s.siteID, s.day); err != nil {
			return fmt.Errorf("failed to recompute daily rollup for site %d day %s before pruning: %w", s.siteID, s.day, err)
		}
		if _, err := dbConn.Exec(
			`DELETE FROM site_traffic_hourly WHERE site_id = ? AND date(hour) = ? AND hour < ?`,
			s.siteID, s.day, cutoffStr,
		); err != nil {
			return fmt.Errorf("failed to prune hourly traffic for site %d day %s: %w", s.siteID, s.day, err)
		}
	}
	return nil
}

func mergeTopEntries(into map[string]int, entriesJSON string) {
	if entriesJSON == "" {
		return
	}
	var entries []models.TrafficTopEntry
	if err := json.Unmarshal([]byte(entriesJSON), &entries); err != nil {
		return
	}
	for _, e := range entries {
		into[e.Key] += e.Count
	}
}

func topNFromCounts(counts map[string]int) []models.TrafficTopEntry {
	entries := make([]models.TrafficTopEntry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, models.TrafficTopEntry{Key: key, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Key < entries[j].Key
	})
	if len(entries) > topEntryLimit {
		entries = entries[:topEntryLimit]
	}
	return entries
}

// GetSiteTraffic returns a site's hourly or daily traffic for the last
// `days` days, oldest first.
func GetSiteTraffic(siteID int, period string, days int) ([]models.SiteTrafficPeriod, error) {
	dbConn := GetDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	table := "site_traffic_hourly"
	periodCol := "hour"
	// hour is stored as RFC3339 ("...T...Z", matching Go's time.RFC3339
	// formatting on the agent side) — strftime with an explicit format
	// produces a directly comparable string. SQLite's own datetime()
	// defaults to a space-separated, non-'Z'-suffixed format instead, which
	// would compare incorrectly against RFC3339 values via a plain TEXT >=.
	cutoffExpr := "strftime('%Y-%m-%dT%H:%M:%SZ', 'now', ?)"
	if period == "daily" {
		table = "site_traffic_daily"
		periodCol = "day"
		cutoffExpr = "date('now', ?)"
	}

	query := fmt.Sprintf(
		`SELECT %s, requests_total, requests_human, requests_bot, unique_visitors, top_pages, top_referrers
		 FROM %s WHERE site_id = ? AND %s >= %s ORDER BY %s ASC`,
		periodCol, table, periodCol, cutoffExpr, periodCol,
	)

	rows, err := dbConn.Query(query, siteID, fmt.Sprintf("-%d days", days))
	if err != nil {
		return nil, fmt.Errorf("failed to query site traffic: %w", err)
	}
	defer rows.Close()

	result := []models.SiteTrafficPeriod{}
	for rows.Next() {
		var p models.SiteTrafficPeriod
		var pagesJSON, referrersJSON string
		if err := rows.Scan(&p.PeriodStart, &p.RequestsTotal, &p.RequestsHuman, &p.RequestsBot, &p.UniqueVisitors, &pagesJSON, &referrersJSON); err != nil {
			return nil, fmt.Errorf("failed to scan site traffic row: %w", err)
		}
		_ = json.Unmarshal([]byte(pagesJSON), &p.TopPages)
		_ = json.Unmarshal([]byte(referrersJSON), &p.TopReferrers)
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site traffic: %w", err)
	}

	return result, nil
}
