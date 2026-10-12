package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

// manifestServer serves a manifest and fails the test on any report.
func manifestServer(t *testing.T, manifest models.AgentManifest) Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/manifest" {
			t.Errorf("--check-wp called %s %s; it must not report", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
			return
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	t.Cleanup(srv.Close)
	return Config{APIURL: srv.URL, Token: "1.x"}
}

func TestCheckWPData(t *testing.T) {
	runner := okRunner()
	runner.fail = map[string]error{}
	stubSite(t, runner)
	// Against an older jman-api: no interval in the manifest.
	cfg := manifestServer(t, models.AgentManifest{Sites: []models.AgentManifestSite{
		{SiteID: "a", Domain: "a.example.com", IsWordpress: true},
		{SiteID: "b", Domain: "b.example.com", IsWordpress: true},
		{SiteID: "c", Domain: "static.example.com", IsWordpress: false},
	}})
	resolveIdentity = func(path, _ string) (siteIdentity, error) {
		if strings.Contains(path, "b.example.com") {
			return siteIdentity{}, errors.New("refusing to run wp-cli: /sites/b.example.com/files is owned by root")
		}
		return siteIdentity{UID: 1001, GID: 1001, Username: "alice", Home: "/sites/a.example.com"}, nil
	}

	var out bytes.Buffer
	failed, err := CheckWPData(context.Background(), cfg, "", &out)
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
	for _, want := range []string{
		"doesn't enable agent collection yet",
		"a.example.com\n  path:   /sites/a.example.com/files\n  runs as: alice (uid 1001, gid 1001",
		"ok: 2 plugins (2 regular, 1 with updates), WordPress 6.6.1, minor update 6.6.2, major update 6.7.1",
		"b.example.com\n  path:   /sites/b.example.com/files\n  FAILED: refusing to run wp-cli: /sites/b.example.com/files is owned by root",
		"1 of 2 site(s) collected, 1 failed",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "static.example.com") {
		t.Error("checked a non-WordPress site")
	}

	// --site limits it to one domain.
	out.Reset()
	if failed, err := CheckWPData(context.Background(), cfg, "A.example.com", &out); err != nil || failed != 0 || strings.Contains(out.String(), "b.example.com") {
		t.Errorf("filtered check: failed=%d err=%v\n%s", failed, err, out.String())
	}
	if _, err := CheckWPData(context.Background(), cfg, "missing.example.com", &out); err == nil {
		t.Error("no error for a domain not in the manifest")
	}
}
