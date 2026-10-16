package monitor

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/pagerduty"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// StopReason says why a check's outage is closed out.
type StopReason int

const (
	StopPaused   StopReason = iota // a user paused the check
	StopEdited                     // a user changed what the check probes
	StopOrphaned                   // the check's site is gone
	StopDeleted                    // a user deleted the check
)

// CloseOutCheck ends a check's current outage because the check stopped
// running as it was: it was paused, edited, orphaned or deleted. The
// check's monitor state is dropped, so it starts afresh if it runs again,
// and a result still in flight is discarded. An active incident is resolved (by `by`),
// its PagerDuty alert resolved, and Slack told why, so the channel never
// reads it as a recovery. Without an active incident nothing is posted.
//
// label names the check in the message (empty: the incident's domain);
// note is an optional reason.
func CloseOutCheck(checkID, label string, reason StopReason, by, note string) error {
	stopActiveStatus(checkID)
	if db.GetAPIDB() != nil {
		globalWriteMu.Lock()
		err := db.DeleteMonitorStatus(checkID)
		globalWriteMu.Unlock()
		if err != nil {
			return fmt.Errorf("failed to reset status of check %s: %w", checkID, err)
		}
	}

	inc, err := db.GetActiveIncidentByCheck(checkID)
	if err != nil || inc == nil {
		return err
	}
	if _, err := db.ResolveIncident(inc.ID, by); err != nil {
		return err
	}
	if inc.PDTriggered && pagerduty.Enabled() {
		if err := pagerduty.ResolveEvent(checkID); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send PagerDuty resolve for check %s: %v\n", checkID, err)
		}
	}

	if label == "" {
		label = inc.Domain
	}
	msg := closeOutMessage(label, reason, by, note, inc.ID)
	verb.LogPrintf(verb.Normal, "%s\n", msg)
	if err := slack.SendMessageToChannel(msg, monitorSlackChannel(), true); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to send Slack message for check %s: %v\n", checkID, err)
	}
	return nil
}

func closeOutMessage(label string, reason StopReason, by, note string, incidentID int64) string {
	var msg string
	switch reason {
	case StopEdited:
		msg = fmt.Sprintf("✏️ Check %s was EDITED by %s; incident #%d closed. It will be re-checked with the new settings.", label, by, incidentID)
	case StopDeleted:
		msg = fmt.Sprintf("🗑️ Check %s was DELETED by %s; incident #%d closed.", label, by, incidentID)
	case StopOrphaned:
		msg = fmt.Sprintf("⏸️ Monitoring for %s STOPPED: its site was removed; incident #%d closed.", label, incidentID)
	default:
		msg = fmt.Sprintf("⏸️ Monitoring for %s was PAUSED by %s; incident #%d closed.", label, by, incidentID)
	}
	if note != "" {
		msg += " Reason: " + note
	}
	return msg
}

// stopActiveStatus drops a check from the running scheduler's state and
// marks its status stopped, so a run in flight discards its result.
func stopActiveStatus(checkID string) {
	activeSchedulerMu.RLock()
	scheduler := activeScheduler
	activeSchedulerMu.RUnlock()
	if scheduler == nil || scheduler.state == nil {
		return
	}

	scheduler.state.Mu.Lock()
	st, ok := scheduler.state.Checks[checkID]
	delete(scheduler.state.Checks, checkID)
	scheduler.state.Mu.Unlock()
	if ok {
		st.Mu.Lock()
		st.Stopped = true
		st.Mu.Unlock()
	}
}

// monitorSlackChannel is the Slack channel for uptime messages.
func monitorSlackChannel() string {
	if config.Cfg.SlackMonitorChannel != "" {
		return config.Cfg.SlackMonitorChannel
	}
	return config.Cfg.SlackChannel
}
