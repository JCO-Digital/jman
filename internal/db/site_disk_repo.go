package db

import (
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// RecordSiteDiskUsage inserts a new disk usage measurement for a site (by UUID).
func RecordSiteDiskUsage(siteID string, bytesUsed int64, measuredAt time.Time) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := dbConn.Exec(
		`INSERT INTO site_disk_usage (site_id, bytes_used, measured_at) VALUES (?, ?, ?)
		 ON CONFLICT(site_id, measured_at) DO UPDATE SET bytes_used = excluded.bytes_used`,
		siteID, bytesUsed, measuredAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record disk usage for site %s: %w", siteID, err)
	}
	return nil
}

// GetLatestSiteDiskUsage returns a map of site UUID to its most recent disk
// usage measurement, for every site that has ever reported one.
func GetLatestSiteDiskUsage() (map[string]models.SiteDiskUsage, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// The table holds every measurement ever reported (millions of rows), so
	// rather than scanning it, walk the distinct site_ids via a recursive
	// skip-scan over the site_id index and look up each site's latest row
	// through the (site_id, measured_at) primary key. This touches roughly
	// two index probes per site instead of every row in the table.
	rows, err := dbConn.Query(`
		WITH RECURSIVE ids(site_id) AS (
			SELECT MIN(site_id) FROM site_disk_usage
			UNION ALL
			SELECT (SELECT MIN(site_id) FROM site_disk_usage WHERE site_id > ids.site_id)
			FROM ids WHERE ids.site_id IS NOT NULL
		)
		SELECT d.site_id, d.bytes_used, d.measured_at
		FROM ids
		JOIN site_disk_usage d ON d.site_id = ids.site_id
		 AND d.measured_at = (SELECT MAX(measured_at) FROM site_disk_usage WHERE site_id = ids.site_id)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest site disk usage: %w", err)
	}
	defer rows.Close()

	result := make(map[string]models.SiteDiskUsage)
	for rows.Next() {
		var siteID string
		var usage models.SiteDiskUsage
		if err := rows.Scan(&siteID, &usage.BytesUsed, &usage.MeasuredAt); err != nil {
			return nil, fmt.Errorf("failed to scan site disk usage: %w", err)
		}
		result[siteID] = usage
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site disk usage: %w", err)
	}

	return result, nil
}

// DownsampleOldSiteDiskUsage thins out disk usage measurements older than
// cutoff to one per site per UTC day (the day's latest), leaving everything
// newer at full resolution. Agents report every few minutes, so without this
// the table grows by tens of thousands of rows a day. Work is done one site
// at a time so each DELETE holds the write lock only briefly.
func DownsampleOldSiteDiskUsage(cutoff time.Time) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	latest, err := GetLatestSiteDiskUsage()
	if err != nil {
		return err
	}

	for siteID := range latest {
		// measured_at is stored as canonical UTC text (see APIDB), so its
		// first ten characters are the UTC day. SQLite returns the bare
		// rowid from the row holding MAX(measured_at) within each group.
		if _, err := dbConn.Exec(
			`DELETE FROM site_disk_usage
			 WHERE site_id = ? AND measured_at < ?
			 AND rowid NOT IN (
			 	SELECT keep_id FROM (
			 		SELECT rowid AS keep_id, MAX(measured_at)
			 		FROM site_disk_usage
			 		WHERE site_id = ? AND measured_at < ?
			 		GROUP BY substr(measured_at, 1, 10)
			 	)
			 )`,
			siteID, cutoff, siteID, cutoff,
		); err != nil {
			return fmt.Errorf("failed to downsample disk usage for site %s: %w", siteID, err)
		}
	}
	return nil
}
