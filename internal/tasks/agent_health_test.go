package tasks

import (
	"testing"
	"time"
)

func TestDecideStaleAgentAction(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	agentStaleThreshold := 30 * time.Minute

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
			got := decideStaleAgentAction(now, tc.lastSeen, tc.lastAlertedAt, agentStaleThreshold)
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

func TestStaleSitesMessage(t *testing.T) {
	if got := staleSitesMessage(nil, nil); got != "" {
		t.Errorf("no changes: %q", got)
	}
	got := staleSitesMessage(
		[]staleSiteChange{
			{domain: "a.example.com", detail: "last collected 3h ago: refusing to run wp-cli: owned by root"},
			{domain: "b.example.com", detail: "last collected 4h ago"},
		},
		[]staleSiteChange{{domain: "c.example.com"}},
	)
	want := "⚠️ jman-agent hasn't collected WordPress data (plugins, core) for 2 site(s):\n" +
		"• a.example.com (last collected 3h ago: refusing to run wp-cli: owned by root)\n" +
		"• b.example.com (last collected 4h ago)\n" +
		"✅ jman-agent is collecting WordPress data again for: c.example.com"
	if got != want {
		t.Errorf("message:\n%s\nwant:\n%s", got, want)
	}
}

func TestRoundedDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		45 * time.Minute:             "45m",
		3*time.Hour + 20*time.Minute: "3h",
	} {
		if got := roundedDuration(d); got != want {
			t.Errorf("roundedDuration(%s) = %q, want %q", d, got, want)
		}
	}
}
