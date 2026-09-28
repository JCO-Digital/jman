package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func TestManagedServerCRUD(t *testing.T) {
	setupTaskRepoTest(t)

	serverID := utils.NewV7UUID()
	server := models.ManagedServer{
		ID:               serverID,
		Provider:         "manual",
		ProviderServerID: "",
		Name:             "WPEngine",
		IsLogical:        true,
		IPAddress:        "",
		SSHPort:          22,
	}

	if err := SaveManagedServer(server); err != nil {
		t.Fatalf("failed to save server: %v", err)
	}

	fetched, err := GetManagedServer(serverID)
	if err != nil {
		t.Fatalf("failed to get server: %v", err)
	}
	if fetched == nil {
		t.Fatalf("expected server, got nil")
	}
	if fetched.Name != "WPEngine" || !fetched.IsLogical {
		t.Errorf("unexpected server data: %+v", fetched)
	}

	// Update server
	server.Name = "WPEngine Managed"
	if err := SaveManagedServer(server); err != nil {
		t.Fatalf("failed to update server: %v", err)
	}
	fetched, err = GetManagedServer(serverID)
	if err != nil || fetched.Name != "WPEngine Managed" {
		t.Fatalf("expected updated name, got %v (err: %v)", fetched, err)
	}

	// List
	list, err := ListManagedServers()
	if err != nil {
		t.Fatalf("failed to list servers: %v", err)
	}
	if len(list) != 1 || list[0].ID != serverID {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Delete
	if err := DeleteManagedServer(serverID); err != nil {
		t.Fatalf("failed to delete server: %v", err)
	}
	fetched, err = GetManagedServer(serverID)
	if err != nil {
		t.Fatalf("unexpected error getting deleted server: %v", err)
	}
	if fetched != nil {
		t.Fatalf("expected nil for deleted server, got %+v", fetched)
	}
}

func TestManagedSiteCRUD(t *testing.T) {
	setupTaskRepoTest(t)

	serverID := utils.NewV7UUID()
	server := models.ManagedServer{
		ID:        serverID,
		Provider:  "wpengine",
		Name:      "WPEngine",
		IsLogical: true,
	}
	if err := SaveManagedServer(server); err != nil {
		t.Fatalf("failed to save server: %v", err)
	}

	siteID := utils.NewV7UUID()
	site := models.ManagedSite{
		ID:             siteID,
		ServerID:       &serverID,
		Provider:       "wpengine",
		Domain:         "example.com",
		Environment:    models.SiteEnvironmentProduction,
		IsWordpress:    true,
		PHPVersion:     "8.2",
		ConnectionType: "ssh",
		SSHHost:        "example.ssh.wpengine.net",
		SSHPort:        2222,
		SSHUser:        "example",
		SitePath:       "sites/example",
		CanWPCLI:       true,
		HasAgent:       false,
		HasMonitoring:  true,
		Status:         "active",
	}

	if err := SaveManagedSite(site); err != nil {
		t.Fatalf("failed to save site: %v", err)
	}

	fetched, err := GetManagedSite(siteID)
	if err != nil {
		t.Fatalf("failed to get site: %v", err)
	}
	if fetched == nil {
		t.Fatalf("expected site, got nil")
	}
	if fetched.Domain != "example.com" || fetched.SSHHost != "example.ssh.wpengine.net" || fetched.SSHPort != 2222 {
		t.Errorf("unexpected site data: %+v", fetched)
	}
	if fetched.ServerName != "WPEngine" {
		t.Errorf("expected server name 'WPEngine', got %q", fetched.ServerName)
	}

	// Lookup by domain (case-insensitive)
	byDomain, err := GetManagedSiteByDomain("EXAMPLE.COM")
	if err != nil || byDomain == nil || byDomain.ID != siteID {
		t.Fatalf("failed to get site by domain: %+v (err: %v)", byDomain, err)
	}

	// List
	list, err := ListManagedSites()
	if err != nil {
		t.Fatalf("failed to list sites: %v", err)
	}
	if len(list) != 1 || list[0].ID != siteID {
		t.Fatalf("unexpected sites list: %+v", list)
	}

	// Filter by provider
	filtered, err := ListManagedSites("wpengine")
	if err != nil || len(filtered) != 1 {
		t.Fatalf("expected 1 site for provider wpengine, got %d (err: %v)", len(filtered), err)
	}
	none, err := ListManagedSites("spinupwp")
	if err != nil || len(none) != 0 {
		t.Fatalf("expected 0 sites for provider spinupwp, got %d (err: %v)", len(none), err)
	}

	// Delete
	if err := DeleteManagedSite(siteID); err != nil {
		t.Fatalf("failed to delete site: %v", err)
	}
	fetched, err = GetManagedSite(siteID)
	if err != nil {
		t.Fatalf("unexpected error after delete: %v", err)
	}
	if fetched != nil {
		t.Fatalf("expected nil for deleted site, got %+v", fetched)
	}
}
