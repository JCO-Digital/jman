package api

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func seedSnapshotSites(t *testing.T) {
	t.Helper()
	oldCacheDir := config.RunData.CacheDir
	config.RunData.CacheDir = config.RunData.DataDir
	t.Cleanup(func() { config.RunData.CacheDir = oldCacheDir })

	sites := []models.ManagedSite{
		// A manual site that overrides the SpinupWP site for the same domain.
		{ID: "manual-1", Provider: "manual", Domain: "Shop.example.com", ConnectionType: "ssh", SSHHost: "shop.host", SSHPort: 2222, SSHUser: "shop", SitePath: "public", CanWPCLI: true, IsWordpress: true, Status: "active"},
		{ID: "spinup-1", Provider: "spinupwp", ProviderSiteID: "1", Domain: "shop.example.com", ConnectionType: "agent", SSHHost: "srv1", SSHPort: 22, SSHUser: "old", SitePath: "files", CanWPCLI: true, IsWordpress: true, Status: "active"},
		{ID: "spinup-2", Provider: "spinupwp", ProviderSiteID: "2", Domain: "blog.example.com", ConnectionType: "agent", SSHHost: "srv1", SSHPort: 22, SSHUser: "blog", SitePath: "files", CanWPCLI: true, IsWordpress: true, Status: "active"},
	}
	for _, s := range sites {
		if err := db.SaveManagedSite(s); err != nil {
			t.Fatalf("seed site %s: %v", s.ID, err)
		}
	}
	now := time.Now().UTC()
	for _, id := range []string{"manual-1", "spinup-1", "spinup-2"} {
		if err := db.ReplaceSitePlugins(id, []models.WPPlugin{{Name: "akismet", Status: "active", Version: "5.0"}}, now); err != nil {
			t.Fatalf("seed plugins: %v", err)
		}
	}
	if err := db.SaveSiteCore("spinup-2", "6.8.3", "", ""); err != nil {
		t.Fatalf("seed core: %v", err)
	}
}

func getSnapshot(t *testing.T, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/cli/snapshot", nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	CLISnapshotHandler(w, req)
	return w
}

func TestCLISnapshotHandler(t *testing.T) {
	setupSiteManagementTest(t)
	seedSnapshotSites(t)

	w := getSnapshot(t, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var snap cliproto.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.SchemaVersion != cliproto.SnapshotSchemaVersion {
		t.Errorf("schema_version = %d", snap.SchemaVersion)
	}

	ids := map[string]cliproto.Site{}
	for _, s := range snap.Sites {
		ids[s.ID] = s
	}
	if _, ok := ids["spinup-1"]; ok || len(snap.Sites) != 2 {
		t.Fatalf("expected the manual site to replace the SpinupWP one, got %+v", snap.Sites)
	}
	if s := ids["manual-1"]; s.SSH != "shop@shop.host:2222" {
		t.Errorf("manual site SSH target = %q", s.SSH)
	}
	if s := ids["spinup-2"]; s.WPCore == nil || s.WPCore.Version != "6.8.3" {
		t.Errorf("expected the core version on spinup-2, got %+v", s.WPCore)
	}
	for _, p := range snap.Plugins {
		if p.SiteID == "spinup-1" {
			t.Errorf("plugins of an overridden site are included: %+v", p)
		}
	}
	if len(snap.Plugins) != 2 {
		t.Errorf("expected 2 plugin rows, got %+v", snap.Plugins)
	}

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	t.Run("unchanged data is a 304", func(t *testing.T) {
		w := getSnapshot(t, map[string]string{"If-None-Match": etag})
		if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Errorf("expected an empty 304, got %d (%d bytes)", w.Code, w.Body.Len())
		}
	})

	t.Run("changed data gets a new ETag", func(t *testing.T) {
		if err := db.SaveSiteCore("spinup-2", "6.8.4", "", ""); err != nil {
			t.Fatal(err)
		}
		w := getSnapshot(t, map[string]string{"If-None-Match": etag})
		if w.Code != http.StatusOK || w.Header().Get("ETag") == etag {
			t.Errorf("expected 200 with a new ETag, got %d %s", w.Code, w.Header().Get("ETag"))
		}
	})

	t.Run("gzip", func(t *testing.T) {
		w := getSnapshot(t, map[string]string{"Accept-Encoding": "gzip"})
		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Fatal("expected a gzipped response")
		}
		gz, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(gz)
		var snap cliproto.Snapshot
		if err := json.Unmarshal(body, &snap); err != nil || len(snap.Sites) != 2 {
			t.Errorf("gzipped body: %v %+v", err, snap.Sites)
		}
	})

	t.Run("SpinupWP sites missing from the SpinupWP cache are dropped", func(t *testing.T) {
		// The SpinupWP list knows only site 2; spinup-1 was deleted there.
		if err := cache.WriteJSONCache("sites", []models.Site{{ID: 2, Domain: "blog.example.com", IsWordpress: true}}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(config.RunData.CacheDir, "sites.json")) })
		if err := db.SaveManagedSite(models.ManagedSite{ID: utils.SpinupWPSiteUUID(3), Provider: "spinupwp", ProviderSiteID: "3", Domain: "gone.example.com", Status: "active"}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveManagedSite(models.ManagedSite{ID: utils.SpinupWPSiteUUID(2), Provider: "spinupwp", ProviderSiteID: "2", Domain: "blog.example.com", Status: "active"}); err != nil {
			t.Fatal(err)
		}

		snap, err := buildCLISnapshot()
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, s := range snap.Sites {
			ids[s.ID] = true
		}
		if ids[utils.SpinupWPSiteUUID(3)] || !ids[utils.SpinupWPSiteUUID(2)] || !ids["manual-1"] {
			t.Errorf("unexpected sites: %+v", snap.Sites)
		}
	})
}

func TestVersionMiddlewareCLIHeaders(t *testing.T) {
	old := config.Cfg
	t.Cleanup(func() { config.Cfg = old })

	serve := func() http.Header {
		h := VersionMiddleware("6.12.0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/cli/snapshot", nil))
		return w.Header()
	}

	config.Cfg.MinCLIVersion = ""
	config.Cfg.LatestCLIVersion = ""
	h := serve()
	if h.Get(cliproto.MinCLIVersionHeader) != "" || h.Get(cliproto.LatestCLIVersionHeader) != "6.12.0" {
		t.Errorf("defaults: min=%q latest=%q", h.Get(cliproto.MinCLIVersionHeader), h.Get(cliproto.LatestCLIVersionHeader))
	}

	config.Cfg.MinCLIVersion = "6.12.0"
	config.Cfg.LatestCLIVersion = "6.13.0"
	h = serve()
	if h.Get(cliproto.MinCLIVersionHeader) != "6.12.0" || h.Get(cliproto.LatestCLIVersionHeader) != "6.13.0" {
		t.Errorf("configured: min=%q latest=%q", h.Get(cliproto.MinCLIVersionHeader), h.Get(cliproto.LatestCLIVersionHeader))
	}
}
