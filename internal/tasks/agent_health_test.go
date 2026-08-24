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
