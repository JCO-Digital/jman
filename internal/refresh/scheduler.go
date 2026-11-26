// Package refresh keeps jman-api's cached SpinupWP/plugin/vulnerability data
// fresh with an in-process scheduler, replacing the external `jman fetch`
// cron job that jman-api previously depended on. Its slow tick also syncs
// vulnerability exposures and sends new-vulnerability Slack alerts.
package refresh

import (
	"context"
	"log"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/inventory"
	"github.com/JCO-Digital/jman/internal/inventory/domainlookup"
	_ "github.com/JCO-Digital/jman/internal/inventory/hetzner"  // register the Hetzner Cloud provider
	_ "github.com/JCO-Digital/jman/internal/inventory/spinupwp" // register the SpinupWP provider
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
		return
	}
	// Keep the asset inventory (SpinupWP-linked servers and domains) in
	// step. The providers read the cache this tick just refreshed, so this
	// doesn't fetch the SpinupWP API again. A failure here is logged only:
	// stale inventory items are not worth paging anyone.
	if _, err := inventory.SyncAll(context.Background()); err != nil {
		log.Printf("Refresh scheduler: inventory sync failed: %v", err)
	}
}

// runSlowTick refreshes plugins, plugin info, vulnerabilities, and core
// versions across every managed site. Per-site/per-plugin failures inside
// RunFullRefresh are already logged and skipped rather than aborting the
// whole tick, so no Slack alert is raised here — that's routine noise
// (a single unreachable server over SSH), not something that should page
// anyone.
//
// On success, it syncs vulnerability exposures (see vuln.SyncVulnExposures)
// and sends a one-off Slack alert for each newly found vulnerability, right
// after the data that feeds them is fetched.
func runSlowTick() {
	// Registry lookups of inventory domains don't depend on the refresh
	// below, and pace themselves per registry, so they run alongside it.
	go runDomainLookups()

	if err := cache.RunFullRefresh(slowTTLs()); err != nil {
		log.Printf("Refresh scheduler: full refresh failed: %v", err)
		return
	}

	// Collect agentless external sites (WP flags, disk usage, plugins, core)
	if err := CollectAgentlessSites(); err != nil {
		log.Printf("Refresh scheduler: agentless collection error: %v", err)
	}

	if err := vuln.SyncVulnExposures(); err != nil {
		log.Printf("Refresh scheduler: vuln exposure sync failed: %v", err)
	}
	if err := vuln.AlertNewVulnerabilities(); err != nil {
		log.Printf("Refresh scheduler: new vuln Slack alerts failed: %v", err)
	}
}

// slowTTLs are the TTLs of the slow tick's refresh (see
// cache.RefreshTTLs). What is read from the sites is always read again
// (a TTL of 0): the tick interval already governs how often, and changes
// made outside jman should show up by the next tick. Plugin metadata
// changes rarely, and the latest version of each plugin is also taken
// from the sites' update data on every read, so a day is enough. Plugin
// and core vulnerability data is fetched again every two hours, which
// keeps new vulnerabilities showing up within hours without asking
// wpvulnerability.net for every plugin every tick.
func slowTTLs() cache.RefreshTTLs {
	return cache.RefreshTTLs{
		Sites:      0,
		PluginInfo: 24 * time.Hour,
		Vulns:      2 * time.Hour,
	}
}

// runDomainLookups refreshes the registry data (nameservers, registrar,
// expiry) of the inventory's domain items whose lookup is due. Failures are
// stored with each item and only logged here.
func runDomainLookups() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if _, err := domainlookup.RefreshDue(ctx); err != nil {
		log.Printf("Refresh scheduler: domain lookups: %v", err)
	}
}
