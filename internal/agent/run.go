package agent

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/JCO-Digital/jman/internal/agent/logs"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verb"
)

// maxTrafficEntriesPerReport bounds how many finalized hourly traffic
// entries (summed across every site on this server, and across both
// rotated and live log processing) a single report may include. jman-api
// enforces a 1MB request body limit; a worst-case entry (20 top pages + 20
// top referrers, each key capped at maxKeyLength) runs roughly 12-13KB, so
// this cap leaves a comfortable margin even on a server with many sites.
// Without it, a large backlog (e.g. months of untouched rotated logs, or
// simply many hours already elapsed in today's not-yet-rotated live file)
// could flush far more than that into one oversized report; anything
// beyond this budget is simply deferred to later cycles, not lost.
const maxTrafficEntriesPerReport = 40

// RunService runs the agent's continuous collection and self-update loop
// until ctx is cancelled.
func RunService(ctx context.Context, cfg Config, version string) error {
	client := NewClient(cfg)

	reportTicker := time.NewTicker(time.Duration(cfg.ReportIntervalMinutes) * time.Minute)
	defer reportTicker.Stop()

	// Run an initial collection immediately rather than waiting for the first tick.
	if err := collectAndReport(ctx, client, cfg, version); err != nil {
		verb.LogPrintf(verb.Normal, "Initial collection failed: %v", err)
	}

	var selfUpdateChan <-chan time.Time
	if cfg.SelfUpdateEnabled {
		// A few minutes of random jitter so many agents restarted around the
		// same time (e.g. after a fleet-wide reboot) don't all hit
		// api.github.com's rate limit in the same instant.
		jitter := time.Duration(rand.Intn(10*60)) * time.Second
		selfUpdateTicker := time.NewTicker(time.Duration(cfg.SelfUpdateCheckIntervalHours)*time.Hour + jitter)
		defer selfUpdateTicker.Stop()
		selfUpdateChan = selfUpdateTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reportTicker.C:
			if err := collectAndReport(ctx, client, cfg, version); err != nil {
				verb.LogPrintf(verb.Normal, "Collection failed: %v", err)
			}
		case <-selfUpdateChan:
			// On success this replaces the running process image and never
			// returns; on failure it logs and the loop continues.
			if err := CheckAndSelfUpdate(version); err != nil {
				verb.LogPrintf(verb.Normal, "Self-update check failed: %v", err)
			}
		}
	}
}

// RunOnce performs a single collection-and-report cycle, for --once/cron use.
func RunOnce(cfg Config, version string) error {
	client := NewClient(cfg)
	return collectAndReport(context.Background(), client, cfg, version)
}

func collectAndReport(ctx context.Context, client *Client, cfg Config, version string) error {
	manifest, err := client.FetchManifest(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// jman-api's own version is a reliable signal that a newer jman-agent
	// release exists too, since every binary in this repo shares one
	// version per release — check immediately rather than waiting for the
	// periodic self-update ticker (internal/agent/selfupdate.go), which
	// could otherwise take up to selfUpdateCheckIntervalHours to notice.
	// This comparison is local (no network call); CheckAndSelfUpdate is
	// only invoked, and GitHub only hit, when a mismatch is actually found.
	if cfg.SelfUpdateEnabled {
		if newer, err := update.IsNewer(manifest.APIVersion, version); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to compare agent version against jman-api's (%s): %v", manifest.APIVersion, err)
		} else if newer {
			verb.LogPrintf(verb.Normal, "jman-api is running %s (agent is %s) — checking for an agent update now", manifest.APIVersion, version)
			if err := CheckAndSelfUpdate(version); err != nil {
				verb.LogPrintf(verb.Normal, "Self-update check failed: %v", err)
			}
		}
	}

	report := models.AgentReport{
		CollectedAt:  time.Now().UTC().Format(time.RFC3339),
		AgentVersion: version,
	}

	// Log-tailing state is only persisted after a successful send (below),
	// so a failed report simply re-reads the same log range next cycle
	// instead of losing or duplicating traffic data.
	pendingLogStates := map[int]*logs.FileState{}

	// Shared across all sites in this cycle so a server with many
	// simultaneously-backlogged sites still produces one bounded report,
	// not one bounded-per-site report that's still too large in aggregate.
	remainingTrafficBudget := maxTrafficEntriesPerReport

	for _, site := range manifest.Sites {
		siteReport := models.AgentReportSite{SiteID: site.SiteID}

		if sitePath, err := ResolveSitePath(site.Domain, site.SiteUser); err != nil {
			verb.LogPrintf(verb.Normal, "Skipping disk usage/wp-flags for %s: %v", site.Domain, err)
		} else {
			if bytesUsed, err := CollectDiskUsage(sitePath); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to measure disk usage for %s at %s: %v", site.Domain, sitePath, err)
			} else {
				siteReport.DiskUsageBytes = &bytesUsed
			}

			if isMultisite, disallowFileMods, err := CollectWpFlags(sitePath); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to read wp-config.php flags for %s at %s: %v", site.Domain, sitePath, err)
			} else {
				siteReport.IsMultisite = &isMultisite
				siteReport.DisallowFileMods = &disallowFileMods
			}
		}

		// Access logs live at /sites/<domain>/logs — a sibling of the site's
		// content directory, independent of how (or whether) that resolved
		// above, so a site with unresolvable content can still report traffic.
		logsDir := filepath.Join("/sites", site.Domain, "logs")
		logState, err := logs.LoadState(cfg.StateDir, site.SiteID)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to load log state for %s: %v", site.Domain, err)
		} else if hourly, err := logs.Collect(logsDir, logState, time.Now(), remainingTrafficBudget); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to collect traffic logs for %s at %s: %v", site.Domain, logsDir, err)
		} else {
			siteReport.TrafficHourly = hourly
			pendingLogStates[site.SiteID] = logState
			remainingTrafficBudget -= len(hourly)
			if remainingTrafficBudget < 0 {
				remainingTrafficBudget = 0
			}
		}

		report.Sites = append(report.Sites, siteReport)
	}

	if err := client.SendReport(ctx, report); err != nil {
		return fmt.Errorf("failed to send report: %w", err)
	}

	for siteID, state := range pendingLogStates {
		if err := logs.SaveState(cfg.StateDir, siteID, state); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to save log state for site %d: %v", siteID, err)
		}
	}

	verb.LogPrintf(verb.Verbose, "Reported data for %d site(s)", len(report.Sites))
	return nil
}
