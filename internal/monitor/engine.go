package monitor

import (
	"fmt"
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Engine handles the execution of health checks for sites.
type Engine struct {
	client       *http.Client
	slackChannel string
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

	return &Engine{
		client:       utils.NewHTTPClient(time.Duration(timeout) * time.Second),
		slackChannel: slackChannel,
	}
}

// CheckSite performs a health check on a single site and updates its status based on the state machine logic.
func (e *Engine) CheckSite(status *SiteStatus) error {
	domain := status.Domain
	verb.LogPrintf(verb.Debug, "Checking %s (Mode: %s)...\n", domain, status.CurrentMode)

	isUp := false
	statusMsg := ""
	errorCode := 0

	req, err := http.NewRequest(http.MethodGet, "https://"+domain, nil)
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
				msgToSend = fmt.Sprintf("🚨 Site %s is DOWN (Status: %s)", domain, statusMsg)
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
			nextInterval = 5 * time.Minute
		} else {
			nextInterval = 1 * time.Minute
			// Repeat alert based on error type intervals
			if e.shouldRepeatAlert(status, errorCode) {
				msgToSend = fmt.Sprintf("🚨 Site %s is STILL DOWN (Status: %s)", domain, statusMsg)
			}
		}

	default:
		// Fallback for unknown modes
		status.CurrentMode = ModeNormal
		nextInterval = 5 * time.Minute
	}

	status.NextCheckAt = time.Now().Add(nextInterval)

	if msgToSend != "" {
		verb.LogPrintf(verb.Normal, "%s\n", msgToSend)
		err := slack.SendMessageToChannel(msgToSend, e.slackChannel, true)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send Slack alert for %s: %v\n", domain, err)
		} else {
			// Update alert time if we sent an alert message
			if !isUp {
				status.LastAlertTime = time.Now()
			}
		}
	}

	if oldMode != status.CurrentMode {
		verb.LogPrintf(verb.Verbose, "Site %s transitioned from %s to %s\n", domain, oldMode, status.CurrentMode)
	}

	return SaveSiteStatus(status)
}

// shouldRepeatAlert determines if enough time has passed to send another alert for a down site.
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
