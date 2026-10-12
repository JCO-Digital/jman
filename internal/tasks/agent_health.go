package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// How long an agent token can go without an authenticated request
// (last_seen_at, touched by AgentAuthMiddleware on every manifest/report
// request — internal/api/agent_auth.go) before it's considered stale is
// config.AgentServerStaleAfter (30 minutes by default): agent sites aren't
// covered by the SSH refresh, so a dead agent means stale plugin and core
// data, and silent staleness is exactly how the site-traffic rollup
// corruption went unnoticed for days. It's still twice the agent's default
// 15-minute report interval, so one slow or retried report doesn't alert.

// agentStaleRepeatInterval bounds how often a still-stale agent is
// re-alerted, so an ongoing outage produces periodic reminders instead of
// either silence or a message on every hourly tick.
const agentStaleRepeatInterval = 24 * time.Hour

// sqliteTimestampFormat matches what SQLite's CURRENT_TIMESTAMP stores (used
// for last_seen_at and stale_alert_sent_at). Parsing it with the wrong format
// is exactly the mismatch this codebase already hit once, fixed in e4c8a90
// for site_traffic_hourly's cutoff comparison.
const sqliteTimestampFormat = "2006-01-02 15:04:05"

// parseSQLiteTimestamp parses a DATETIME value as read back through the
// driver. modernc.org/sqlite converts DATETIME columns to time.Time, so
// scanning one into a string yields RFC3339 ("2006-01-02T15:04:05Z") rather
// than the stored CURRENT_TIMESTAMP text; both forms are accepted.
func parseSQLiteTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
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

// decideStaleAgentAction applies a stale threshold and
// agentStaleRepeatInterval: alert the first time lastSeen crosses the
// threshold, alert again every agentStaleRepeatInterval while it remains
// stale, and report a recovery exactly once when something previously
// alerted on is no longer stale. It serves both whole agents and single
// sites' WordPress data collection.
func decideStaleAgentAction(now, lastSeen time.Time, lastAlertedAt *time.Time, threshold time.Duration) staleAgentAction {
	isStale := now.Sub(lastSeen) >= threshold
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
	threshold := config.AgentServerStaleAfter()
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
			name = fmt.Sprintf("server #%v", tok.ServerID)
		}

		switch decideStaleAgentAction(now, lastSeen, lastAlertedAt, threshold) {
		case staleAgentActionAlert:
			msg := fmt.Sprintf("🚨 jman-agent on %s hasn't reported in over %s (last seen %s UTC)", name, roundedDuration(now.Sub(lastSeen)), lastSeen.Format("2006-01-02 15:04"))
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

// roundedDuration formats d as whole minutes under an hour, otherwise
// whole hours ("45m", "3h").
func roundedDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// staleSiteAlertKind is the agent_stale_alerts kind for sites.
const staleSiteAlertKind = "site"

// staleSiteChange is a site whose WordPress data collection went stale or
// recovered, for checkStaleAgentSites' grouped messages.
type staleSiteChange struct {
	siteID string
	domain string
	detail string
}

// checkStaleAgentSites warns via Slack when jman-agent hasn't successfully
// collected a site's WordPress data (plugins, core) for
// config.AgentSiteStaleAfter. Only sites the agent has taken over from the
// SSH refresh count (it has collected them at least once); sites whose
// agent is stale as a whole are left to checkStaleAgents' alert. Sites
// that went stale or recovered in one check share one message each way,
// so an agent whose wp-cli breaks for every site sends one alert.
func checkStaleAgentSites() error {
	statuses, err := db.ListSiteAgentWPStatus()
	if err != nil {
		return err
	}
	alerted, err := db.ListAgentStaleAlerts(staleSiteAlertKind)
	if err != nil {
		return err
	}
	sites, err := db.ListManagedSites()
	if err != nil {
		return fmt.Errorf("failed to list sites for the stale site check: %w", err)
	}
	staleServers, err := staleAgentServers(time.Now().UTC())
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	threshold := config.AgentSiteStaleAfter()
	var stale, recovered []staleSiteChange
	checked := map[string]bool{}
	for _, site := range sites {
		status, ok := statuses[site.ID]
		if !ok || status.CollectedAt.IsZero() || !site.IsWordpress ||
			(site.Status != "deployed" && site.Status != "active") ||
			(site.ServerID != nil && staleServers[*site.ServerID]) {
			continue
		}
		checked[site.ID] = true

		var lastAlertedAt *time.Time
		if at, ok := alerted[site.ID]; ok {
			lastAlertedAt = &at
		}
		switch decideStaleAgentAction(now, status.CollectedAt, lastAlertedAt, threshold) {
		case staleAgentActionAlert:
			detail := fmt.Sprintf("last collected %s ago", roundedDuration(now.Sub(status.CollectedAt)))
			if status.Error != "" {
				detail += ": " + status.Error
			}
			stale = append(stale, staleSiteChange{siteID: site.ID, domain: site.Domain, detail: detail})
		case staleAgentActionRecovered:
			recovered = append(recovered, staleSiteChange{siteID: site.ID, domain: site.Domain})
		}
	}
	// Alerts of sites no longer checked (deleted, archived, no longer
	// WordPress) are dropped without a message.
	for id := range alerted {
		if !checked[id] {
			_ = db.ClearAgentStaleAlert(staleSiteAlertKind, id)
		}
	}

	slackChannel := config.Cfg.SlackMonitorChannel
	if slackChannel == "" {
		slackChannel = config.Cfg.SlackChannel
	}
	msg := staleSitesMessage(stale, recovered)
	if msg == "" {
		return nil
	}
	// Alert state only changes once the message is out, so a failed send
	// is retried on the next check.
	if err := slack.SendMessageToChannel(msg, slackChannel, true); err != nil {
		return fmt.Errorf("failed to send stale-site Slack alert: %w", err)
	}
	for _, s := range stale {
		if err := db.MarkAgentStaleAlerted(staleSiteAlertKind, s.siteID); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to record stale-site alert for %s: %v", s.domain, err)
		}
	}
	for _, s := range recovered {
		if err := db.ClearAgentStaleAlert(staleSiteAlertKind, s.siteID); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to clear stale-site alert for %s: %v", s.domain, err)
		}
	}
	return nil
}

// staleSitesMessage formats one Slack message for the sites that went
// stale and recovered in one check, or "" if there are none.
func staleSitesMessage(stale, recovered []staleSiteChange) string {
	var b strings.Builder
	if len(stale) > 0 {
		fmt.Fprintf(&b, "⚠️ jman-agent hasn't collected WordPress data (plugins, core) for %d site(s):", len(stale))
		for _, s := range stale {
			fmt.Fprintf(&b, "\n• %s (%s)", s.domain, s.detail)
		}
	}
	if len(recovered) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		names := make([]string, len(recovered))
		for i, s := range recovered {
			names[i] = s.domain
		}
		fmt.Fprintf(&b, "✅ jman-agent is collecting WordPress data again for: %s", strings.Join(names, ", "))
	}
	return b.String()
}

// staleAgentServers returns the servers whose agent is currently stale.
func staleAgentServers(now time.Time) (map[string]bool, error) {
	tokens, err := db.ListAgentTokens()
	if err != nil {
		return nil, fmt.Errorf("failed to list agent tokens: %w", err)
	}
	// A server is stale only if none of its live tokens has been seen
	// recently.
	fresh := map[string]bool{}
	seen := map[string]bool{}
	threshold := config.AgentServerStaleAfter()
	for _, tok := range tokens {
		if tok.Revoked || tok.LastSeenAt == nil {
			continue
		}
		seen[tok.ServerID] = true
		if lastSeen, err := parseSQLiteTimestamp(*tok.LastSeenAt); err == nil && now.Sub(lastSeen) < threshold {
			fresh[tok.ServerID] = true
		}
	}
	stale := map[string]bool{}
	for id := range seen {
		if !fresh[id] {
			stale[id] = true
		}
	}
	return stale, nil
}
