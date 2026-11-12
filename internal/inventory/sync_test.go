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
	// Only the server changed, so only it is written.
	if res.Created != 0 || res.Updated != 1 {
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
	// (Another domain is still listed: a fetch with no domains at all
	// would be taken for a partial fetch and remove nothing.)
	p.items = []ExternalItem{serverItem("web1.examplehost.com"), domainItem("site-13:example.org", "example.org")}
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

func mustSync(t *testing.T, name string) SyncResult {
	t.Helper()
	res, err := Sync(context.Background(), name)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	return res
}

// An entity that disappears and comes back, or is unlinked by hand, reuses
// its removed item instead of failing on the primary key.
func TestSyncReactivatesRemovedItem(t *testing.T) {
	setupInventoryTest(t)
	p := &fakeProvider{}
	registerFake(t, p)

	both := []ExternalItem{domainItem("1:example.com", "example.com"), domainItem("2:example.org", "example.org")}
	p.items = both
	mustSync(t, p.name)
	id := utils.InventoryItemUUID(p.name, "site_domain", "1:example.com")

	p.items = both[1:]
	if res := mustSync(t, p.name); res.Removed != 1 {
		t.Fatalf("removal sync: %+v", res)
	}
	p.items = both
	res := mustSync(t, p.name)
	if res.Created != 0 || res.LinksAdded != 1 || res.Updated != 1 {
		t.Fatalf("reappear sync: %+v", res)
	}
	item, _ := db.GetInventoryItem(id)
	if item == nil || item.Status != models.InventoryItemStatusActive || len(item.Links) != 1 {
		t.Fatalf("reactivated item: %+v", item)
	}

	// Unlinked by hand (as the unlink handler does): the next sync relinks.
	if err := db.DeleteInventoryItemLink(p.name, "site_domain", "1:example.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInventoryItemStatus(id, models.InventoryItemStatusRemoved); err != nil {
		t.Fatal(err)
	}
	mustSync(t, p.name)
	item, _ = db.GetInventoryItem(id)
	if item == nil || item.Status != models.InventoryItemStatusActive || len(item.Links) != 1 {
		t.Fatalf("relinked item: %+v", item)
	}
	if res := mustSync(t, p.name); res.Updated != 0 || res.LinksAdded != 0 {
		t.Fatalf("steady-state sync: %+v", res)
	}
}

// A fetch that returns nothing for one provider type removes nothing of
// that type.
func TestSyncPartialFetchKeepsLinks(t *testing.T) {
	setupInventoryTest(t)
	p := &fakeProvider{}
	registerFake(t, p)

	p.items = []ExternalItem{serverItem("web1"), serverItem("web2"), domainItem("1:example.com", "example.com")}
	mustSync(t, p.name)

	p.items = []ExternalItem{serverItem("web1")}
	res := mustSync(t, p.name)
	if res.LinksRemoved != 1 || res.Removed != 1 {
		t.Fatalf("partial sync: %+v", res)
	}
	domain, _ := db.GetInventoryItemByLink(p.name, "site_domain", "1:example.com")
	if domain == nil || domain.Status != models.InventoryItemStatusActive {
		t.Fatalf("domain removed by a fetch without domains: %+v", domain)
	}
}

// A new entity joins the existing item of its name whatever the fetch
// order, and the item's data doesn't depend on that order either.
func TestSyncSharedItemIsOrderIndependent(t *testing.T) {
	setupInventoryTest(t)
	p := &fakeProvider{}
	registerFake(t, p)

	first := domainItem("1:example.com", "example.com")
	first.SiteID = "site-1"
	p.items = []ExternalItem{first}
	mustSync(t, p.name)

	second := domainItem("2:staging.example.com", "example.com")
	second.Data = map[string]any{"domain": "staging.example.com"}
	second.SiteID = "site-2"
	p.items = []ExternalItem{second, first}
	res := mustSync(t, p.name)
	if res.Created != 0 || res.LinksAdded != 1 || res.Updated != 0 {
		t.Fatalf("second entity: %+v", res)
	}
	items, _ := db.FindInventoryItemsByTypeName(models.InventoryItemTypeDomain, "example.com")
	if len(items) != 1 || len(items[0].Links) != 2 {
		t.Fatalf("items: %+v", items)
	}
	if items[0].ProviderData["domain"] != "example.com" || *items[0].SiteID != "site-1" {
		t.Fatalf("shared item took the wrong entity's data: %+v", items[0])
	}
	p.items = []ExternalItem{first, second}
	if res := mustSync(t, p.name); res.Updated != 0 {
		t.Fatalf("reordered fetch rewrote the item: %+v", res)
	}
}

// A sync keeps a site link set by hand and the audit fields of the last
// manual edit.
func TestSyncKeepsManualEdits(t *testing.T) {
	setupInventoryTest(t)
	p := &fakeProvider{}
	registerFake(t, p)

	ext := domainItem("1:example.com", "example.com")
	ext.SiteID = "site-1"
	p.items = []ExternalItem{ext}
	mustSync(t, p.name)

	item, _ := db.GetInventoryItemByLink(p.name, "site_domain", "1:example.com")
	manualSite := "site-by-hand"
	item.SiteID = &manualSite
	item.Description = "edited"
	if err := db.UpdateInventoryItem(item, "editor"); err != nil {
		t.Fatal(err)
	}

	p.items[0].Data = map[string]any{"domain": "example.com", "status": "changed"}
	if res := mustSync(t, p.name); res.Updated != 1 {
		t.Fatalf("sync after edit: %+v", res)
	}
	got, _ := db.GetInventoryItem(item.ID)
	if *got.SiteID != manualSite || got.Description != "edited" || got.UpdatedBy != "editor" {
		t.Fatalf("manual edit lost: site=%s description=%q updated_by=%q", *got.SiteID, got.Description, got.UpdatedBy)
	}
	if got.ProviderData["status"] != "changed" {
		t.Fatalf("provider_data not refreshed: %+v", got.ProviderData)
	}
}
