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
	"github.com/JCO-Digital/jman/internal/verb"
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

	verb.LogPrintf(verb.Normal, "Monitoring %d sites...\n", len(sites))

	start := time.Now()

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
			verb.LogPrintf(verb.Debug, "Skipping ignored site: %s\n", site.Domain)
			continue
		}

		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			verb.LogPrintf(verb.Debug, "Checking %s...\n", domain)

			req, err := http.NewRequest(http.MethodGet, "https://"+domain, nil)
			if err != nil {
				mu.Lock()
				status := state.GetStatus(domain)
				status.FailureCount++
				mu.Unlock()
				return
			}
			req.Header.Set("User-Agent", "JMan Uptime Monitoring/1.0")
			resp, err := client.Do(req)
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
					verb.LogPrintf(verb.Normal, "%s\n", msg)
					_ = slack.SendMessageToChannel(msg, slackChannel, true)
				}
				state.RemoveStatus(domain)
			} else {
				status.FailureCount++
				verb.LogPrintf(verb.Verbose, "Site %s failure count: %d (%s)\n", domain, status.FailureCount, statusMsg)

				if status.FailureCount >= config.Cfg.MonitorThreshold {
					// Check if we should send an alert (not sent in last hour or never sent)
					if status.LastAlertTime.IsZero() || time.Since(status.LastAlertTime) > time.Hour {
						msg := fmt.Sprintf("🚨 Site %s is DOWN (Status: %s)", domain, statusMsg)
						verb.LogPrintf(verb.Normal, "%s\n", msg)

						err := slack.SendMessageToChannel(msg, slackChannel, true)
						if err == nil {
							status.LastAlertTime = time.Now()
							status.IsDown = true
						} else {
							verb.LogPrintf(verb.Normal, "Failed to send Slack alert for %s: %v\n", domain, err)
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

	verb.LogPrintf(verb.Verbose, "Monitoring check complete in %f seconds.\n", time.Since(start).Seconds())
	return nil
}
