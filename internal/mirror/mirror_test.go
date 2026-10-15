package mirror

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/client"
	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/credentials"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/zalando/go-keyring"
)

// fakeAPI serves a snapshot with an ETag. status, when non-zero, replaces
// every response with that status.
type fakeAPI struct {
	srv      *httptest.Server
	hits     atomic.Int32
	snapshot cliproto.Snapshot
	status   atomic.Int32
	min      atomic.Value // string
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{snapshot: cliproto.Snapshot{
		SchemaVersion: 1,
		Sites: []cliproto.Site{
			{ID: "a", Provider: "manual", Domain: "a.example.com", IsWordpress: true, CanWPCLI: true, SSHHost: "host", SSHPort: 2222, SSHUser: "u", SitePath: "public",
				UpdateLocks: []models.UpdateLock{{SiteID: "a", Plugin: "woocommerce"}}},
			{ID: "b", Provider: "spinupwp", Domain: "b.example.com", IsWordpress: true, CanWPCLI: true, SSHHost: "srv.example.com", SSHPort: 22, SSHUser: "b", ServerName: "srv.example.com", SitePath: "files",
				UpdateLocks: []models.UpdateLock{{SiteID: "b", Plugin: ""}}},
			{ID: "c", Provider: "manual", Domain: "c.example.com", IsWordpress: true, CanWPCLI: false},
		},
		PluginInfo: []cliproto.PluginInfo{{Slug: "akismet", Name: "Akismet Anti-spam"}, {Slug: "woocommerce", Name: "WooCommerce"}},
	}}
	f.min.Store("")
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		w.Header().Set(cliproto.LatestCLIVersionHeader, "6.20.0")
		if min := f.min.Load().(string); min != "" {
			w.Header().Set(cliproto.MinCLIVersionHeader, min)
		}
		if s := f.status.Load(); s != 0 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"error":"nope"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer jman_cli_test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := json.Marshal(f.snapshot)
		etag := `"` + string(rune('a'+len(body)%26)) + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func setup(t *testing.T, apiURL string) {
	t.Helper()
	oldRun, oldCfg, oldVersion := config.RunData, config.Cfg, config.AppVersion
	dir := t.TempDir()
	config.RunData.DataDir = dir
	config.RunData.ConfigDir = dir
	config.Cfg.APIURL = apiURL
	config.Cfg.CacheTTL = time.Minute
	config.Cfg.APITimeout = time.Second
	config.Cfg.OfflineBackoff = 15 * time.Minute
	config.Cfg.StaleWarning = 7 * 24 * time.Hour
	config.AppVersion = "6.12.0"
	keyring.MockInit()
	if _, err := credentials.Set(apiURL, "jman_cli_test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		config.RunData, config.Cfg, config.AppVersion = oldRun, oldCfg, oldVersion
	})
}

// age makes the stored copy look older by d.
func age(t *testing.T, d time.Duration) {
	t.Helper()
	m, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	m.Meta.FetchedAt = m.Meta.FetchedAt.Add(-d)
	m.Meta.LastAttemptAt = m.Meta.LastAttemptAt.Add(-d)
	if err := m.saveMeta(); err != nil {
		t.Fatal(err)
	}
}

func TestGetFetchesAndRespectsTTL(t *testing.T) {
	api := newFakeAPI(t)
	setup(t, api.srv.URL)
	ctx := context.Background()

	m, err := Get(ctx, Options{})
	if err != nil || m.Snapshot == nil || len(m.Sites()) != 3 {
		t.Fatalf("first Get: %v %+v", err, m)
	}
	if m.Meta.LatestCLIVersion != "6.20.0" {
		t.Errorf("latest version not recorded: %+v", m.Meta)
	}

	if _, err := Get(ctx, Options{}); err != nil || api.hits.Load() != 1 {
		t.Fatalf("Get within the TTL contacted the API (%d hits, %v)", api.hits.Load(), err)
	}

	age(t, 2*time.Minute)
	before, _ := Load()
	m, err = Get(ctx, Options{})
	if err != nil || api.hits.Load() != 2 {
		t.Fatalf("Get after the TTL: %d hits, %v", api.hits.Load(), err)
	}
	if !m.Meta.FetchedAt.After(before.Meta.FetchedAt) || len(m.Sites()) != 3 {
		t.Errorf("304 should keep the data and bump fetched_at: %+v", m.Meta)
	}

	if _, err := Get(ctx, Options{Force: true}); err != nil || api.hits.Load() != 3 {
		t.Errorf("forced Get: %d hits, %v", api.hits.Load(), err)
	}
}

func TestGetFallsBackWhenAPIIsDown(t *testing.T) {
	api := newFakeAPI(t)
	setup(t, api.srv.URL)
	ctx := context.Background()

	if _, err := Get(ctx, Options{}); err != nil {
		t.Fatal(err)
	}
	age(t, 2*time.Minute)
	api.status.Store(http.StatusBadGateway)

	m, err := Get(ctx, Options{})
	if err != nil || len(m.Sites()) != 3 {
		t.Fatalf("expected the local copy, got %v", err)
	}
	if m.Meta.LastError == "" {
		t.Error("expected the failure to be recorded")
	}

	// Within the backoff, the API isn't contacted again.
	hits := api.hits.Load()
	if _, err := Get(ctx, Options{}); err != nil || api.hits.Load() != hits {
		t.Errorf("Get during the backoff contacted the API (%v)", err)
	}

	// A forced refresh fails instead of falling back.
	if _, err := Get(ctx, Options{Force: true}); err == nil {
		t.Error("expected a forced Get to fail while the API is down")
	}

	// Once it's back, the next check after the backoff recovers.
	api.status.Store(0)
	age(t, 20*time.Minute)
	if m, err := Get(ctx, Options{}); err != nil || m.Meta.LastError != "" {
		t.Errorf("expected recovery, got %v %+v", err, m.Meta)
	}
}

func TestGetFailsWithoutLocalCopy(t *testing.T) {
	api := newFakeAPI(t)
	setup(t, api.srv.URL)
	api.status.Store(http.StatusBadGateway)
	if _, err := Get(context.Background(), Options{}); err == nil {
		t.Fatal("expected an error with no local copy and the API down")
	}

	config.Cfg.APIURL = ""
	if _, err := Get(context.Background(), Options{}); !errors.Is(err, ErrNoAPIURL) {
		t.Errorf("expected ErrNoAPIURL, got %v", err)
	}
}

func TestGetRejectedLogin(t *testing.T) {
	api := newFakeAPI(t)
	setup(t, api.srv.URL)
	if _, err := Get(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	age(t, 2*time.Minute)

	// A revoked token doesn't fall back to the local copy.
	api.status.Store(http.StatusUnauthorized)
	if _, err := Get(context.Background(), Options{}); !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}

	_ = credentials.Delete(api.srv.URL)
	if _, err := Get(context.Background(), Options{Force: true}); !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("without a token: expected ErrUnauthorized, got %v", err)
	}
}

func TestMinimumVersion(t *testing.T) {
	api := newFakeAPI(t)
	setup(t, api.srv.URL)
	if _, err := Get(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	if err := CheckVersion(); err != nil {
		t.Fatalf("no minimum yet: %v", err)
	}

	api.min.Store("6.13.0")
	var upgrade *client.UpgradeRequiredError
	if _, err := Get(context.Background(), Options{Force: true}); !errors.As(err, &upgrade) {
		t.Fatalf("expected an upgrade error, got %v", err)
	}
	// The minimum is remembered, so it applies offline too.
	if err := CheckVersion(); !errors.As(err, &upgrade) {
		t.Errorf("CheckVersion offline: %v", err)
	}

	config.AppVersion = "dev"
	if err := CheckVersion(); err != nil {
		t.Errorf("development builds aren't blocked: %v", err)
	}
}

func TestAPIURLChangeDiscardsCopy(t *testing.T) {
	api := newFakeAPI(t)
	setup(t, api.srv.URL)
	if _, err := Get(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}

	config.Cfg.APIURL = "http://127.0.0.1:1" // nothing listens here
	if _, err := credentials.Set(config.Cfg.APIURL, "jman_cli_other"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(context.Background(), Options{}); err == nil {
		t.Fatal("expected an error: the new API is down and its copy is empty")
	}
	if _, err := os.Stat(snapshotPath()); !os.IsNotExist(err) {
		t.Error("the other API's snapshot was kept")
	}
}

func TestLookups(t *testing.T) {
	api := newFakeAPI(t)
	setup(t, api.srv.URL)
	m, err := Get(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}

	sites := m.CliSites()
	if len(sites) != 2 {
		t.Fatalf("expected only WP-CLI sites, got %+v", sites)
	}
	if sites[0].SSH != "u@host:2222" || sites[0].Name != "a.example.com" {
		t.Errorf("manual site: %+v", sites[0])
	}
	if sites[1].ServerName != "srv" {
		t.Errorf("SpinupWP server names are shortened like before: %+v", sites[1])
	}

	if m.PluginName("woocommerce") != "WooCommerce" || m.PluginName("unknown") != "unknown" {
		t.Error("PluginName")
	}
	if !m.IsUpdateLocked("a", "woocommerce") || m.IsUpdateLocked("a", "akismet") {
		t.Error("plugin lock on site a")
	}
	if !m.IsUpdateLocked("b", "anything") || m.IsUpdateLocked("c", "anything") {
		t.Error("site lock on site b")
	}
}
