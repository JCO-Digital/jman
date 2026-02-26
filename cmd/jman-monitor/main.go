package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verbosity"
)

// Version is injected by the build flags
var Version = "dev"

var (
	flagVerbose bool
	flagDebug   bool
)

func main() {
	// Parse flags
	flag.BoolVar(&flagVerbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&flagVerbose, "v", false, "Enable verbose output (shorthand)")
	flag.BoolVar(&flagDebug, "debug", false, "Enable debug output")
	flag.BoolVar(&flagDebug, "d", false, "Enable debug output (shorthand)")
	flag.Parse()

	// Set verbosity
	if flagDebug {
		verbosity.Set(verbosity.Debug)
	} else if flagVerbose {
		verbosity.Set(verbosity.Verbose)
	} else {
		verbosity.Set(verbosity.Normal)
	}

	// Initialize config
	if err := config.Init(Version); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		os.Exit(1)
	}

	// Load monitor state
	state, err := monitor.LoadState()
	if err != nil {
		verbosity.Printf(verbosity.Normal, "Error loading monitor state: %v\n", err)
		os.Exit(1)
	}

	// Fetch sites
	sites, err := cache.GetCachedSites()
	if err != nil {
		verbosity.Printf(verbosity.Normal, "Error fetching sites: %v\n", err)
		os.Exit(1)
	}

	verbosity.Printf(verbosity.Normal, "Monitoring %d sites...\n", len(sites))

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
			verbosity.Printf(verbosity.Verbose, "Skipping ignored site: %s\n", site.Domain)
			continue
		}

		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			verbosity.Printf(verbosity.Debug, "Checking %s...\n", domain)

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
					verbosity.Printf(verbosity.Normal, "%s\n", msg)
					slack.SendMessageToChannel(msg, slackChannel, true)
				}
				state.RemoveStatus(domain)
			} else {
				status.FailureCount++
				verbosity.Printf(verbosity.Verbose, "Site %s failure count: %d (%s)\n", domain, status.FailureCount, statusMsg)

				if status.FailureCount >= config.Cfg.MonitorThreshold {
					// Check if we should send an alert (not sent in last hour or never sent)
					if status.LastAlertTime.IsZero() || time.Since(status.LastAlertTime) > time.Hour {
						msg := fmt.Sprintf("🚨 Site %s is DOWN (Status: %s)", domain, statusMsg)
						verbosity.Printf(verbosity.Normal, "%s\n", msg)

						err := slack.SendMessageToChannel(msg, slackChannel, true)
						if err == nil {
							status.LastAlertTime = time.Now()
							status.IsDown = true
						} else {
							verbosity.Printf(verbosity.Normal, "Failed to send Slack alert for %s: %v\n", domain, err)
						}
					}
				}
			}
		}(site.Domain)
	}

	wg.Wait()

	// Save updated state
	if err := state.SaveState(); err != nil {
		verbosity.Printf(verbosity.Normal, "Error saving monitor state: %v\n", err)
	}

	verbosity.Printf(verbosity.Normal, "Monitoring check complete.\n")
}
