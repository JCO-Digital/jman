package monitor

import (
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/pagerduty"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// outcome is what a probe result means for a check's outage, as decided
// by the check type's confirmation rules.
type outcome int

const (
	outcomeNone      outcome = iota // no outage, or not confirmed yet
	outcomeDown                     // a new outage is confirmed
	outcomeStillDown                // an ongoing outage continues
	outcomeRecovered                // an ongoing outage ended
)

// notifications are the messages a check run sends once status.Mu is
// released.
type notifications struct {
	slack      string
	pdSeverity string // "" = no PagerDuty trigger
	pdResolve  bool
	// alert marks slack as a down alert, which sets LastAlertTime.
	alert   bool
	summary string // PagerDuty summary
}

// handleOutcome is the part of a check run shared by every check type: it
// keeps the check's outage state, opens and resolves its incident, and
// decides on Slack alerts and PagerDuty escalation. Called with status.Mu
// held; the returned notifications are sent by notify after unlocking.
func (e *Engine) handleOutcome(status *CheckStatus, oc outcome, r ProbeResult) notifications {
	label := status.label()
	var n notifications

	switch oc {
	case outcomeDown:
		status.IsDown = true
		status.DownSince = time.Now()
		n.slack = fmt.Sprintf("🚨 %s %s is DOWN (Status: %s)", status.subject(), label, r.Message)
		n.alert = true
		if _, err := db.CreateIncident(status.CheckID, status.Domain, r.Message, r.Code, status.DownSince); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to create incident for %s: %v\n", label, err)
		}

	case outcomeStillDown:
		activeInc, _ := db.GetActiveIncidentByCheck(status.CheckID)
		isAcked := activeInc != nil && activeInc.Status == models.IncidentStatusAcknowledged

		// Repeat alert based on error type intervals if not acknowledged
		if !isAcked && shouldRepeatAlert(status, r.Code) {
			n.slack = fmt.Sprintf("🚨 %s %s is STILL DOWN (Status: %s)", status.subject(), label, r.Message)
			n.alert = true
		}
		// Trigger the PagerDuty incident with critical severity once the check
		// has been down longer than the configured threshold, provided the incident
		// has NOT been acknowledged or resolved in jman.
		if e.pagerDutyEnabled && activeInc != nil && activeInc.Status == models.IncidentStatusOpen &&
			!status.PDTriggered && !status.DownSince.IsZero() &&
			time.Since(status.DownSince) >= e.pdEscalationThreshold {
			n.pdSeverity = pagerduty.SeverityCritical
			n.summary = fmt.Sprintf("%s %s is down (Status: %s)", status.subject(), label, r.Message)
		}

	case outcomeRecovered:
		status.IsDown = false
		n.slack = fmt.Sprintf("✅ %s %s is back up.", status.subject(), label)
		if _, err := db.ResolveActiveIncidentByCheck(status.CheckID, ""); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to auto-resolve incident for %s: %v\n", label, err)
		}
		n.pdResolve = e.pagerDutyEnabled && status.PDTriggered
		status.DownSince = time.Time{}
		status.PDTriggered = false
		status.PDEscalated = false
	}

	return n
}

// notify sends what handleOutcome decided, unless the check was stopped in
// the meantime. Called without status.Mu held.
func (e *Engine) notify(status *CheckStatus, n notifications) {
	status.Mu.Lock()
	stopped := status.Stopped
	checkID := status.CheckID
	domain := status.Domain
	label := status.label()
	status.Mu.Unlock()
	if stopped {
		return
	}

	if n.slack != "" {
		verb.LogPrintf(verb.Normal, "%s\n", n.slack)
		if err := slack.SendMessageToChannel(n.slack, e.slackChannel, true); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send Slack alert for %s: %v\n", label, err)
		} else if n.alert {
			status.Mu.Lock()
			status.LastAlertTime = time.Now()
			status.Mu.Unlock()
		}
	}

	if n.pdSeverity != "" {
		if err := pagerduty.TriggerEvent(checkID, domain, n.summary, n.pdSeverity); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send PagerDuty trigger for %s: %v\n", label, err)
		} else {
			status.Mu.Lock()
			status.PDTriggered = true
			if n.pdSeverity == pagerduty.SeverityCritical {
				status.PDEscalated = true
			}
			status.Mu.Unlock()
			if activeInc, err := db.GetActiveIncidentByCheck(checkID); err == nil && activeInc != nil {
				_ = db.SetIncidentPDTriggered(activeInc.ID, true)
			}
		}
	}

	if n.pdResolve {
		if err := pagerduty.ResolveEvent(checkID); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send PagerDuty resolve for %s: %v\n", label, err)
		}
	}
}

// shouldRepeatAlert determines if enough time has passed to send another alert for a down check.
// Called with status.Mu held.
func shouldRepeatAlert(status *CheckStatus, errorCode int) bool {
	if status.LastAlertTime.IsZero() {
		return true
	}

	var interval time.Duration
	switch {
	case errorCode >= 500:
		interval = 30 * time.Minute
	case errorCode >= 400:
		interval = 60 * time.Minute
	default:
		interval = 120 * time.Minute
	}

	return time.Since(status.LastAlertTime) >= interval
}
