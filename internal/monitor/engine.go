package monitor

import (
	"net/http"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/pagerduty"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Engine runs monitor checks.
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

	pdEscalationMinutes := 10
	if config.Cfg.PagerDutyEscalationMinutes > 0 {
		pdEscalationMinutes = config.Cfg.PagerDutyEscalationMinutes
	}

	return &Engine{
		client:                utils.NewHTTPClient(time.Duration(timeout) * time.Second),
		slackChannel:          monitorSlackChannel(),
		pagerDutyEnabled:      pagerduty.Enabled(),
		pdEscalationThreshold: time.Duration(pdEscalationMinutes) * time.Minute,
	}
}

// Check runs a monitor check once. The check's type decides, from the
// probe result, whether an outage started, continues or ended
// (confirmHTTP); what follows is the same for every type (handleOutcome).
func (e *Engine) Check(status *CheckStatus) error {
	status.Mu.Lock()
	if status.Stopped {
		status.Mu.Unlock()
		return nil
	}
	checkID := status.CheckID
	domain := status.Domain
	label := status.label()
	url := status.requestURL()
	currentMode := status.CurrentMode
	status.Mu.Unlock()

	verb.LogPrintf(verb.Debug, "Checking %s (Mode: %s)...\n", label, currentMode)

	result := e.probeHTTP(url)
	RecordHistory(checkID, domain, result.Up, result.Message, result.Code)

	status.Mu.Lock()
	// A check paused, edited or orphaned while probing: its outage was
	// closed out, so this result no longer counts.
	if status.Stopped {
		status.Mu.Unlock()
		return nil
	}
	oldMode := status.CurrentMode
	status.LastChecked = time.Now()
	oc, nextInterval := confirmHTTP(status, result.Up)
	n := e.handleOutcome(status, oc, result)
	status.NextCheckAt = time.Now().Add(nextInterval)
	newMode := status.CurrentMode
	status.Mu.Unlock()

	e.notify(status, n)

	if oldMode != newMode {
		verb.LogPrintf(verb.Verbose, "Check %s transitioned from %s to %s\n", label, oldMode, newMode)
	}

	status.Mu.Lock()
	stopped := status.Stopped
	status.Mu.Unlock()
	if stopped {
		return nil
	}
	return SaveCheckStatus(status)
}
