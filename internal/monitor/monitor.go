package monitor

import (
	"context"
	"fmt"
	"sync"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// NotifyIfAlertingCheckPaused sends a Slack message if a check that is in Alert Mode was paused.
func NotifyIfAlertingCheckPaused(checkID, label, reason string) {
	inAlert, err := db.IsCheckInAlertMode(checkID)
	if err != nil || !inAlert {
		return
	}

	slackChannel := config.Cfg.SlackMonitorChannel
	if slackChannel == "" {
		slackChannel = config.Cfg.SlackChannel
	}

	msg := fmt.Sprintf("⏸️ Monitoring for %s has been PAUSED (it was in Alert Mode). Reason: %s", label, reason)
	if err := slack.SendMessageToChannel(msg, slackChannel, true); err != nil {
		verb.LogPrintf(verb.Debug, "Failed to send paused alerting check notification for %s to Slack channel %s: %v\n", label, slackChannel, err)
	}
}

// Run is the legacy entry point that performs a one-off check.
// In the future, this might be changed to start the service depending on flags.
func Run() error {
	return RunOnce()
}

// RunService starts the continuous monitoring scheduler.
// It staggers initial checks and runs until the context is cancelled.
func RunService(ctx context.Context) error {
	scheduler, err := NewScheduler()
	if err != nil {
		return err
	}
	return scheduler.Run(ctx)
}

// RunOnce runs every check once and updates their states.
// This is suitable for cron-based execution or manual troubleshooting.
func RunOnce() error {
	state, err := LoadState()
	if err != nil {
		return err
	}

	engine := NewEngine()
	targets, err := LoadTargets()
	if err != nil {
		return err
	}

	verb.LogPrintf(verb.Normal, "Running %d checks (one-off mode)...\n", len(targets))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 24)

	// Keep track of the checks that should run
	active := make(map[string]bool)

	for _, t := range targets {
		active[t.CheckID] = true
		status := state.GetStatus(t)

		wg.Add(1)
		go func(s *CheckStatus) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := engine.Check(s); err != nil {
				verb.LogPrintf(verb.Normal, "Error running check %s: %v\n", s.CheckID, err)
			}
		}(status)
	}

	wg.Wait()

	// Drop stale checks (those in the database but no longer to be run)
	state.Mu.RLock()
	var stale []string
	for checkID := range state.Checks {
		if !active[checkID] {
			stale = append(stale, checkID)
		}
	}
	state.Mu.RUnlock()

	for _, checkID := range stale {
		verb.LogPrintf(verb.Debug, "Removing stale check status: %s\n", checkID)
		state.RemoveStatus(checkID)
	}

	return nil
}
