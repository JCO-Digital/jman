package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/verb"
)

const timestampMigrationName = "normalize_api_timestamps"

// canonicalTimeFormat is how APIDB stores time.Time values: the driver's
// `_time_format=sqlite` layout, always applied to UTC times.
const canonicalTimeFormat = "2006-01-02 15:04:05.999999999-07:00"

// legacyTimeFormats are the encodings found in api.db before APIDB
// existed, mirroring what modernc.org/sqlite itself accepts when reading
// a DATETIME column (time.String() output is handled separately).
var legacyTimeFormats = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// parseStoredTime parses a DATETIME value as stored by any earlier version
// of jman: Go's time.String() output (with zone abbreviation and optional
// monotonic suffix, e.g. "2026-10-02 08:54:39.4 +0300 EEST m=+156.95"),
// RFC3339 strings sent by jman-agent, or SQLite's CURRENT_TIMESTAMP text.
// Values without an offset are UTC, as in the driver.
func parseStoredTime(s string) (time.Time, bool) {
	if i := strings.Index(s, " m="); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", s); err == nil {
		return t, true
	}
	s = strings.TrimSuffix(s, "Z")
	for _, f := range legacyTimeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// isCurrentTimestampText reports whether s is SQLite's own CURRENT_TIMESTAMP
// form ("YYYY-MM-DD HH:MM:SS", UTC). Column defaults keep producing it, and
// it already sorts consistently with the canonical format, so it is left as
// is rather than rewritten.
func isCurrentTimestampText(s string) bool {
	if len(s) != 19 || s[10] != ' ' {
		return false
	}
	_, err := time.Parse("2006-01-02 15:04:05", s)
	return err == nil
}

// NormalizeAPITimestamps rewrites every DATETIME value in the given api.db
// tables into the canonical UTC form APIDB writes, and lowercases the
// domain-keyed monitor/incident columns. Before this, values mixed
// time.String() output in local and UTC offsets with RFC3339 strings, so
// text comparisons on those columns (billing cutoffs, traffic and disk
// retention) were off by the UTC offset or compared 'T' against ' '.
// Runs once per database.
func NormalizeAPITimestamps(dbConn *sql.DB, tables []TableDefinition) error {
	return runOnce(dbConn, timestampMigrationName, func(tx *sql.Tx) error {
		for _, def := range tables {
			for col, typ := range def.Columns {
				if !strings.HasPrefix(strings.ToUpper(typ), "DATETIME") {
					continue
				}
				if err := normalizeTimestampColumn(tx, def.Name, col); err != nil {
					return err
				}
			}
		}
		for _, table := range []string{"monitor_status", "monitor_history", "incidents"} {
			// COLLATE BINARY: these columns are NOCASE, under which every
			// value already "equals" its lowercase form.
			if _, err := tx.Exec(fmt.Sprintf(
				`UPDATE %s SET domain = lower(domain) WHERE domain COLLATE BINARY <> lower(domain)`, table,
			)); err != nil {
				return fmt.Errorf("failed to lowercase %s.domain: %w", table, err)
			}
		}
		return nil
	})
}

func normalizeTimestampColumn(tx *sql.Tx, table, col string) error {
	if !config.IsSafeIdentifier(table) || !config.IsSafeIdentifier(col) {
		return fmt.Errorf("invalid identifier %s.%s", table, col)
	}

	// CAST to TEXT so the driver hands back the stored text instead of
	// parsing it into a time.Time.
	rows, err := tx.Query(fmt.Sprintf(
		`SELECT rowid, CAST(%s AS TEXT) FROM %s WHERE %s IS NOT NULL`, col, table, col,
	))
	if err != nil {
		return fmt.Errorf("failed to read %s.%s: %w", table, col, err)
	}
	type change struct {
		rowid int64
		value string
	}
	var changes []change
	unparsed := 0
	for rows.Next() {
		var rowid int64
		var raw string
		if err := rows.Scan(&rowid, &raw); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan %s.%s: %w", table, col, err)
		}
		if raw == "" || isCurrentTimestampText(raw) {
			continue
		}
		t, ok := parseStoredTime(raw)
		if !ok {
			unparsed++
			continue
		}
		if canonical := t.UTC().Format(canonicalTimeFormat); canonical != raw {
			changes = append(changes, change{rowid, canonical})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("failed to iterate %s.%s: %w", table, col, err)
	}
	rows.Close()

	if unparsed > 0 {
		verb.LogPrintf(verb.Normal, "%s: left %d unrecognized value(s) in %s.%s unchanged", timestampMigrationName, unparsed, table, col)
	}
	if len(changes) == 0 {
		return nil
	}

	// OR IGNORE plus delete: when the column is part of a composite key
	// (site_disk_usage.measured_at, site_traffic_hourly.hour), two rows can
	// hold the same instant in different encodings. The canonical one wins
	// and the duplicate is dropped.
	update, err := tx.Prepare(fmt.Sprintf(`UPDATE OR IGNORE %s SET %s = ? WHERE rowid = ?`, table, col))
	if err != nil {
		return err
	}
	defer update.Close()
	remove, err := tx.Prepare(fmt.Sprintf(`DELETE FROM %s WHERE rowid = ?`, table))
	if err != nil {
		return err
	}
	defer remove.Close()

	duplicates := 0
	for _, c := range changes {
		res, err := update.Exec(c.value, c.rowid)
		if err != nil {
			return fmt.Errorf("failed to update %s.%s: %w", table, col, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if _, err := remove.Exec(c.rowid); err != nil {
				return fmt.Errorf("failed to drop duplicate %s row: %w", table, err)
			}
			duplicates++
		}
	}
	if duplicates > 0 {
		verb.LogPrintf(verb.Normal, "%s: dropped %d duplicate row(s) from %s", timestampMigrationName, duplicates, table)
	}
	return nil
}
