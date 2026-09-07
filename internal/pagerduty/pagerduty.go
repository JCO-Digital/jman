// Package pagerduty sends uptime alerts to PagerDuty's Events API v2.
package pagerduty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/utils"
)

// PagerDuty runs two separate data-center regions (US and EU), each with its
// own Events API v2 endpoint. An EU account's integration key is only
// recognized directly by the EU endpoint; PagerDuty will forward requests
// sent to the US endpoint for an EU key, but recommends against relying on
// that forwarding for latency/reliability reasons.
const (
	usEventsURL = "https://events.pagerduty.com/v2/enqueue"
	euEventsURL = "https://events.eu.pagerduty.com/v2/enqueue"
)

// eventsURLOverride, when non-empty, replaces the region-derived events URL.
// Tests use this to point at an httptest server.
var eventsURLOverride string

// eventsURL returns the Events API v2 endpoint to use, honoring
// config.Cfg.PagerDutyEURegion unless a test override is set.
func eventsURL() string {
	if eventsURLOverride != "" {
		return eventsURLOverride
	}
	if config.Cfg.PagerDutyEURegion {
		return euEventsURL
	}
	return usEventsURL
}

// Severity values accepted by the Events API v2 payload.
const (
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// event is the Events API v2 "enqueue" request body.
type event struct {
	RoutingKey  string        `json:"routing_key"`
	EventAction string        `json:"event_action"` // "trigger" or "resolve"
	DedupKey    string        `json:"dedup_key"`
	Payload     *eventPayload `json:"payload,omitempty"`
}

// eventPayload is required for "trigger" events and omitted for "resolve".
type eventPayload struct {
	Summary  string `json:"summary"`
	Source   string `json:"source"`
	Severity string `json:"severity"`
}

// Enabled reports whether a PagerDuty routing key is configured.
func Enabled() bool {
	return config.Cfg.TokenPagerDuty != ""
}

// DedupKey returns the deterministic PagerDuty dedup_key for domain's outage.
// It is a pure function of the (lowercased) domain, so the same site's
// repeated down/up cycles, and process restarts in between, all address the
// same PagerDuty alert lineage.
func DedupKey(domain string) string {
	return "jman-monitor:" + strings.ToLower(domain)
}

// TriggerEvent sends a "trigger" event for domain at the given severity
// (SeverityWarning or SeverityCritical), reusing DedupKey(domain) so a later
// critical trigger escalates the same alert rather than opening a new one.
func TriggerEvent(domain, summary, severity string) error {
	return sendEvent(event{
		RoutingKey:  config.Cfg.TokenPagerDuty,
		EventAction: "trigger",
		DedupKey:    DedupKey(domain),
		Payload: &eventPayload{
			Summary:  summary,
			Source:   domain,
			Severity: severity,
		},
	})
}

// ResolveEvent sends a "resolve" event for domain. Safe to call even if no
// trigger was ever sent for this outage: PagerDuty drops resolve events that
// have no matching open alert rather than erroring.
func ResolveEvent(domain string) error {
	return sendEvent(event{
		RoutingKey:  config.Cfg.TokenPagerDuty,
		EventAction: "resolve",
		DedupKey:    DedupKey(domain),
	})
}

func sendEvent(e event) error {
	if e.RoutingKey == "" {
		return fmt.Errorf("PagerDuty routing key is not configured")
	}

	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to encode PagerDuty event: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, eventsURL(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build PagerDuty request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	utils.SetStandardHeaders(req)

	client := utils.NewHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("PagerDuty request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PagerDuty API error (status %d): %s", resp.StatusCode, string(raw))
	}

	return nil
}
