package monitor

import (
	"fmt"
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/pagerduty"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Engine handles the execution of health checks for sites.
type Engine struct {
	client                *http.Client
	slackChannel          string
	pagerDutyEnabled      bool
	pdEscalationThreshold time.Duration
}

// NewEngine creates a new monitoring engine with configured timeout and slack channel.
func NewEngine() *Engine {
	timeout := 30
	if config.Cfg.MonitorTimeout > 0 {
		timeout = config.Cfg.MonitorTimeout
	}

	slackChannel := config.Cfg.SlackMonitorChannel
	if slackChannel == "" {
		slackChannel = config.Cfg.SlackChannel
	}

	pdEscalationMinutes := 10
	if config.Cfg.PagerDutyEscalationMinutes > 0 {
		pdEscalationMinutes = config.Cfg.PagerDutyEscalationMinutes
	}

	return &Engine{
		client:                utils.NewHTTPClient(time.Duration(timeout) * time.Second),
		slackChannel:          slackChannel,
		pagerDutyEnabled:      pagerduty.Enabled(),
		pdEscalationThreshold: time.Duration(pdEscalationMinutes) * time.Minute,
	}
}

// CheckSite performs a health check on a single site and updates its status based on the state machine logic.
func (e *Engine) CheckSite(status *SiteStatus) error {
	status.Mu.Lock()
	domain := status.Domain
	currentMode := status.CurrentMode
	status.Mu.Unlock()

	verb.LogPrintf(verb.Debug, "Checking %s (Mode: %s)...\n", domain, currentMode)

	isUp := false
	statusMsg := ""
	errorCode := 0

	url := "https://" + domain
	if config.Cfg.MonitorCacheBypass {
		url += fmt.Sprintf("?jman_cache_bypass=%d", time.Now().UnixNano())
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err == nil {
		utils.SetStandardHeaders(req)
		resp, errDo := e.client.Do(req)
		if errDo == nil {
			errorCode = resp.StatusCode
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				isUp = true
			} else {
				statusMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			resp.Body.Close()
		} else {
			statusMsg = fmt.Sprintf("Error: %v", errDo)
		}
	} else {
		statusMsg = fmt.Sprintf("Request Error: %v", err)
	}

	// Record check in history
	RecordHistory(domain, isUp, statusMsg, errorCode)

	// State transition logic
	status.Mu.Lock()
	oldMode := status.CurrentMode
	status.LastChecked = time.Now()

	if isUp {
		status.ConsecutiveSuccesses++
		status.FailureCount = 0
	} else {
		status.FailureCount++
		status.ConsecutiveSuccesses = 0
	}

	var msgToSend string
	var nextInterval time.Duration
	var pdSeverity string // "" = no PagerDuty trigger this tick
	var pdResolve bool

	switch status.CurrentMode {
	case ModeNormal:
		if !isUp {
			status.CurrentMode = ModeInvestigation
			nextInterval = 1 * time.Minute
		} else {
			nextInterval = 5 * time.Minute
		}

	case ModeInvestigation:
		if isUp {
			if status.ConsecutiveSuccesses >= 3 {
				status.CurrentMode = ModeNormal
				nextInterval = 5 * time.Minute
			} else {
				nextInterval = 1 * time.Minute
			}
		} else {
			if status.FailureCount >= 3 {
				status.CurrentMode = ModeAlert
				status.IsDown = true
				status.DownSince = time.Now()
				msgToSend = fmt.Sprintf("🚨 Site %s is DOWN (Status: %s)", domain, statusMsg)
				if e.pagerDutyEnabled {
					pdSeverity = pagerduty.SeverityWarning
				}
				nextInterval = 1 * time.Minute
			} else {
				nextInterval = 1 * time.Minute
			}
		}

	case ModeAlert:
		if isUp {
			status.CurrentMode = ModeNormal
			status.IsDown = false
			msgToSend = fmt.Sprintf("✅ Site %s is back up.", domain)
			if e.pagerDutyEnabled {
				pdResolve = true
			}
			status.DownSince = time.Time{}
			status.PDTriggered = false
			status.PDEscalated = false
			nextInterval = 5 * time.Minute
		} else {
			nextInterval = 1 * time.Minute
			// Repeat alert based on error type intervals
			if e.shouldRepeatAlert(status, errorCode) {
				msgToSend = fmt.Sprintf("🚨 Site %s is STILL DOWN (Status: %s)", domain, statusMsg)
			}
			// Escalate the PagerDuty alert to critical severity once the site
			// has been down longer than the configured threshold. Independent
			// of shouldRepeatAlert's Slack-repeat throttle above.
			if e.pagerDutyEnabled && !status.PDEscalated && !status.DownSince.IsZero() &&
				time.Since(status.DownSince) >= e.pdEscalationThreshold {
				pdSeverity = pagerduty.SeverityCritical
			}
		}

	default:
		// Fallback for unknown modes
		status.CurrentMode = ModeNormal
		nextInterval = 5 * time.Minute
	}

	status.NextCheckAt = time.Now().Add(nextInterval)
	newMode := status.CurrentMode
	status.Mu.Unlock()

	if msgToSend != "" {
		verb.LogPrintf(verb.Normal, "%s\n", msgToSend)
		err := slack.SendMessageToChannel(msgToSend, e.slackChannel, true)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send Slack alert for %s: %v\n", domain, err)
		} else if !isUp {
			// Update alert time if we sent an alert message and site is down
			status.Mu.Lock()
			status.LastAlertTime = time.Now()
			status.Mu.Unlock()
		}
	}

	if pdSeverity != "" {
		summary := fmt.Sprintf("Site %s is down (Status: %s)", domain, statusMsg)
		if err := pagerduty.TriggerEvent(domain, summary, pdSeverity); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send PagerDuty trigger for %s: %v\n", domain, err)
		} else {
			status.Mu.Lock()
			status.PDTriggered = true
			if pdSeverity == pagerduty.SeverityCritical {
				status.PDEscalated = true
			}
			status.Mu.Unlock()
		}
	}

	if pdResolve {
		if err := pagerduty.ResolveEvent(domain); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send PagerDuty resolve for %s: %v\n", domain, err)
		}
	}

	if oldMode != newMode {
		verb.LogPrintf(verb.Verbose, "Site %s transitioned from %s to %s\n", domain, oldMode, newMode)
	}

	return SaveSiteStatus(status)
}

// shouldRepeatAlert determines if enough time has passed to send another alert for a down site.
// Note: This is called within CheckSite while status.Mu is held.
func (e *Engine) shouldRepeatAlert(status *SiteStatus, errorCode int) bool {
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
