package monitor

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Scheduler manages the continuous monitoring of sites.
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
	if err := s.refreshSites(true); err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Worker pool settings
	const maxWorkers = 24
	jobs := make(chan *SiteStatus, maxWorkers)
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
					// CheckSite handles its own state persistence and Slack notifications
					if err := s.engine.CheckSite(status); err != nil {
						verb.LogPrintf(verb.Normal, "Error checking site %s: %v\n", status.Domain, err)
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
			// Refresh site list from cache in case new sites were added
			if err := s.refreshSites(false); err != nil {
				verb.LogPrintf(verb.Normal, "Error refreshing sites: %v\n", err)
				continue
			}

			// Fetch ignored domains to skip them
			ignoredDomains, err := db.GetIgnoredDomains()
			if err != nil {
				verb.LogPrintf(verb.Normal, "Warning: failed to fetch ignored sites: %v\n", err)
				ignoredDomains = make(map[string]bool)
			}

			now := time.Now()
			s.state.Mu.RLock()
			for _, status := range s.state.Sites {
				if ignoredDomains[strings.ToLower(status.Domain)] {
					continue
				}

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
						// Site queued for check
					default:
						// Pool is busy; reset in-flight status so it can be tried again
						status.Mu.Lock()
						status.InFlight = false
						status.Mu.Unlock()
						verb.LogPrintf(verb.Debug, "Worker pool full, skipping %s for this tick\n", status.Domain)
					}
				}
			}
			s.state.Mu.RUnlock()
		}
	}
}

// refreshSites syncs the internal state with the current site list from the cache.
// If applyJitter is true, it staggers the next check times for all sites.
func (s *Scheduler) refreshSites(applyJitter bool) error {
	sites, err := cache.GetCachedSites()
	if err != nil {
		return err
	}

	activeDomains := make(map[string]bool)
	for _, site := range sites {
		activeDomains[site.Domain] = true
		status := s.state.GetStatus(site.Domain)

		// Jitter check times on startup or for new sites
		status.Mu.Lock()
		if applyJitter || status.NextCheckAt.IsZero() {
			// Spread initial checks over a 5-minute window
			jitter := time.Duration(rand.Intn(300)) * time.Second
			status.NextCheckAt = time.Now().Add(jitter)
		}
		status.Mu.Unlock()
	}

	// Cleanup stale sites (those in status state but no longer in the cache)
	s.state.Mu.RLock()
	var staleDomains []string
	for domain := range s.state.Sites {
		if !activeDomains[domain] {
			staleDomains = append(staleDomains, domain)
		}
	}
	s.state.Mu.RUnlock()

	for _, domain := range staleDomains {
		verb.LogPrintf(verb.Debug, "Removing stale site status: %s\n", domain)
		s.state.RemoveStatus(domain)
	}

	return nil
}
