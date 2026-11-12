package inventory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func setupInventoryTest(t *testing.T) {
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

type fakeProvider struct {
	name  string
	items []ExternalItem
	err   error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Fetch(context.Context) ([]ExternalItem, error) {
	return f.items, f.err
}

func registerFake(t *testing.T, p *fakeProvider) {
	t.Helper()
	// Register under a per-test name so tests in one process don't clash.
	p.name = "fake-" + strings.ReplaceAll(t.Name(), "/", "-")
	Register(p)
	t.Cleanup(func() {
		registryMu.Lock()
		delete(registry, p.name)
		registryMu.Unlock()
	})
}

func serverItem(name string) ExternalItem {
	return ExternalItem{
		Type:         models.InventoryItemTypeServer,
		ProviderType: "server",
		ProviderID:   "srv-" + name,
		Name:         name,
		Data:         map[string]any{"ip_address": "203.0.113.10"},
	}
}

func domainItem(providerID, apex string) ExternalItem {
	return ExternalItem{
		Type:         models.InventoryItemTypeDomain,
		ProviderType: "site_domain",
		ProviderID:   providerID,
		Name:         apex,
		Data:         map[string]any{"domain": apex},
	}
}

func TestSyncCreatesUpdatesRemoves(t *testing.T) {
	setupInventoryTest(t)
	p := &fakeProvider{}
	registerFake(t, p)

	// A manual item sharing a name with a provider entity stays separate.
	manual := models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "example.com"}
	if err := db.InsertInventoryItem(&manual, "tester"); err != nil {
		t.Fatal(err)
	}

	p.items = []ExternalItem{
		serverItem("web1.examplehost.com"),
		domainItem("site-12:example.com", "example.com"),     // primary domain of site 12
		domainItem("site-12:www.example.com", "example.com"), // additional domain, same apex
	}
	res, err := Sync(context.Background(), p.name)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Created != 2 || res.LinksAdded != 3 || res.Fetched != 3 {
		t.Fatalf("first sync: %+v", res)
	}

	// The two domain entities share one item named by the apex domain.
	matches, err := db.FindInventoryItemsByTypeName(models.InventoryItemTypeDomain, "example.com")
	if err != nil || len(matches) != 2 {
		t.Fatalf("domain items: %d, %v", len(matches), err)
	}
	var synced *models.InventoryItem
	for i := range matches {
		if matches[i].Origin == models.InventoryItemOriginProvider {
			synced = &matches[i]
		}
	}
	if synced == nil || len(synced.Links) != 2 {
		t.Fatalf("synced domain item: %+v", synced)
	}
	if synced.ID != utils.InventoryItemUUID(p.name, "site_domain", "site-12:example.com") {
		t.Errorf("item ID = %s", synced.ID)
	}

	// Second sync refreshes provider_data, creates nothing, keeps the manual item.
	p.items[0].Data["ip_address"] = "203.0.113.99"
	res, err = Sync(context.Background(), p.name)
	if err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	if res.Created != 0 || res.Updated != 3 {
		t.Fatalf("second sync: %+v", res)
	}
	srv, err := db.GetInventoryItemByLink(p.name, "server", "srv-web1.examplehost.com")
	if err != nil || srv == nil || srv.ProviderData["ip_address"] != "203.0.113.99" {
		t.Fatalf("provider_data not refreshed: %+v %v", srv, err)
	}
	gotManual, _ := db.GetInventoryItem(manual.ID)
	if len(gotManual.Links) != 0 || gotManual.Status != models.InventoryItemStatusActive {
		t.Fatalf("manual item touched by sync: %+v", gotManual)
	}

	// When the provider stops listing the domains, their links go and the
	// provider-origin item is marked removed; the manual item stays.
	p.items = []ExternalItem{serverItem("web1.examplehost.com")}
	p.items[0].Data["ip_address"] = "203.0.113.99"
	res, err = Sync(context.Background(), p.name)
	if err != nil {
		t.Fatalf("sync 3: %v", err)
	}
	if res.LinksRemoved != 2 || res.Removed != 1 {
		t.Fatalf("third sync: %+v", res)
	}
	gotSynced, _ := db.GetInventoryItem(synced.ID)
	if gotSynced.Status != models.InventoryItemStatusRemoved || len(gotSynced.Links) != 0 {
		t.Fatalf("removed item: %+v", gotSynced)
	}
	gotManual, _ = db.GetInventoryItem(manual.ID)
	if gotManual.Status != models.InventoryItemStatusActive {
		t.Fatalf("manual item after removal sync: %+v", gotManual)
	}
}

func TestSyncSkipsWhenNotConfiguredOrEmpty(t *testing.T) {
	setupInventoryTest(t)
	p := &fakeProvider{}
	registerFake(t, p)

	p.items = []ExternalItem{serverItem("web1.examplehost.com")}
	if _, err := Sync(context.Background(), p.name); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Not configured: nothing changes, no error.
	p.err = ErrNotConfigured
	res, err := Sync(context.Background(), p.name)
	if err != nil || !res.Skipped {
		t.Fatalf("not configured: %+v %v", res, err)
	}
	if l, _ := db.ListInventoryLinksByProvider(p.name); len(l) != 1 {
		t.Fatalf("links changed on skipped sync: %+v", l)
	}

	// Empty fetch: treated as a likely failed fetch, also skipped.
	p.err = nil
	p.items = nil
	res, err = Sync(context.Background(), p.name)
	if err != nil || !res.Skipped {
		t.Fatalf("empty: %+v %v", res, err)
	}
	if l, _ := db.ListInventoryLinksByProvider(p.name); len(l) != 1 {
		t.Fatalf("links removed on empty sync: %+v", l)
	}

	// A real fetch error is reported.
	p.err = errors.New("boom")
	if _, err := Sync(context.Background(), p.name); err == nil {
		t.Fatal("fetch error not reported")
	}
}

func TestSyncUnknownProvider(t *testing.T) {
	setupInventoryTest(t)
	if _, err := Sync(context.Background(), "nope"); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
