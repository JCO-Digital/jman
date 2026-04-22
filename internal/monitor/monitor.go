package monitor

import (
	"context"
	"fmt"
	"sync"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// NotifyIfAlertingSiteIgnored sends a Slack message if a site that is in Alert Mode is ignored.
func NotifyIfAlertingSiteIgnored(domain, reason string) {
	inAlert, err := db.IsSiteInAlertMode(domain)
	if err != nil || !inAlert {
		return
	}

	slackChannel := config.Cfg.SlackMonitorChannel
	if slackChannel == "" {
		slackChannel = config.Cfg.SlackChannel
	}

	msg := fmt.Sprintf("⏸️ Monitoring for site %s has been PAUSED (site was in Alert Mode). Reason: %s", domain, reason)
	_ = slack.SendMessageToChannel(msg, slackChannel, true)
}

// Run is the legacy entry point that performs a one-off check.
// In the future, this might be changed to start the service depending on flags.
func Run() error {
	return RunOnce()
}

// RunService starts the continuous monitoring scheduler.
// It staggers initial checks and runs until the context is cancelled.
func RunService(ctx context.Context) error {
	migrateIgnoredSites()

	scheduler, err := NewScheduler()
	if err != nil {
		return err
	}
	return scheduler.Run(ctx)
}

// RunOnce performs a single check of all configured sites and updates their states.
// This is suitable for cron-based execution or manual troubleshooting.
func RunOnce() error {
	migrateIgnoredSites()

	state, err := LoadState()
	if err != nil {
		return err
	}

	engine := NewEngine()
	sites, err := cache.GetCachedSites()
	if err != nil {
		return err
	}

	verb.LogPrintf(verb.Normal, "Monitoring %d sites (one-off mode)...\n", len(sites))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 24)

	// Keep track of domains currently in cache
	activeDomains := make(map[string]bool)

	for _, site := range sites {
		activeDomains[site.Domain] = true
		status := state.GetStatus(site.Domain)

		wg.Add(1)
		go func(s *SiteStatus) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := engine.CheckSite(s); err != nil {
				verb.LogPrintf(verb.Normal, "Error checking site %s: %v\n", s.Domain, err)
			}
		}(status)
	}

	wg.Wait()

	// Cleanup stale sites (those in database but no longer in the cache)
	state.Mu.RLock()
	var staleDomains []string
	for domain := range state.Sites {
		if !activeDomains[domain] {
			staleDomains = append(staleDomains, domain)
		}
	}
	state.Mu.RUnlock()

	for _, domain := range staleDomains {
		verb.LogPrintf(verb.Debug, "Removing stale site status: %s\n", domain)
		state.RemoveStatus(domain)
	}

	return nil
}

// migrateIgnoredSites moves ignored sites from config.toml to the database if they are not already there.
func migrateIgnoredSites() {
	ignoredDomains, err := db.GetIgnoredDomains()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to fetch ignored sites during migration: %v\n", err)
		return
	}

	if len(config.Cfg.IgnoreSites) > 0 {
		migrated := false
		for _, domain := range config.Cfg.IgnoreSites {
			if !ignoredDomains[domain] {
				if err := db.IgnoreSite(domain, "Migrated from config.toml"); err == nil {
					ignoredDomains[domain] = true
					migrated = true
				} else {
					verb.LogPrintf(verb.Normal, "Warning: failed to migrate ignored site %s: %v\n", domain, err)
				}
			}
		}
		if migrated {
			verb.LogPrintf(verb.Normal, "Migration complete. You can now remove 'ignoreSites' from your config.toml.\n")
		}
	}
}
