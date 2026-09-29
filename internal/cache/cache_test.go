package cache

import (
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func setupCacheTest(t *testing.T) {
	t.Helper()

	cacheDir, err := os.MkdirTemp("", "jman-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp cache dir: %v", err)
	}
	dataDir, err := os.MkdirTemp("", "jman-data-test-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}

	oldCacheDir := config.RunData.CacheDir
	oldDataDir := config.RunData.DataDir
	config.RunData.CacheDir = cacheDir
	config.RunData.DataDir = dataDir

	t.Cleanup(func() {
		os.RemoveAll(cacheDir)
		os.RemoveAll(dataDir)
		config.RunData.CacheDir = oldCacheDir
		config.RunData.DataDir = oldDataDir
	})
}

func TestWriteJSONCacheRejectsPathTraversal(t *testing.T) {
	setupCacheTest(t)

	err := WriteJSONCache("../../etc/evil", map[string]string{"pwned": "true"})
	if err == nil {
		t.Fatal("expected error for path-traversing filename, got nil")
	}

	// Confirm nothing was written outside the cache dir.
	if _, statErr := os.Stat("/etc/evil.json"); !os.IsNotExist(statErr) {
		t.Fatalf("traversal write should not have succeeded, stat err: %v", statErr)
	}
}

func TestReadJSONCacheRejectsPathTraversal(t *testing.T) {
	setupCacheTest(t)

	var dest map[string]string
	err := ReadJSONCache("../../etc/passwd", &dest, DefaultTTL)
	if err == nil {
		t.Fatal("expected error for path-traversing filename, got nil")
	}
}

func TestWriteJSONCacheAllowsNormalNestedFilename(t *testing.T) {
	setupCacheTest(t)

	if err := WriteJSONCache("vulnerabilities/some-plugin", map[string]string{"ok": "true"}); err != nil {
		t.Fatalf("expected normal nested filename to succeed, got: %v", err)
	}
}

func TestWriteJSONCacheAllowsCoreVersionNestedFilename(t *testing.T) {
	setupCacheTest(t)

	if err := WriteJSONCache("vulnerabilities/core/6.6.1", map[string]string{"ok": "true"}); err != nil {
		t.Fatalf("expected core version nested filename to succeed, got: %v", err)
	}
}

func TestGetSitesForServer_IncludesNonWPSites(t *testing.T) {
	setupCacheTest(t)
	if err := db.InitInventory(); err != nil {
		t.Fatalf("failed to init inventory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	server10 := utils.SpinupWPServerUUID(10)
	server20 := utils.SpinupWPServerUUID(20)
	for _, srv := range []string{server10, server20} {
		if err := db.SaveManagedServer(models.ManagedServer{ID: srv, Name: srv, Provider: "spinupwp", SSHPort: 22}); err != nil {
			t.Fatalf("failed to seed server: %v", err)
		}
	}
	sites := []models.ManagedSite{
		{ID: utils.SpinupWPSiteUUID(1), ServerID: &server10, Provider: "spinupwp", Domain: "wp.example.com", IsWordpress: true, Status: "deployed", Environment: models.SiteEnvironmentProduction},
		{ID: utils.SpinupWPSiteUUID(2), ServerID: &server10, Provider: "spinupwp", Domain: "nonwp.example.com", IsWordpress: false, Status: "deployed", Environment: models.SiteEnvironmentProduction},
		{ID: utils.SpinupWPSiteUUID(3), ServerID: &server20, Provider: "spinupwp", Domain: "other.example.com", IsWordpress: true, Status: "deployed", Environment: models.SiteEnvironmentProduction},
	}
	for _, site := range sites {
		if err := db.SaveManagedSite(site); err != nil {
			t.Fatalf("failed to seed site: %v", err)
		}
	}

	got, err := GetSitesForServer(server10)
	if err != nil {
		t.Fatalf("GetSitesForServer error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 sites for server 10, got %d", len(got))
	}
	byDomain := map[string]models.ManagedSite{}
	for _, s := range got {
		byDomain[s.Domain] = s
	}
	if s, ok := byDomain["wp.example.com"]; !ok || !s.IsWordpress {
		t.Errorf("missing or wrong WordPress site: %+v", got)
	}
	if s, ok := byDomain["nonwp.example.com"]; !ok || s.IsWordpress {
		t.Errorf("missing or wrong non-WordPress site: %+v", got)
	}
}
