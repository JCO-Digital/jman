package refresh

import (
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func setupAgentlessTest(t *testing.T) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "jman-agentless-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = tempDir

	if err := db.InitInventory(); err != nil {
		t.Fatalf("Failed to init inventory DB: %v", err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatalf("Failed to init API DB: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tempDir)
		config.RunData.DataDir = oldDataDir
	})
}

func TestCollectAgentlessSites_EmptyOrNonTarget(t *testing.T) {
	setupAgentlessTest(t)

	// No sites: should be a clean no-op
	if err := CollectAgentlessSites(); err != nil {
		t.Fatalf("expected nil for empty sites, got: %v", err)
	}

	// Site with has_agent = true: should be skipped
	siteWithAgent := models.ManagedSite{
		ID:             utils.NewV7UUID(),
		Domain:         "with-agent.com",
		Provider:       "manual",
		HasAgent:       true,
		CanWPCLI:       true,
		SSHHost:        "host.example.com",
		Status:         "active",
		ConnectionType: "agent",
	}
	if err := db.SaveManagedSite(siteWithAgent); err != nil {
		t.Fatalf("failed to save site: %v", err)
	}

	// Site with can_wp_cli = false: should be skipped
	siteNoCLI := models.ManagedSite{
		ID:             utils.NewV7UUID(),
		Domain:         "no-cli.com",
		Provider:       "manual",
		HasAgent:       false,
		CanWPCLI:       false,
		SSHHost:        "host.example.com",
		Status:         "active",
		ConnectionType: "none",
	}
	if err := db.SaveManagedSite(siteNoCLI); err != nil {
		t.Fatalf("failed to save site: %v", err)
	}

	if err := CollectAgentlessSites(); err != nil {
		t.Fatalf("expected nil when no eligible agentless sites exist, got: %v", err)
	}
}

func TestManagedSiteToCliSite(t *testing.T) {
	site := models.ManagedSite{
		ID:         "test-uuid-1234",
		Domain:     "wpengine-test.com",
		ServerName: "WPEngine",
		SSHHost:    "wpengine-test.ssh.wpengine.net",
		SSHPort:    2222,
		SSHUser:    "wpenginetest",
		SitePath:   "sites/wpenginetest",
	}

	cliSite := site.ToCliSite()
	if cliSite.UUID != "test-uuid-1234" {
		t.Errorf("expected UUID 'test-uuid-1234', got %q", cliSite.UUID)
	}
	if cliSite.Name != "wpengine-test.com" {
		t.Errorf("expected Name 'wpengine-test.com', got %q", cliSite.Name)
	}
	if cliSite.SSH != "wpenginetest@wpengine-test.ssh.wpengine.net:2222" {
		t.Errorf("expected SSH 'wpenginetest@wpengine-test.ssh.wpengine.net:2222', got %q", cliSite.SSH)
	}
	if cliSite.Path != "sites/wpenginetest" {
		t.Errorf("expected Path 'sites/wpenginetest', got %q", cliSite.Path)
	}
}
