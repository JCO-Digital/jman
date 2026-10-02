package db

import (
	"context"
	"database/sql"
	"time"
)

// APIDB is jman-api's database handle. It embeds *sql.DB and intercepts
// the query methods so every query argument is written in one canonical
// form: time.Time values (direct, pointer, or sql.NullTime) are converted
// to UTC before reaching the driver. Combined with the `_time_format=sqlite`
// DSN option this stores every timestamp as
// "YYYY-MM-DD HH:MM:SS[.fff]+00:00", so text comparisons and ORDER BY on
// DATETIME columns follow chronological order. Before, the driver's
// default time.String() encoding mixed local and UTC offsets.
//
// This is also the single seam where a future PostgreSQL backend would
// rebind `?` placeholders (see docs/postgres-migration.md).
//
// Transactions from Begin/BeginTx and connections from Conn are not
// wrapped; callers using them must pass UTC times themselves.
type APIDB struct {
	*sql.DB
}

// sqlDB returns the underlying handle, tolerating a nil *APIDB so callers
// can keep their "database not initialized" checks.
func (d *APIDB) sqlDB() *sql.DB {
	if d == nil {
		return nil
	}
	return d.DB
}

func (d *APIDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(query, normalizeArgs(args)...)
}

func (d *APIDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.DB.ExecContext(ctx, query, normalizeArgs(args)...)
}

func (d *APIDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(query, normalizeArgs(args)...)
}

func (d *APIDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.DB.QueryContext(ctx, query, normalizeArgs(args)...)
}

func (d *APIDB) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(query, normalizeArgs(args)...)
}

func (d *APIDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.DB.QueryRowContext(ctx, query, normalizeArgs(args)...)
}

// normalizeArgs returns args with every time value converted to UTC. The
// input slice is copied only when something actually changes.
func normalizeArgs(args []any) []any {
	var out []any
	for i, a := range args {
		n, changed := normalizeArg(a)
		if !changed {
			continue
		}
		if out == nil {
			out = make([]any, len(args))
			copy(out, args)
		}
		out[i] = n
	}
	if out == nil {
		return args
	}
	return out
}

func normalizeArg(a any) (any, bool) {
	switch v := a.(type) {
	case time.Time:
		return v.UTC(), true
	case *time.Time:
		if v == nil {
			return a, false
		}
		u := v.UTC()
		return &u, true
	case sql.NullTime:
		if !v.Valid {
			return a, false
		}
		return sql.NullTime{Time: v.Time.UTC(), Valid: true}, true
	case *sql.NullTime:
		if v == nil || !v.Valid {
			return a, false
		}
		return sql.NullTime{Time: v.Time.UTC(), Valid: true}, true
	}
	return a, false
}
