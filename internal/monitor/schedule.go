package monitor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
)

var (
	activeScheduler   *Scheduler
	activeSchedulerMu sync.RWMutex
)

// StartScheduler starts the continuous site-monitoring scheduler as a
// background goroutine, following the same StartScheduler(ctx) convention
// used by internal/backup and internal/tasks. Construction errors (e.g.
// failure to load state from the database) are returned synchronously so
// the caller can decide whether to abort startup; the monitoring loop
// itself then runs until ctx is cancelled.
//
// Do not call this from a process that also runs a separate jman-monitor
// instance against the same database — see state.go's globalWriteMu doc
// comment for why that's unsafe.
func StartScheduler(ctx context.Context) error {
	scheduler, err := NewScheduler()
	if err != nil {
		return fmt.Errorf("failed to start monitor scheduler: %w", err)
	}

	activeSchedulerMu.Lock()
	activeScheduler = scheduler
	activeSchedulerMu.Unlock()

	go func() {
		defer func() {
			activeSchedulerMu.Lock()
			if activeScheduler == scheduler {
				activeScheduler = nil
			}
			activeSchedulerMu.Unlock()
		}()
		if err := scheduler.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("Monitor scheduler stopped with error: %v", err)
		}
	}()

	return nil
}

// ResetDomainStatus resets the monitoring status of a domain back to normal,
// clearing down state so that subsequent checks will re-evaluate from scratch.
func ResetDomainStatus(domain string) {
	domain = strings.ToLower(domain)

	activeSchedulerMu.RLock()
	scheduler := activeScheduler
	activeSchedulerMu.RUnlock()

	if scheduler != nil && scheduler.state != nil {
		scheduler.state.Mu.Lock()
		if st, ok := scheduler.state.Sites[domain]; ok {
			st.Mu.Lock()
			st.CurrentMode = ModeNormal
			st.IsDown = false
			st.FailureCount = 0
			st.ConsecutiveSuccesses = 0
			st.DownSince = time.Time{}
			st.PDTriggered = false
			st.PDEscalated = false
			st.NextCheckAt = time.Now()
			st.Mu.Unlock()
			_ = SaveSiteStatus(st)
		}
		scheduler.state.Mu.Unlock()
	} else {
		database := db.GetAPIDB()
		if database != nil {
			_, _ = database.Exec(`
				UPDATE monitor_status
				SET current_mode = 'normal', is_down = 0, failure_count = 0, consecutive_successes = 0,
				    down_since = NULL, pd_triggered = 0, pd_escalated = 0, next_check_at = CURRENT_TIMESTAMP
				WHERE domain = ?
			`, domain)
		}
	}
}
