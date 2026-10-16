package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func TestSyncSpinupWPIntoInventory(t *testing.T) {
	setupTaskRepoTest(t)

	servers := []models.Server{
		{
			ID:        101,
			Name:      "lon1.spinupwp.com",
			IPAddress: "192.0.2.1",
			SSHPort:   2222,
		},
	}

	sites := []models.Site{
		{
			ID:          201,
			ServerID:    101,
			Domain:      "mywp.example.com",
			SiteUser:    "mywpuser",
			Environment: models.SiteEnvironmentProduction,
			IsWordpress: true,
			PHPVersion:  "8.3",
			Status:      "deployed",
		},
	}

	if err := SyncSpinupWPIntoInventory(servers, sites); err != nil {
		t.Fatalf("failed to sync SpinupWP inventory: %v", err)
	}

	expectedServerUUID := utils.SpinupWPServerUUID(101)
	expectedSiteUUID := utils.SpinupWPSiteUUID(201)

	srv, err := GetManagedServer(expectedServerUUID)
	if err != nil || srv == nil {
		t.Fatalf("expected server %s, got %+v (err: %v)", expectedServerUUID, srv, err)
	}
	if srv.Name != "lon1.spinupwp.com" || srv.SSHPort != 2222 || srv.Provider != "spinupwp" {
		t.Errorf("unexpected server fields: %+v", srv)
	}

	site, err := GetManagedSite(expectedSiteUUID)
	if err != nil || site == nil {
		t.Fatalf("expected site %s, got %+v (err: %v)", expectedSiteUUID, site, err)
	}
	if site.Domain != "mywp.example.com" || site.SSHHost != "lon1.spinupwp.com" || site.SSHPort != 2222 || site.SSHUser != "mywpuser" {
		t.Errorf("unexpected site fields: %+v", site)
	}
	if site.ServerID == nil || *site.ServerID != expectedServerUUID {
		t.Errorf("expected server_id %s, got %v", expectedServerUUID, site.ServerID)
	}
}

func TestSyncMarksRemovedSpinupWPSites(t *testing.T) {
	setupTaskRepoTest(t)

	servers := []models.Server{{ID: 101, Name: "lon1.spinupwp.com"}}
	site := func(id int, domain string) models.Site {
		return models.Site{ID: id, ServerID: 101, Domain: domain, Status: "deployed"}
	}
	status := func(id int) string {
		t.Helper()
		ms, err := GetManagedSite(utils.SpinupWPSiteUUID(id))
		if err != nil || ms == nil {
			t.Fatalf("site %d: %+v, %v", id, ms, err)
		}
		return ms.Status
	}
	manual := models.ManagedSite{ID: "manual-1", Provider: "manual", Domain: "manual.example", Status: "active"}
	if err := SaveManagedSite(manual); err != nil {
		t.Fatal(err)
	}

	if err := SyncSpinupWPIntoInventory(servers, []models.Site{site(1, "a.example"), site(2, "b.example")}); err != nil {
		t.Fatal(err)
	}

	// b.example is gone from SpinupWP.
	if err := SyncSpinupWPIntoInventory(servers, []models.Site{site(1, "a.example")}); err != nil {
		t.Fatal(err)
	}
	if s := status(1); s != "deployed" {
		t.Errorf("listed site status = %q", s)
	}
	if s := status(2); s != SiteStatusRemoved {
		t.Errorf("unlisted site status = %q, want removed", s)
	}
	if ms, _ := GetManagedSite("manual-1"); ms == nil || ms.Status != "active" {
		t.Errorf("manual site touched: %+v", ms)
	}

	// An empty list (a failed fetch) removes nothing.
	if err := SyncSpinupWPIntoInventory(servers, nil); err != nil {
		t.Fatal(err)
	}
	if s := status(1); s != "deployed" {
		t.Errorf("empty list marked a site: %q", s)
	}

	// A site SpinupWP lists again gets its status back.
	if err := SyncSpinupWPIntoInventory(servers, []models.Site{site(1, "a.example"), site(2, "b.example")}); err != nil {
		t.Fatal(err)
	}
	if s := status(2); s != "deployed" {
		t.Errorf("relisted site status = %q", s)
	}
}
