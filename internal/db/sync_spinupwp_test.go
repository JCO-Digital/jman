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
