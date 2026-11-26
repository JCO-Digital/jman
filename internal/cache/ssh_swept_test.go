package cache

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestSSHSweptSiteIDsSkipsAgentSites(t *testing.T) {
	setupCacheTest(t)
	if err := db.InitInventory(); err != nil {
		t.Fatalf("init inventory: %v", err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatalf("init api: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	const ssh, agent, noCLI = "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333"
	for _, ms := range []models.ManagedSite{
		{ID: ssh, Provider: "manual", Domain: "ssh.example.org", IsWordpress: true, CanWPCLI: true, SSHHost: "h1", SSHUser: "u"},
		{ID: agent, Provider: "manual", Domain: "agent.example.org", IsWordpress: true, CanWPCLI: true, SSHHost: "h2", SSHUser: "u"},
		{ID: noCLI, Provider: "manual", Domain: "nocli.example.org", IsWordpress: true, CanWPCLI: false, SSHHost: "h3"},
	} {
		if err := db.SaveManagedSite(ms); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RecordSiteAgentWPCollection(agent, time.Now(), ""); err != nil {
		t.Fatalf("record agent collection: %v", err)
	}

	got := SSHSweptSiteIDs()
	if len(got) != 1 || !got[ssh] {
		t.Errorf("SSHSweptSiteIDs = %v, want only %s", got, ssh)
	}
}
