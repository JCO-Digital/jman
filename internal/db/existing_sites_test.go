package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestExistingSiteIDs(t *testing.T) {
	setupTaskRepoTest(t)

	for _, s := range []models.ManagedSite{
		{ID: "active", Provider: "manual", Domain: "a.example.com", Status: "active"},
		{ID: "paused", Provider: "manual", Domain: "p.example.com", Status: "paused"},
		{ID: "removed", Provider: "spinupwp", Domain: "r.example.com", Status: SiteStatusRemoved},
		{ID: "deleted", Provider: "manual", Domain: "d.example.com", Status: "active"},
	} {
		if err := SaveManagedSite(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := DeleteManagedSite("deleted"); err != nil {
		t.Fatal(err)
	}

	ids, err := ExistingSiteIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || !ids["active"] || !ids["paused"] {
		t.Errorf("ExistingSiteIDs = %v, want active and paused", ids)
	}
}
