package monitor

import (
	"errors"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// HeartbeatSettings configure a heartbeat check.
type HeartbeatSettings struct {
	Interval      time.Duration // expected time between finished check-ins
	Grace         time.Duration // allowed lateness on top of Interval
	FailThreshold int           // failed check-ins in a row before alerting
}

// Signal is what a check-in reports.
type Signal string

const (
	SignalOK    Signal = "ok"    // the job finished fine
	SignalFail  Signal = "fail"  // the job finished with an error
	SignalStart Signal = "start" // the job started; informational
)

// ErrNotHeartbeat is returned for a check-in to a check of another type.
var ErrNotHeartbeat = errors.New("not a heartbeat check")

// waitingUntil is the next-check time of a heartbeat check that has never
// checked in: it waits for its first check-in rather than alerting.
var waitingUntil = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

// heartbeatTarget is the target of a heartbeat check.
func heartbeatTarget(c models.MonitorCheck) Target {
	label := CheckLabel(c, "")
	threshold := c.FailThreshold
	if threshold < 1 {
		threshold = 1
	}
	return Target{
		CheckID: c.ID,
		Type:    models.MonitorCheckTypeHeartbeat,
		Domain:  label,
		Label:   label,
		Heartbeat: HeartbeatSettings{
			Interval:      time.Duration(c.IntervalSeconds) * time.Second,
			Grace:         time.Duration(c.GraceSeconds) * time.Second,
			FailThreshold: threshold,
		},
	}
}

// heartbeatDeadline is when a heartbeat check misses its next check-in:
// interval + grace after the last finished one. Zero while it waits for
// its first check-in.
func heartbeatDeadline(s *CheckStatus) time.Time {
	if s.LastChecked.IsZero() {
		return time.Time{}
	}
	return s.LastChecked.Add(s.Heartbeat.Interval + s.Heartbeat.Grace)
}

// heartbeatNextCheck is when the scheduler should next look at a heartbeat
// check: at its deadline, or every minute while it is down (to repeat
// alerts and escalate). Called with status.Mu held.
func heartbeatNextCheck(s *CheckStatus, now time.Time) time.Time {
	if s.CurrentMode == ModeAlert {
		if !s.NextCheckAt.IsZero() && s.NextCheckAt.Before(waitingUntil) {
			return s.NextCheckAt
		}
		return now.Add(time.Minute)
	}
	if d := heartbeatDeadline(s); !d.IsZero() {
		return d
	}
	return waitingUntil
}

// confirmHeartbeat applies a finished check-in (SignalOK or SignalFail),
// or with signal "" a look at the deadline, to a heartbeat check. It is
// the heartbeat counterpart of confirmHTTP: a missed deadline confirms an
// outage at once, failed check-ins after FailThreshold in a row, and the
// next OK check-in ends it. Called with status.Mu held.
func confirmHeartbeat(s *CheckStatus, signal Signal, now time.Time) outcome {
	threshold := max(s.Heartbeat.FailThreshold, 1)

	switch signal {
	case SignalOK:
		s.LastChecked = now
		s.FailureCount = 0
		s.ConsecutiveSuccesses++
		s.NextCheckAt = heartbeatDeadline(s)
		if s.CurrentMode == ModeAlert {
			s.CurrentMode = ModeNormal
			return outcomeRecovered
		}
		s.CurrentMode = ModeNormal
		return outcomeNone

	case SignalFail:
		s.LastChecked = now
		s.FailureCount++
		s.ConsecutiveSuccesses = 0
		switch {
		case s.CurrentMode == ModeAlert:
			s.NextCheckAt = now.Add(time.Minute)
			return outcomeStillDown
		case s.FailureCount >= threshold:
			s.CurrentMode = ModeAlert
			s.NextCheckAt = now.Add(time.Minute)
			return outcomeDown
		default:
			s.CurrentMode = ModeNormal
			s.NextCheckAt = heartbeatDeadline(s)
			return outcomeNone
		}

	default: // the deadline sweep
		deadline := heartbeatDeadline(s)
		if deadline.IsZero() {
			s.NextCheckAt = waitingUntil
			return outcomeNone
		}
		if s.CurrentMode == ModeAlert {
			s.NextCheckAt = now.Add(time.Minute)
			return outcomeStillDown
		}
		if now.Before(deadline) {
			s.NextCheckAt = deadline
			return outcomeNone
		}
		s.CurrentMode = ModeAlert
		s.NextCheckAt = now.Add(time.Minute)
		return outcomeDown
	}
}

// missedMessage describes a missed check-in deadline.
func missedMessage(s *CheckStatus) string {
	return "No check-in since " + s.LastChecked.UTC().Format("2006-01-02 15:04 UTC")
}

// sweepHeartbeat is a scheduler run of a heartbeat check: it alerts when
// the check-in deadline has passed, and repeats or escalates an ongoing
// outage. There is nothing to probe.
func (e *Engine) sweepHeartbeat(status *CheckStatus) error {
	now := time.Now()
	status.Mu.Lock()
	if status.Stopped {
		status.Mu.Unlock()
		return nil
	}
	missed := !status.LastChecked.IsZero() && !now.Before(heartbeatDeadline(status))
	oc := confirmHeartbeat(status, "", now)
	result := ProbeResult{Message: status.LastMessage}
	if missed {
		result.Message = missedMessage(status)
	} else if result.Message == "" {
		result.Message = "Check-in reported failure"
	}
	var n notifications
	if oc != outcomeNone {
		n = e.handleOutcome(status, oc, result)
	}
	checkID, domain := status.CheckID, status.Domain
	status.Mu.Unlock()

	if missed {
		RecordHistory(checkID, domain, false, result.Message, 0)
	}
	e.notify(status, n)

	status.Mu.Lock()
	stopped := status.Stopped
	status.Mu.Unlock()
	if stopped {
		return nil
	}
	return SaveCheckStatus(status)
}

// CheckIn records a check-in from a heartbeat check's job. A paused or
// orphaned check accepts it without effect, reporting accepted = false.
// message is the job's optional note, used for a failure.
func CheckIn(c models.MonitorCheck, signal Signal, message string) (accepted bool, err error) {
	if c.Type != models.MonitorCheckTypeHeartbeat {
		return false, ErrNotHeartbeat
	}
	if !c.Enabled || c.OrphanedAt != nil {
		return false, nil
	}

	engine, status, err := checkInStatus(heartbeatTarget(c))
	if err != nil {
		return false, err
	}

	now := time.Now()
	status.Mu.Lock()
	if status.Stopped {
		status.Mu.Unlock()
		return false, nil
	}
	if signal == SignalStart {
		status.LastStartedAt = now
		status.Mu.Unlock()
		return true, SaveCheckStatus(status)
	}

	result := ProbeResult{Up: signal == SignalOK}
	if !result.Up {
		result.Message = message
		if result.Message == "" {
			result.Message = "Check-in reported failure"
		}
		status.LastMessage = result.Message
	}
	oc := confirmHeartbeat(status, signal, now)
	var n notifications
	if oc != outcomeNone {
		n = engine.handleOutcome(status, oc, result)
	}
	checkID, domain := status.CheckID, status.Domain
	status.Mu.Unlock()

	RecordHistory(checkID, domain, result.Up, result.Message, 0)
	engine.notify(status, n)
	return true, SaveCheckStatus(status)
}

// checkInStatus returns the engine and status a check-in applies to: the
// running scheduler's, or, without one, a status loaded from the database.
func checkInStatus(t Target) (*Engine, *CheckStatus, error) {
	activeSchedulerMu.RLock()
	scheduler := activeScheduler
	activeSchedulerMu.RUnlock()
	if scheduler != nil && scheduler.state != nil {
		engine := scheduler.engine
		if engine == nil {
			engine = NewEngine()
		}
		return engine, scheduler.state.GetStatus(t), nil
	}

	state, err := LoadState()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load monitor state: %w", err)
	}
	return NewEngine(), state.GetStatus(t), nil
}
