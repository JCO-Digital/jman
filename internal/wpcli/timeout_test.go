package wpcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

// fakeWP puts a fake wp executable first on PATH. It answers by matching
// the full argument list against the given shell case patterns.
func fakeWP(t *testing.T, cases map[string]string) {
	t.Helper()
	dir := t.TempDir()
	var body strings.Builder
	body.WriteString("#!/bin/sh\ncase \"$*\" in\n")
	for pattern, action := range cases {
		body.WriteString("  " + pattern + ") " + action + " ;;\n")
	}
	body.WriteString("  *) echo \"unexpected: $*\" >&2; exit 99 ;;\nesac\n")
	if err := os.WriteFile(filepath.Join(dir, "wp"), []byte(body.String()), 0o755); err != nil {
		t.Fatalf("failed to write fake wp: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestEffectiveTimeout(t *testing.T) {
	old := config.Cfg.WPCLIHostTimeouts
	t.Cleanup(func() { config.Cfg.WPCLIHostTimeouts = old })
	config.Cfg.WPCLIHostTimeouts = []config.WPCLIHostTimeout{
		{Host: "*.ssh.wpengine.net", Minutes: 5},
		{Host: "slow.example.com", Minutes: 20},
	}

	cases := []struct {
		name string
		opts CliOptions
		want time.Duration
	}{
		{"default", CliOptions{SSH: "u@fast.example.com"}, DefaultTimeout},
		{"local call", CliOptions{}, DefaultTimeout},
		{"host glob raises a read", CliOptions{SSH: "acme@acme.ssh.wpengine.net"}, 5 * time.Minute},
		{"port is ignored when matching", CliOptions{SSH: "acme@acme.ssh.wpengine.net:2222"}, 5 * time.Minute},
		{"host minimum never lowers a write", CliOptions{SSH: "acme@acme.ssh.wpengine.net", Timeout: WriteTimeout}, WriteTimeout},
		{"host minimum raises a write", CliOptions{SSH: "u@slow.example.com", Timeout: WriteTimeout}, 20 * time.Minute},
	}
	for _, c := range cases {
		if got := effectiveTimeout(c.opts); got != c.want {
			t.Errorf("%s: effectiveTimeout = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestRunWPReportsTimeout(t *testing.T) {
	fakeWP(t, map[string]string{"*": "sleep 30"})

	start := time.Now()
	_, err := RunWP(CliOptions{Timeout: 200 * time.Millisecond}, "plugin", "list")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("RunWP took %s, timeout not enforced", elapsed)
	}
}

func TestUpdatePluginChecksSiteAfterFailure(t *testing.T) {
	fakeWP(t, map[string]string{
		"*eval*":                         "exit 0",
		"*user\\ list*":                  "echo '[]'",
		"*plugin\\ update*":              "echo 'Error: Download failed.' >&2; exit 1",
		"*plugin\\ list*":                `echo '[{"name":"akismet","status":"active","update":"available","version":"5.1"},{"name":"other","status":"active","update":"none","version":"1.0"}]'`,
		"*maintenance-mode\\ is-active*": "exit 0",
	})
	site := models.CliSite{ID: "test-site", Name: "example.com", Path: "/srv/www"}

	_, err := UpdatePlugin(site, []string{"akismet"})
	var failure *UpdateFailure
	if !errors.As(err, &failure) {
		t.Fatalf("err = %v (%T), want *UpdateFailure", err, err)
	}
	if got := failure.Versions["akismet"]; got != "5.1" {
		t.Errorf("Versions[akismet] = %q, want 5.1", got)
	}
	if _, ok := failure.Versions["other"]; ok {
		t.Errorf("Versions includes a plugin that wasn't requested: %v", failure.Versions)
	}
	if !failure.MaintenanceMode {
		t.Errorf("MaintenanceMode = false, want true")
	}
	if !strings.Contains(err.Error(), "maintenance mode") || !strings.Contains(err.Error(), "Download failed") {
		t.Errorf("error message missing details: %v", err)
	}
}

func TestMaintenanceModeInactive(t *testing.T) {
	fakeWP(t, map[string]string{"*maintenance-mode\\ is-active*": "exit 1"})
	if maintenanceModeActive(models.CliSite{Path: "/srv/www"}) {
		t.Errorf("maintenanceModeActive = true for a site that isn't in maintenance mode")
	}
}
