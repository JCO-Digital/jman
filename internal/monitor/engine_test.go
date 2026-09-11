package monitor

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
)

func setupEngineTest(t *testing.T) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "jman-monitor-engine-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = tempDir

	if err := db.InitInventory(); err != nil {
		t.Fatalf("failed to init inventory DB: %v", err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatalf("failed to init api DB: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tempDir)
		config.RunData.DataDir = oldDataDir
	})
}

// redirectTransport rewrites every outgoing request to point at a test
// server, regardless of the scheme/host CheckSite hardcodes.
type redirectTransport struct {
	target *url.URL
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	req.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newTestEngine(t *testing.T, handler http.HandlerFunc) *Engine {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	return &Engine{
		client: &http.Client{Transport: &redirectTransport{target: target}},
	}
}

func lastHistoryStatus(t *testing.T, domain string) string {
	t.Helper()
	var status string
	err := db.GetAPIDB().QueryRow(
		"SELECT status FROM monitor_history WHERE domain = ? ORDER BY id DESC LIMIT 1", domain,
	).Scan(&status)
	if err != nil {
		t.Fatalf("failed to read monitor_history for %s: %v", domain, err)
	}
	return status
}

func TestCheckSiteMarksEmptyBodyAsDown(t *testing.T) {
	setupEngineTest(t)

	engine := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	status := &SiteStatus{Domain: "example.com", CurrentMode: ModeNormal}
	if err := engine.CheckSite(status); err != nil {
		t.Fatalf("CheckSite returned error: %v", err)
	}

	if status.CurrentMode != ModeInvestigation {
		t.Errorf("expected mode %q for a blank 200 response, got %q", ModeInvestigation, status.CurrentMode)
	}
	if status.FailureCount != 1 {
		t.Errorf("expected FailureCount 1, got %d", status.FailureCount)
	}
	if got := lastHistoryStatus(t, "example.com"); got == "UP" {
		t.Errorf("expected history status other than UP for a blank response, got %q", got)
	}
}

func TestCheckSiteMarksNonHTMLBodyAsDown(t *testing.T) {
	setupEngineTest(t)

	engine := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"upstream misconfigured"}`))
	})

	status := &SiteStatus{Domain: "example.com", CurrentMode: ModeNormal}
	if err := engine.CheckSite(status); err != nil {
		t.Fatalf("CheckSite returned error: %v", err)
	}

	if status.CurrentMode != ModeInvestigation {
		t.Errorf("expected mode %q for a non-HTML 200 response, got %q", ModeInvestigation, status.CurrentMode)
	}
}

func TestCheckSiteMarksValidHTMLAsUp(t *testing.T) {
	setupEngineTest(t)

	engine := newTestEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><head><title>Home</title></head><body>Hi</body></html>"))
	})

	status := &SiteStatus{Domain: "example.com", CurrentMode: ModeNormal}
	if err := engine.CheckSite(status); err != nil {
		t.Fatalf("CheckSite returned error: %v", err)
	}

	if status.CurrentMode != ModeNormal {
		t.Errorf("expected mode to stay %q for a healthy response, got %q", ModeNormal, status.CurrentMode)
	}
	if status.FailureCount != 0 {
		t.Errorf("expected FailureCount 0, got %d", status.FailureCount)
	}
	if status.ConsecutiveSuccesses != 1 {
		t.Errorf("expected ConsecutiveSuccesses 1, got %d", status.ConsecutiveSuccesses)
	}
	if got := lastHistoryStatus(t, "example.com"); got != "UP" {
		t.Errorf("expected history status UP, got %q", got)
	}
}

func TestLooksLikeHTML(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"empty", "", false},
		{"whitespace only", "   \n\t  ", false},
		{"doctype", "<!DOCTYPE html><html><head></head><body>Hi</body></html>", true},
		{"html tag without doctype", "<html><head></head><body>Hi</body></html>", true},
		{"uppercase markup", "<HTML><BODY>Hi</BODY></HTML>", true},
		{"plain text", "OK", false},
		{"json body", `{"status":"ok"}`, false},
		{"leading whitespace before doctype", "\n\n<!doctype html><html></html>", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeHTML([]byte(tc.body)); got != tc.want {
				t.Errorf("looksLikeHTML(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
