package domainlookup

import (
	"context"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := config.RunData
	config.RunData.DataDir = dir
	config.RunData.CacheDir = dir
	if err := db.InitInventory(); err != nil {
		t.Fatalf("init inventory: %v", err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatalf("init api: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		config.RunData = old
	})
}

func addDomain(t *testing.T, name string) *models.InventoryItem {
	t.Helper()
	item := &models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: name}
	if err := db.InsertInventoryItem(item, "tester"); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestRefreshDueStoresAndKeepsData(t *testing.T) {
	setupDB(t)
	c, reg := newTestClient(t)
	old := DefaultClient
	DefaultClient = c
	t.Cleanup(func() { DefaultClient = old })

	com := addDomain(t, "example.com")
	fi := addDomain(t, "example.fi")
	dk := addDomain(t, "example.dk")
	server := &models.InventoryItem{Type: models.InventoryItemTypeServer, Name: "web1"}
	if err := db.InsertInventoryItem(server, "tester"); err != nil {
		t.Fatal(err)
	}

	n, err := RefreshDue(context.Background())
	if err != nil || n != 3 {
		t.Fatalf("first run: %d %v", n, err)
	}
	got, _ := db.GetInventoryDomainLookup(com.ID)
	if got == nil || got.Registrar != "Example Registrar Inc." || got.ExpiresAt == nil || got.LastSuccessAt == nil || got.Error != "" {
		t.Fatalf("com lookup: %+v", got)
	}
	if got, _ := db.GetInventoryDomainLookup(fi.ID); got == nil || got.Source != "rdap+whois" || len(got.Nameservers) != 2 {
		t.Fatalf("fi lookup: %+v", got)
	}
	if got, _ := db.GetInventoryDomainLookup(dk.ID); got == nil || got.Error == "" {
		t.Fatalf("unsupported TLD stored without error: %+v", got)
	}

	// Nothing is due right after.
	if n, err := RefreshDue(context.Background()); err != nil || n != 0 {
		t.Fatalf("second run: %d %v", n, err)
	}

	// A renamed item is due at once.
	fi.Name = "other.fi"
	if err := db.UpdateInventoryItem(fi, "tester"); err != nil {
		t.Fatal(err)
	}
	due, _ := db.ListDueInventoryDomainLookups(time.Now().Add(-RefreshAfter), time.Now().Add(-RetryAfter), 10)
	if len(due) != 1 || due[0].ItemID != fi.ID || due[0].Domain != "other.fi" {
		t.Fatalf("due after rename: %+v", due)
	}

	// A failing lookup keeps the last good data and records the error.
	reg.rdap.Close()
	l, err := LookupItem(context.Background(), com.ID, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if l.Error == "" || l.Registrar != "Example Registrar Inc." || l.LastSuccessAt == nil || l.LastSuccessAt.After(l.CheckedAt) {
		t.Fatalf("failed lookup: %+v", l)
	}
}
