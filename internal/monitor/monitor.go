package monitor

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/utils"
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

	client := utils.NewHTTPClient(time.Duration(config.Cfg.MonitorTimeout) * time.Second)

	// Fetch ignored sites from DB
	ignoredDomains, err := db.GetIgnoredDomains()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to fetch ignored sites from database: %v\n", err)
		ignoredDomains = make(map[string]bool)
	}

	// Migrate from config if necessary
	if len(config.Cfg.IgnoreSites) > 0 {
		verb.LogPrintf(verb.Normal, "Migrating ignored sites from config to database...\n")
		for _, domain := range config.Cfg.IgnoreSites {
			if !ignoredDomains[domain] {
				if err := db.IgnoreSite(domain, "Migrated from config.toml"); err != nil {
					verb.LogPrintf(verb.Normal, "Warning: failed to migrate site %s: %v\n", domain, err)
				} else {
					ignoredDomains[domain] = true
				}
			}
		}
		verb.LogPrintf(verb.Normal, "Migration complete. You can now remove 'ignoreSites' from your config.toml.\n")
	}

	activeSites := make(map[string]bool)
	for _, site := range sites {
		activeSites[site.Domain] = true

		// Check if site is ignored
		if ignoredDomains[site.Domain] {
			verb.LogPrintf(verb.Debug, "Skipping ignored site: %s\n", site.Domain)
			continue
		}

		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			verb.LogPrintf(verb.Debug, "Checking %s...\n", domain)

			isUp := false
			statusMsg := ""
			errorCode := 0

			req, err := http.NewRequest(http.MethodGet, "https://"+domain, nil)
			if err == nil {
				utils.SetStandardHeaders(req)
				resp, errDo := client.Do(req)
				if errDo == nil {
					errorCode = resp.StatusCode
					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						isUp = true
					} else {
						statusMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
					}
					resp.Body.Close()
				} else {
					statusMsg = fmt.Sprintf("Error: %v", errDo)
				}
			} else {
				statusMsg = fmt.Sprintf("Request Error: %v", err)
			}

			mu.Lock()
			status := state.GetStatus(domain)
			state.RecordHistory(domain, isUp, statusMsg, errorCode)

			var msgToSend string
			var isRecovery bool

			if isUp {
				if status.IsDown {
					// Site came back up
					msgToSend = fmt.Sprintf("✅ Site %s is back up.", domain)
					isRecovery = true
				}
				status.IsDown = false
				status.FailureCount = 0
			} else {
				status.FailureCount++
				verb.LogPrintf(verb.Verbose, "Site %s failure count: %d (%s)\n", domain, status.FailureCount, statusMsg)

				if status.FailureCount >= config.Cfg.MonitorThreshold {
					// Check if we should send an alert (not sent in last hour or never sent)
					if status.LastAlertTime.IsZero() || time.Since(status.LastAlertTime) > time.Hour {
						msgToSend = fmt.Sprintf("🚨 Site %s is DOWN (Status: %s)", domain, statusMsg)
					}
				}
			}
			mu.Unlock()

			if msgToSend != "" {
				verb.LogPrintf(verb.Normal, "%s\n", msgToSend)
				err := slack.SendMessageToChannel(msgToSend, slackChannel, true)

				if err != nil {
					verb.LogPrintf(verb.Normal, "Failed to send Slack alert for %s: %v\n", domain, err)
				} else if !isRecovery {
					mu.Lock()
					status = state.GetStatus(domain)
					status.LastAlertTime = time.Now()
					status.IsDown = true
					mu.Unlock()
				}
			}
		}(site.Domain)
	}

	wg.Wait()

	// Cleanup stale sites (those in status state but no longer in the cache)
	for domain := range state.Sites {
		if !activeSites[domain] {
			verb.LogPrintf(verb.Debug, "Removing stale site status: %s\n", domain)
			state.RemoveStatus(domain)
		}
	}

	// Save updated state
	if err := state.SaveState(); err != nil {
		return fmt.Errorf("error saving monitor state: %w", err)
	}

	verb.LogPrintf(verb.Verbose, "Monitoring check complete in %f seconds.\n", time.Since(start).Seconds())
	return nil
}
