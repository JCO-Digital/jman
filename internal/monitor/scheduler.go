package monitor

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Scheduler manages the continuous running of monitor checks.
type Scheduler struct {
	engine *Engine
	state  *State
}

// NewScheduler creates a new scheduler instance and initializes its state.
func NewScheduler() (*Scheduler, error) {
	state, err := LoadState()
	if err != nil {
		return nil, err
	}

	return &Scheduler{
		engine: NewEngine(),
		state:  state,
	}, nil
}

// Run starts the continuous monitoring loop. It runs until the context is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	verb.LogPrintf(verb.Normal, "Starting monitor scheduler...\n")

	// Initial population and jittering of check times to spread load.
	if err := s.refreshChecks(true); err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Worker pool settings
	const maxWorkers = 24
	jobs := make(chan *CheckStatus, maxWorkers)
	var wg sync.WaitGroup

	// Start worker pool
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case status, ok := <-jobs:
					if !ok {
						return
					}
					// Check handles its own state persistence and Slack notifications
					if err := s.engine.Check(status); err != nil {
						verb.LogPrintf(verb.Normal, "Error running check %s: %v\n", status.CheckID, err)
					}

					// Clear in-flight status after processing
					status.Mu.Lock()
					status.InFlight = false
					status.Mu.Unlock()
				}
			}
		}()
	}

	// Main scheduling loop
	for {
		select {
		case <-ctx.Done():
			verb.LogPrintf(verb.Normal, "Shutting down monitor scheduler...\n")
			close(jobs)
			wg.Wait()
			return ctx.Err()

		case <-ticker.C:
			// Refresh the checks in case they or their sites changed
			if err := s.refreshChecks(false); err != nil {
				verb.LogPrintf(verb.Normal, "Error refreshing checks: %v\n", err)
				continue
			}

			now := time.Now()
			s.state.Mu.RLock()
			for _, status := range s.state.Checks {
				status.Mu.Lock()
				isDue := now.After(status.NextCheckAt)
				alreadyInFlight := status.InFlight
				if isDue && !alreadyInFlight {
					status.InFlight = true
				}
				status.Mu.Unlock()

				if isDue && !alreadyInFlight {
					// Queue the check if the worker pool is not full
					select {
					case jobs <- status:
						// Check queued
					default:
						// Pool is busy; reset in-flight status so it can be tried again
						status.Mu.Lock()
						status.InFlight = false
						status.Mu.Unlock()
						verb.LogPrintf(verb.Debug, "Worker pool full, skipping %s for this tick\n", status.CheckID)
					}
				}
			}
			s.state.Mu.RUnlock()
		}
	}
}

// refreshChecks syncs the internal state with the checks that should run
// now: new ones are added and the rest (removed, paused, orphaned) dropped.
// If applyJitter is true, it staggers the next check times for all checks.
func (s *Scheduler) refreshChecks(applyJitter bool) error {
	targets, err := LoadTargets()
	if err != nil {
		return err
	}

	active := make(map[string]bool, len(targets))
	for _, t := range targets {
		active[t.CheckID] = true
		status := s.state.GetStatus(t)

		status.Mu.Lock()
		// A heartbeat check runs when its check-in deadline passes, never
		// on a jittered schedule.
		if t.Type == models.MonitorCheckTypeHeartbeat {
			status.NextCheckAt = heartbeatNextCheck(status, time.Now())
			status.Mu.Unlock()
			continue
		}
		// Jitter check times on startup or for new checks
		if applyJitter || status.NextCheckAt.IsZero() {
			// Spread initial checks over a 5-minute window
			jitter := time.Duration(rand.Intn(300)) * time.Second
			status.NextCheckAt = time.Now().Add(jitter)
		}
		status.Mu.Unlock()
	}

	// Drop stale checks (those in the state but no longer to be run)
	s.state.Mu.RLock()
	var stale []string
	for checkID := range s.state.Checks {
		if !active[checkID] {
			stale = append(stale, checkID)
		}
	}
	s.state.Mu.RUnlock()

	for _, checkID := range stale {
		verb.LogPrintf(verb.Debug, "Removing stale check status: %s\n", checkID)
		s.state.RemoveStatus(checkID)
	}

	return nil
}
