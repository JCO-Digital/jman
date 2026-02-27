package monitor

import (
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verbosity"
)

// Run checks all configured sites and sends alerts if they are down.
func Run() error {
	// Load monitor state
	state, err := LoadState()
	if err != nil {
		return fmt.Errorf("error loading monitor state: %w", err)
	}

	// Fetch sites
	sites, err := cache.GetCachedSites()
	if err != nil {
		return fmt.Errorf("error fetching sites: %w", err)
	}

	verbosity.LogPrintf(verbosity.Normal, "Monitoring %d sites...\n", len(sites))

	// Monitoring parameters
	semaphore := make(chan struct{}, 24)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Select Slack channel
	slackChannel := config.Cfg.SlackMonitorChannel
	if slackChannel == "" {
		slackChannel = config.Cfg.SlackChannel
	}

	client := &http.Client{
		Timeout: time.Duration(config.Cfg.MonitorTimeout) * time.Second,
	}

	for _, site := range sites {
		// Check if site is ignored
		isIgnored := slices.Contains(config.Cfg.IgnoreSites, site.Domain)
		if isIgnored {
			verbosity.LogPrintf(verbosity.Verbose, "Skipping ignored site: %s\n", site.Domain)
			continue
		}

		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			verbosity.LogPrintf(verbosity.Debug, "Checking %s...\n", domain)

			resp, err := client.Get("https://" + domain)
			isUp := false
			statusMsg := ""

			if err == nil {
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					isUp = true
				} else {
					statusMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
				}
				resp.Body.Close()
			} else {
				statusMsg = fmt.Sprintf("Error: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()

			status := state.GetStatus(domain)

			if isUp {
				if status.IsDown {
					// Site came back up
					msg := fmt.Sprintf("✅ Site %s is back up.", domain)
					verbosity.LogPrintf(verbosity.Normal, "%s\n", msg)
					_ = slack.SendMessageToChannel(msg, slackChannel, true)
				}
				state.RemoveStatus(domain)
			} else {
				status.FailureCount++
				verbosity.LogPrintf(verbosity.Verbose, "Site %s failure count: %d (%s)\n", domain, status.FailureCount, statusMsg)

				if status.FailureCount >= config.Cfg.MonitorThreshold {
					// Check if we should send an alert (not sent in last hour or never sent)
					if status.LastAlertTime.IsZero() || time.Since(status.LastAlertTime) > time.Hour {
						msg := fmt.Sprintf("🚨 Site %s is DOWN (Status: %s)", domain, statusMsg)
						verbosity.LogPrintf(verbosity.Normal, "%s\n", msg)

						err := slack.SendMessageToChannel(msg, slackChannel, true)
						if err == nil {
							status.LastAlertTime = time.Now()
							status.IsDown = true
						} else {
							verbosity.LogPrintf(verbosity.Normal, "Failed to send Slack alert for %s: %v\n", domain, err)
						}
					}
				}
			}
		}(site.Domain)
	}

	wg.Wait()

	// Save updated state
	if err := state.SaveState(); err != nil {
		return fmt.Errorf("error saving monitor state: %w", err)
	}

	verbosity.LogPrintf(verbosity.Verbose, "Monitoring check complete.\n")
	return nil
}
