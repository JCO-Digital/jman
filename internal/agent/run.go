package agent

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verb"
)

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

	for _, site := range manifest.Sites {
		siteReport := models.AgentReportSite{SiteID: site.SiteID}

		sitePath, err := ResolveSitePath(site.Domain, site.SiteUser)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Skipping %s: %v", site.Domain, err)
			report.Sites = append(report.Sites, siteReport)
			continue
		}

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

		report.Sites = append(report.Sites, siteReport)
	}

	if err := client.SendReport(ctx, report); err != nil {
		return fmt.Errorf("failed to send report: %w", err)
	}
	verb.LogPrintf(verb.Verbose, "Reported data for %d site(s)", len(report.Sites))
	return nil
}
