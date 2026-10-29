// Package refresh keeps jman-api's cached SpinupWP/plugin/vulnerability data
// fresh with an in-process scheduler, replacing the external `jman fetch`
// cron job that jman-api previously depended on. Its slow tick also sends
// new-vulnerability Slack alerts.
package refresh

import (
	"context"
	"log"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/vuln"
)

// StartScheduler starts the background routines that keep servers/sites
// (fast tick) and plugins/plugin-info/vulnerabilities/core-versions (slow
// tick) fresh in the API process. It replaces the external `jman fetch`
// cron job that jman-api previously relied on for data freshness.
func StartScheduler(ctx context.Context) {
	if config.Cfg.RefreshDisabled {
		log.Println("Refresh scheduler disabled via config (refreshDisabled=true).")
		return
	}

	go func() {
		log.Println("Starting refresh scheduler (fast tick: servers/sites)...")

		time.Sleep(10 * time.Second)
		runFastTick()

		ticker := time.NewTicker(fastInterval())
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runFastTick()
			case <-ctx.Done():
				log.Println("Refresh scheduler (fast tick) stopped.")
				return
			}
		}
	}()

	go func() {
		log.Println("Starting refresh scheduler (slow tick: plugins/vulnerabilities/core)...")

		// Stagger the slow tick's initial run relative to the fast tick so
		// the two don't both hit external APIs the moment the process starts.
		time.Sleep(30 * time.Second)
		runSlowTick()

		ticker := time.NewTicker(slowInterval())
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				runSlowTick()
			case <-ctx.Done():
				log.Println("Refresh scheduler (slow tick) stopped.")
				return
			}
		}
	}()
}

func fastInterval() time.Duration {
	minutes := config.Cfg.RefreshFastInterval
	if minutes <= 0 {
		minutes = 5
	}
	return time.Duration(minutes) * time.Minute
}

func slowInterval() time.Duration {
	minutes := config.Cfg.RefreshSlowInterval
	if minutes <= 0 {
		minutes = 30
	}
	return time.Duration(minutes) * time.Minute
}

// runFastTick refreshes the cheap, frequently-needed server/site lists. A
// hard failure here stalls everything downstream (the monitor scheduler's
// site list, the slow tick's per-site work), so it's the one failure mode
// in this package worth alerting on.
func runFastTick() {
	if _, err := RefreshServersAndSites(); err != nil {
		msg := "🚨 jman-api failed to refresh servers/sites from SpinupWP: " + err.Error()
		log.Println(msg)
		_ = slack.SendMessage(msg, true)
	}
}

// runSlowTick refreshes plugins, plugin info, vulnerabilities, and core
// versions across every managed site. Per-site/per-plugin failures inside
// RunFullRefresh are already logged and skipped rather than aborting the
// whole tick, so no Slack alert is raised here — that's routine noise
// (a single unreachable server over SSH), not something that should page
// anyone.
//
// On success, it sends a one-off Slack alert for each newly found
// vulnerability, right after the data that feeds them is fetched.
func runSlowTick() {
	if err := cache.RunFullRefresh(slowTTL()); err != nil {
		log.Printf("Refresh scheduler: full refresh failed: %v", err)
		return
	}

	// Collect agentless external sites (WP flags, disk usage, plugins, core)
	if err := CollectAgentlessSites(); err != nil {
		log.Printf("Refresh scheduler: agentless collection error: %v", err)
	}

	if err := vuln.AlertNewVulnerabilities(); err != nil {
		log.Printf("Refresh scheduler: new vuln Slack alerts failed: %v", err)
	}
}

// slowTTL is the TTL passed to the cache layer's refresh functions. A TTL
// of 0 forces a fetch (see cache.ReadJSONCache), which is what a scheduler
// wants: the tick interval itself already governs how often data is
// refreshed, so each tick should unconditionally fetch rather than be
// skipped by the cache's own TTL check. The fast tick always fetches too
// (see RefreshServersAndSites).
func slowTTL() time.Duration {
	return 0
}
