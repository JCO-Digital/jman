package tasks

import (
	"testing"
	"time"
)

func TestDecideStaleAgentAction(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		lastSeen      time.Time
		lastAlertedAt *time.Time
		want          staleAgentAction
	}{
		{
			name:     "fresh, never alerted",
			lastSeen: now.Add(-5 * time.Minute),
			want:     staleAgentActionNone,
		},
		{
			name:     "just crossed the stale threshold, never alerted",
			lastSeen: now.Add(-agentStaleThreshold - time.Minute),
			want:     staleAgentActionAlert,
		},
		{
			name:          "still stale but alerted recently — no repeat yet",
			lastSeen:      now.Add(-agentStaleThreshold - time.Hour),
			lastAlertedAt: timePtr(now.Add(-time.Hour)),
			want:          staleAgentActionNone,
		},
		{
			name:          "still stale and last alert is past the repeat interval",
			lastSeen:      now.Add(-agentStaleThreshold - 48*time.Hour),
			lastAlertedAt: timePtr(now.Add(-agentStaleRepeatInterval - time.Minute)),
			want:          staleAgentActionAlert,
		},
		{
			name:          "recovered after a prior alert",
			lastSeen:      now.Add(-time.Minute),
			lastAlertedAt: timePtr(now.Add(-2 * time.Hour)),
			want:          staleAgentActionRecovered,
		},
		{
			name:     "fresh and never alerted — recovery must not fire spuriously",
			lastSeen: now.Add(-time.Minute),
			want:     staleAgentActionNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decideStaleAgentAction(now, tc.lastSeen, tc.lastAlertedAt)
			if got != tc.want {
				t.Errorf("decideStaleAgentAction() = %v, want %v", got, tc.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestParseSQLiteTimestamp(t *testing.T) {
	want := time.Date(2026, 9, 28, 13, 26, 28, 0, time.UTC)
	for _, raw := range []string{
		"2026-09-28 13:26:28",  // CURRENT_TIMESTAMP text as stored
		"2026-09-28T13:26:28Z", // what the driver returns for DATETIME columns
		"2026-09-28T16:26:28+03:00",
	} {
		got, err := parseSQLiteTimestamp(raw)
		if err != nil {
			t.Errorf("parseSQLiteTimestamp(%q) error = %v", raw, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("parseSQLiteTimestamp(%q) = %v, want %v", raw, got, want)
		}
	}
	if _, err := parseSQLiteTimestamp("not a time"); err == nil {
		t.Error("parseSQLiteTimestamp should reject garbage")
	}
}
