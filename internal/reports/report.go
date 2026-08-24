// Package reports defines a small framework for backend-defined, read-only
// tabular reports: each report accepts query parameters and returns column
// definitions plus row data as JSON, which the frontend renders as a table
// and can export to CSV client-side.
package reports

import (
	"fmt"
	"net/url"
	"sort"
	"time"
)

// ColumnType hints the frontend how to render/format a column's values.
type ColumnType string

const (
	ColumnText     ColumnType = "text"
	ColumnNumber   ColumnType = "number"
	ColumnCurrency ColumnType = "currency" // integer cents
	ColumnDate     ColumnType = "date"
)

// Column describes one column of tabular report output.
type Column struct {
	Key   string     `json:"key"`
	Label string     `json:"label"`
	Type  ColumnType `json:"type"`
}

// ParamType enumerates supported input kinds. New kinds can be added here as
// reports need them, without changing the Report interface or the HTTP layer.
type ParamType string

const (
	ParamDateRange ParamType = "daterange"
	// ParamEndDate is a single date input with no lower bound — used by
	// reports that want to show everything up to a cutoff, including
	// anything already overdue (e.g. upcoming billing).
	ParamEndDate ParamType = "enddate"
)

// ParamDef describes one input field a report accepts, so the frontend can
// render a generic form without hardcoding per-report knowledge.
type ParamDef struct {
	Key      string    `json:"key"`
	Type     ParamType `json:"type"`
	Label    string    `json:"label"`
	Required bool      `json:"required"`
	Default  string    `json:"default,omitempty"`
}

// Result is the JSON body returned by Run.
type Result struct {
	Columns []Column         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// Report is implemented by every concrete report. Each report owns its own
// parameter parsing/validation, keeping the registry and HTTP layer generic.
type Report interface {
	ID() string
	Name() string
	Description() string
	Params() []ParamDef
	Run(q url.Values) (*Result, error)
}

// Meta is the JSON-facing description of a report, without its Run logic.
type Meta struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Params      []ParamDef `json:"params"`
}

var registry = map[string]Report{}

// Register adds a report to the registry. Intended to be called from an
// init() in the file defining the report.
func Register(r Report) {
	registry[r.ID()] = r
}

// All returns every registered report, sorted by ID for deterministic output.
func All() []Report {
	out := make([]Report, 0, len(registry))
	for _, r := range registry {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// AllMeta returns Meta for every registered report, sorted by ID.
func AllMeta() []Meta {
	all := All()
	out := make([]Meta, 0, len(all))
	for _, r := range all {
		out = append(out, Meta{ID: r.ID(), Name: r.Name(), Description: r.Description(), Params: r.Params()})
	}
	return out
}

// Get looks up a registered report by ID.
func Get(id string) (Report, bool) {
	r, ok := registry[id]
	return r, ok
}

// defaultDateRangeDays is how far back a report looks when no start date is
// given.
const defaultDateRangeDays = 30

// dateLayout is the wire format for date query params and report output.
const dateLayout = "2006-01-02"

// ParseDateRange parses ?start=YYYY-MM-DD&end=YYYY-MM-DD, defaulting end to
// today and start to defaultDateRangeDays before end when absent, and caps
// the span at maxRangeDays.
func ParseDateRange(q url.Values, maxRangeDays int) (start, end string, err error) {
	startStr := q.Get("start")
	endStr := q.Get("end")

	var startT, endT time.Time
	now := time.Now().UTC()

	if endStr == "" {
		endT = now
	} else {
		endT, err = time.Parse(dateLayout, endStr)
		if err != nil {
			return "", "", fmt.Errorf("invalid end date %q: must be YYYY-MM-DD", endStr)
		}
	}

	if startStr == "" {
		startT = endT.AddDate(0, 0, -defaultDateRangeDays)
	} else {
		startT, err = time.Parse(dateLayout, startStr)
		if err != nil {
			return "", "", fmt.Errorf("invalid start date %q: must be YYYY-MM-DD", startStr)
		}
	}

	if startT.After(endT) {
		return "", "", fmt.Errorf("start date must not be after end date")
	}
	if maxRangeDays > 0 && endT.Sub(startT) > time.Duration(maxRangeDays)*24*time.Hour {
		return "", "", fmt.Errorf("date range too large: max %d days", maxRangeDays)
	}

	return startT.Format(dateLayout), endT.Format(dateLayout), nil
}

// ParseEndDate parses a single ?end=YYYY-MM-DD query param, defaulting to
// defaultEnd when absent.
func ParseEndDate(q url.Values, defaultEnd time.Time) (string, error) {
	raw := q.Get("end")
	if raw == "" {
		return defaultEnd.Format(dateLayout), nil
	}
	t, err := time.Parse(dateLayout, raw)
	if err != nil {
		return "", fmt.Errorf("invalid end date %q: must be YYYY-MM-DD", raw)
	}
	return t.Format(dateLayout), nil
}
