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

			isUp := false
			statusMsg := ""
			errorCode := 0

			req, err := http.NewRequest(http.MethodGet, "https://"+domain, nil)
			if err == nil {
				req.Header.Set("User-Agent", "JMan Uptime Monitoring/1.0")
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
				state.RemoveStatus(domain)
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

	// Save updated state
	if err := state.SaveState(); err != nil {
		return fmt.Errorf("error saving monitor state: %w", err)
	}

	verb.LogPrintf(verb.Verbose, "Monitoring check complete in %f seconds.\n", time.Since(start).Seconds())
	return nil
}
