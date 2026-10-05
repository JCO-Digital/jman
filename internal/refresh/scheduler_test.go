package refresh

import (
	"testing"
	"time"
)

func TestSiteReportDue(t *testing.T) {
	loc := time.FixedZone("EEST", 3*60*60)
	at := func(h, m int) time.Time { return time.Date(2026, 10, 5, h, m, 0, 0, loc) }

	tests := []struct {
		name       string
		now        time.Time
		lastDate   string
		reportTime string
		want       bool
	}{
		{"before report time", at(9, 59), "2026-10-04", "10:00", false},
		{"at report time", at(10, 0), "2026-10-04", "10:00", true},
		{"after report time, never sent", at(14, 30), "", "10:00", true},
		{"already sent today", at(10, 15), "2026-10-05", "10:00", false},
		{"custom time not reached", at(10, 15), "", "11:30", false},
		{"invalid time falls back to 10:00", at(10, 15), "", "bogus", true},
		{"invalid time before fallback", at(9, 0), "", "bogus", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := siteReportDue(tt.now, tt.lastDate, tt.reportTime); got != tt.want {
				t.Errorf("siteReportDue() = %v, want %v", got, tt.want)
			}
		})
	}
}
