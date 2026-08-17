package db

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// RecordSiteDiskUsage inserts a new disk usage measurement for a site.
func RecordSiteDiskUsage(siteID int, bytesUsed int64, measuredAt string) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := dbConn.Exec(
		`INSERT INTO site_disk_usage (site_id, bytes_used, measured_at) VALUES (?, ?, ?)
		 ON CONFLICT(site_id, measured_at) DO UPDATE SET bytes_used = excluded.bytes_used`,
		siteID, bytesUsed, measuredAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record disk usage for site %d: %w", siteID, err)
	}
	return nil
}

// GetLatestSiteDiskUsage returns a map of site ID to its most recent disk
// usage measurement, for every site that has ever reported one.
func GetLatestSiteDiskUsage() (map[int]models.SiteDiskUsage, error) {
	dbConn := GetDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := dbConn.Query(`
		SELECT s.site_id, s.bytes_used, s.measured_at
		FROM site_disk_usage s
		WHERE s.measured_at = (
			SELECT MAX(s2.measured_at) FROM site_disk_usage s2 WHERE s2.site_id = s.site_id
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest site disk usage: %w", err)
	}
	defer rows.Close()

	result := make(map[int]models.SiteDiskUsage)
	for rows.Next() {
		var siteID int
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
