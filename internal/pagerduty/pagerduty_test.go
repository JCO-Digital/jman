package pagerduty

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
)

func withTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	originalURL := eventsURLOverride
	originalKey := config.Cfg.TokenPagerDuty
	eventsURLOverride = server.URL
	config.Cfg.TokenPagerDuty = "test-routing-key"
	t.Cleanup(func() {
		eventsURLOverride = originalURL
		config.Cfg.TokenPagerDuty = originalKey
	})

	return server
}

func TestTriggerEventSendsExpectedPayload(t *testing.T) {
	cases := []struct {
		name     string
		severity string
	}{
		{"warning", SeverityWarning},
		{"critical", SeverityCritical},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got event
			withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Fatalf("failed to decode request body: %v", err)
				}
				w.WriteHeader(http.StatusAccepted)
			})

			if err := TriggerEvent("example.com", "Site example.com is down", tc.severity); err != nil {
				t.Fatalf("TriggerEvent returned error: %v", err)
			}

			if got.EventAction != "trigger" {
				t.Errorf("event_action = %q, want %q", got.EventAction, "trigger")
			}
			if got.DedupKey != DedupKey("example.com") {
				t.Errorf("dedup_key = %q, want %q", got.DedupKey, DedupKey("example.com"))
			}
			if got.Payload == nil {
				t.Fatal("payload is nil, want non-nil for trigger events")
			}
			if got.Payload.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", got.Payload.Severity, tc.severity)
			}
			if got.Payload.Source != "example.com" {
				t.Errorf("source = %q, want %q", got.Payload.Source, "example.com")
			}
		})
	}
}

func TestResolveEventOmitsPayload(t *testing.T) {
	var raw map[string]interface{}
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	})

	if err := ResolveEvent("example.com"); err != nil {
		t.Fatalf("ResolveEvent returned error: %v", err)
	}

	if raw["event_action"] != "resolve" {
		t.Errorf("event_action = %v, want %q", raw["event_action"], "resolve")
	}
	if raw["dedup_key"] != DedupKey("example.com") {
		t.Errorf("dedup_key = %v, want %q", raw["dedup_key"], DedupKey("example.com"))
	}
	if _, ok := raw["payload"]; ok {
		t.Error("resolve event body should omit payload entirely")
	}
}

func TestSendEventErrorsOnNon2xx(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"invalid event","message":"Event object is invalid"}`))
	})

	err := TriggerEvent("example.com", "summary", SeverityWarning)
	if err == nil {
		t.Fatal("expected an error for a non-2xx response, got nil")
	}
}

func TestSendEventErrorsWhenNotConfigured(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	originalURL := eventsURLOverride
	originalKey := config.Cfg.TokenPagerDuty
	eventsURLOverride = server.URL
	config.Cfg.TokenPagerDuty = ""
	defer func() {
		eventsURLOverride = originalURL
		config.Cfg.TokenPagerDuty = originalKey
	}()

	if err := TriggerEvent("example.com", "summary", SeverityWarning); err == nil {
		t.Fatal("expected an error when no routing key is configured, got nil")
	}
	if called {
		t.Error("expected no HTTP call to be made when not configured")
	}
}

func TestEventsURLSelectsRegion(t *testing.T) {
	originalOverride := eventsURLOverride
	originalRegion := config.Cfg.PagerDutyEURegion
	eventsURLOverride = ""
	defer func() {
		eventsURLOverride = originalOverride
		config.Cfg.PagerDutyEURegion = originalRegion
	}()

	config.Cfg.PagerDutyEURegion = false
	if got := eventsURL(); got != usEventsURL {
		t.Errorf("eventsURL() = %q, want US endpoint %q", got, usEventsURL)
	}

	config.Cfg.PagerDutyEURegion = true
	if got := eventsURL(); got != euEventsURL {
		t.Errorf("eventsURL() = %q, want EU endpoint %q", got, euEventsURL)
	}
}

func TestDedupKeyIsStableAndCaseInsensitive(t *testing.T) {
	if DedupKey("Example.com") != DedupKey("example.com") {
		t.Errorf("DedupKey should be case-insensitive: %q != %q", DedupKey("Example.com"), DedupKey("example.com"))
	}
	if DedupKey("example.com") != "jman-monitor:example.com" {
		t.Errorf("DedupKey(%q) = %q, want %q", "example.com", DedupKey("example.com"), "jman-monitor:example.com")
	}
}
