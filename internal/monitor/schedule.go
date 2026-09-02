package monitor

import (
	"context"
	"fmt"
	"log"
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

	go func() {
		if err := scheduler.Run(ctx); err != nil && err != context.Canceled {
			log.Printf("Monitor scheduler stopped with error: %v", err)
		}
	}()

	return nil
}
