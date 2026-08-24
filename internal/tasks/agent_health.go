package tasks

import (
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// agentStaleThreshold is how long an agent token can go without an
// authenticated request (last_seen_at, touched by AgentAuthMiddleware on
// every manifest/report request — internal/api/agent_auth.go) before it's
// considered stale and alerted on. Generous relative to the agent's default
// 15-minute report interval (and its own send retries/backoff) to avoid
// false positives from transient network blips, while still catching a
// stuck agent same-day instead of days later — silent staleness like this is
// exactly how the site-traffic rollup corruption went unnoticed for days.
const agentStaleThreshold = 3 * time.Hour

// agentStaleRepeatInterval bounds how often a still-stale agent is
// re-alerted, so an ongoing outage produces periodic reminders instead of
// either silence or a message on every hourly tick.
const agentStaleRepeatInterval = 24 * time.Hour

// sqliteTimestampFormat matches what SQLite's CURRENT_TIMESTAMP produces
// (used for last_seen_at and stale_alert_sent_at) — not RFC3339. Parsing it
// with the wrong format is exactly the mismatch this codebase already hit
// once, fixed in e4c8a90 for site_traffic_hourly's cutoff comparison.
const sqliteTimestampFormat = "2006-01-02 15:04:05"

func parseSQLiteTimestamp(s string) (time.Time, error) {
	return time.ParseInLocation(sqliteTimestampFormat, s, time.UTC)
}

// staleAgentAction is the outcome of comparing an agent token's last-seen
// and last-alerted timestamps against now — kept separate from
// checkStaleAgents' Slack/DB side effects so the throttling rules
// (threshold, repeat interval, recovery) can be tested directly.
type staleAgentAction int

const (
	staleAgentActionNone staleAgentAction = iota
	staleAgentActionAlert
	staleAgentActionRecovered
)

// decideStaleAgentAction applies agentStaleThreshold/agentStaleRepeatInterval:
// alert the first time a token crosses the stale threshold, alert again
// every agentStaleRepeatInterval while it remains stale, and report a
// recovery exactly once when a previously-alerted token is no longer stale.
func decideStaleAgentAction(now, lastSeen time.Time, lastAlertedAt *time.Time) staleAgentAction {
	isStale := now.Sub(lastSeen) >= agentStaleThreshold
	switch {
	case isStale && (lastAlertedAt == nil || now.Sub(*lastAlertedAt) >= agentStaleRepeatInterval):
		return staleAgentActionAlert
	case !isStale && lastAlertedAt != nil:
		return staleAgentActionRecovered
	default:
		return staleAgentActionNone
	}
}

// checkStaleAgents alerts via Slack when a server's jman-agent stops making
// authenticated requests entirely (no report, no manifest fetch), and sends
// a follow-up "recovered" message once it's reporting again.
//
// This tracks each token's own alert state (agent_tokens.stale_alert_sent_at)
// rather than relying on slack.SendMessageToChannel's built-in dedup — that
// dedup is a permanent exact-text match with no expiry, so a fixed message
// like "X hasn't reported in over 3 hours" would only ever be delivered
// once, ever, and silently swallowed on every later, unrelated occurrence of
// the same server going stale again.
func checkStaleAgents() error {
	tokens, err := db.ListAgentTokens()
	if err != nil {
		return fmt.Errorf("failed to list agent tokens for staleness check: %w", err)
	}

	slackChannel := config.Cfg.SlackMonitorChannel
	if slackChannel == "" {
		slackChannel = config.Cfg.SlackChannel
	}

	now := time.Now().UTC()
	for _, tok := range tokens {
		// A revoked or never-yet-seen token (freshly created, not deployed
		// yet) isn't a reporting failure — nothing to alert on.
		if tok.Revoked || tok.LastSeenAt == nil {
			continue
		}
		lastSeen, err := parseSQLiteTimestamp(*tok.LastSeenAt)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to parse last_seen_at for agent token %d: %v", tok.ID, err)
			continue
		}
		var lastAlertedAt *time.Time
		if tok.StaleAlertSentAt != nil {
			if parsed, err := parseSQLiteTimestamp(*tok.StaleAlertSentAt); err == nil {
				lastAlertedAt = &parsed
			}
		}

		name := tok.ServerName
		if name == "" {
			name = fmt.Sprintf("server #%d", tok.ServerID)
		}

		switch decideStaleAgentAction(now, lastSeen, lastAlertedAt) {
		case staleAgentActionAlert:
			hours := int(now.Sub(lastSeen).Hours())
			msg := fmt.Sprintf("🚨 jman-agent on %s hasn't reported in over %dh (last seen %s UTC)", name, hours, lastSeen.Format("2006-01-02 15:04"))
			if err := slack.SendMessageToChannel(msg, slackChannel, true); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to send stale-agent Slack alert for token %d: %v", tok.ID, err)
				continue
			}
			if err := db.MarkAgentTokenStaleAlerted(tok.ID); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to record stale-agent alert for token %d: %v", tok.ID, err)
			}

		case staleAgentActionRecovered:
			msg := fmt.Sprintf("✅ jman-agent on %s is reporting again", name)
			if err := slack.SendMessageToChannel(msg, slackChannel, true); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to send agent-recovered Slack alert for token %d: %v", tok.ID, err)
				continue
			}
			if err := db.ClearAgentTokenStaleAlert(tok.ID); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to clear stale-agent alert for token %d: %v", tok.ID, err)
			}
		}
	}
	return nil
}
