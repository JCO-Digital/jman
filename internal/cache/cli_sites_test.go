package cache

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func TestBuildCliSites(t *testing.T) {
	setupCacheTest(t)
	if err := db.InitInventory(); err != nil {
		t.Fatalf("failed to init inventory DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	servers := []models.Server{
		{ID: 10, Name: "web1.example.net", SSHPort: 22},
		{ID: 20, Name: "web2.example.net", SSHPort: 2222},
	}
	sites := []models.Site{
		{ID: 1, ServerID: 10, Domain: "b.example.com", SiteUser: "bee", IsWordpress: true},
		{ID: 2, ServerID: 20, Domain: "a.example.com", SiteUser: "ay", IsWordpress: true},
		{ID: 3, ServerID: 10, Domain: "static.example.com", SiteUser: "st", IsWordpress: false},
		{ID: 4, ServerID: 99, Domain: "orphan.example.com", SiteUser: "or", IsWordpress: true}, // server not cached
	}

	// A SpinupWP site deleted upstream: still in inventory, gone from the cache.
	server10 := utils.SpinupWPServerUUID(10)
	if err := db.SaveManagedSite(models.ManagedSite{
		ID: utils.SpinupWPSiteUUID(5), ServerID: &server10, Provider: "spinupwp", ProviderSiteID: "5",
		Domain: "deleted.example.com", IsWordpress: true, CanWPCLI: true, SSHHost: "web1.example.net", SSHUser: "del", SitePath: "files",
	}); err != nil {
		t.Fatal(err)
	}
	// An external site, plus one with WP-CLI disabled.
	for _, ms := range []models.ManagedSite{
		{ID: "11111111-1111-1111-1111-111111111111", Provider: "wpengine", Domain: "ext.example.org", IsWordpress: true, CanWPCLI: true, SSHHost: "ext.ssh.wpengine.net", SSHUser: "ext", SitePath: "sites/ext"},
		{ID: "22222222-2222-2222-2222-222222222222", Provider: "manual", Domain: "nocli.example.org", IsWordpress: true, CanWPCLI: false, SSHHost: "h"},
	} {
		if err := db.SaveManagedSite(ms); err != nil {
			t.Fatal(err)
		}
	}

	// Inventory has never synced the cached SpinupWP sites; building the
	// list must sync them first.
	got, err := buildCliSites(sites, servers)
	if err != nil {
		t.Fatalf("buildCliSites: %v", err)
	}

	want := []models.CliSite{
		{ID: utils.SpinupWPSiteUUID(1), ProviderSiteID: 1, Provider: "spinupwp", ServerID: server10, Name: "b.example.com", ServerName: "web1", SSH: "bee@web1.example.net", Path: "files"},
		{ID: utils.SpinupWPSiteUUID(2), ProviderSiteID: 2, Provider: "spinupwp", ServerID: utils.SpinupWPServerUUID(20), Name: "a.example.com", ServerName: "web2", SSH: "ay@web2.example.net:2222", Path: "files"},
		{ID: "11111111-1111-1111-1111-111111111111", Provider: "wpengine", Name: "ext.example.org", SSH: "ext@ext.ssh.wpengine.net", Path: "sites/ext"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sites %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("site %d:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}
