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
	if err != nil || len(matches) != 1 {
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

	// Second sync refreshes provider_data and creates nothing.
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
	if err != nil || srv == nil || srv.ProviderDataFor(p.name)["ip_address"] != "203.0.113.99" {
		t.Fatalf("provider_data not refreshed: %+v %v", srv, err)
	}

	// When the provider stops listing the domains, their links go and the
	// provider-origin item is marked removed.
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
}

// A synced domain joins the domain item already there (imported or entered
// by hand) instead of making a second one; a server, whose name is not its
// identity, doesn't.
func TestSyncJoinsExistingDomainItem(t *testing.T) {
	setupInventoryTest(t)
	p := &fakeProvider{}
	registerFake(t, p)

	imported := models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "example.com", Details: map[string]any{"customer": "Example Oy"}}
	manualServer := models.InventoryItem{Type: models.InventoryItemTypeServer, Name: "web1.examplehost.com"}
	for _, it := range []*models.InventoryItem{&imported, &manualServer} {
		if err := db.InsertInventoryItem(it, "tester"); err != nil {
			t.Fatal(err)
		}
	}

	p.items = []ExternalItem{
		serverItem("web1.examplehost.com"),
		domainItem("site-12:example.com", "example.com"),
		domainItem("site-12:www.example.com", "example.com"),
		domainItem("site-13:example.org", "example.org"),
	}
	res := mustSync(t, p.name)
	if res.Matched != 1 || res.Created != 2 || res.LinksAdded != 4 {
		t.Fatalf("sync: %+v", res)
	}
	got, _ := db.GetInventoryItem(imported.ID)
	if len(got.Links) != 2 || got.Details["customer"] != "Example Oy" || got.ProviderDataFor(p.name) == nil || got.Origin != models.InventoryItemOriginManual {
		t.Fatalf("imported item after sync: %+v", got)
	}
	if items, _ := db.FindInventoryItemsByTypeName(models.InventoryItemTypeServer, "web1.examplehost.com"); len(items) != 2 {
		t.Fatalf("server joined by name: %d items", len(items))
	}

	// When the provider drops it, the manual item stays active without its
	// provider snapshot.
	p.items = []ExternalItem{serverItem("web1.examplehost.com"), domainItem("site-13:example.org", "example.org")}
	if res := mustSync(t, p.name); res.LinksRemoved != 2 || res.Removed != 0 {
		t.Fatalf("removal sync: %+v", res)
	}
	got, _ = db.GetInventoryItem(imported.ID)
	if got.Status != models.InventoryItemStatusActive || len(got.Links) != 0 || got.ProviderDataFor(p.name) != nil {
		t.Fatalf("imported item after removal: %+v", got)
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
	if items[0].ProviderDataFor(p.name)["domain"] != "example.com" || *items[0].SiteID != "site-1" {
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
	if got.ProviderDataFor(p.name)["status"] != "changed" {
		t.Fatalf("provider_data not refreshed: %+v", got.ProviderData)
	}
}

// matchingFake is a fakeProvider with IPv4 match keys under data["ip"].
type matchingFake struct{ fakeProvider }

func (m *matchingFake) MatchKeys(itemType models.InventoryItemType, data map[string]any) []string {
	if itemType != models.InventoryItemTypeServer {
		return nil
	}
	return IPv4MatchKeys(data["ip"])
}

func registerMatching(t *testing.T, suffix string) *matchingFake {
	t.Helper()
	m := &matchingFake{}
	m.name = "fake-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + suffix
	Register(m)
	t.Cleanup(func() {
		registryMu.Lock()
		delete(registry, m.name)
		registryMu.Unlock()
	})
	return m
}

func matchServer(id, name, ip string, extra map[string]any) ExternalItem {
	data := map[string]any{"ip": ip}
	for k, v := range extra {
		data[k] = v
	}
	return ExternalItem{Type: models.InventoryItemTypeServer, ProviderType: "server", ProviderID: id, Name: name, Data: data}
}

// A server another provider already has (same public IPv4) joins its item;
// each provider keeps its own snapshot, and the creator keeps the name.
func TestSyncMatchesAcrossProviders(t *testing.T) {
	setupInventoryTest(t)
	panel := registerMatching(t, "panel")
	cloud := registerMatching(t, "cloud")

	panel.items = []ExternalItem{
		matchServer("1", "web1.example.com", "203.0.113.10", map[string]any{"php": "8.3"}),
		matchServer("2", "web2.example.com", "203.0.113.20", nil),
		matchServer("3", "lan.example.com", "10.0.0.5", nil), // private: never matched
	}
	mustSync(t, panel.name)

	cloud.items = []ExternalItem{
		matchServer("101", "hz-web1", "203.0.113.10", map[string]any{"cores": 4.0}),
		matchServer("103", "hz-lan", "10.0.0.5", nil),
		matchServer("104", "hz-new", "203.0.113.40", nil),
	}
	res := mustSync(t, cloud.name)
	if res.Matched != 1 || res.Created != 2 || res.LinksAdded != 3 {
		t.Fatalf("cloud sync: %+v", res)
	}

	web1, _ := db.GetInventoryItemByLink(panel.name, "server", "1")
	viaCloud, _ := db.GetInventoryItemByLink(cloud.name, "server", "101")
	if web1 == nil || viaCloud == nil || web1.ID != viaCloud.ID {
		t.Fatalf("matched server not joined: %+v %+v", web1, viaCloud)
	}
	if web1.Name != "web1.example.com" {
		t.Errorf("name = %q, want the creating provider's", web1.Name)
	}
	if web1.ProviderDataFor(panel.name)["php"] != "8.3" || web1.ProviderDataFor(cloud.name)["cores"] != 4.0 {
		t.Errorf("snapshots: %+v", web1.ProviderData)
	}
	if lan, _ := db.GetInventoryItemByLink(cloud.name, "server", "103"); lan == nil || lan.Name != "hz-lan" || len(lan.ProviderData) != 1 {
		t.Errorf("private address matched: %+v", lan)
	}

	// Both syncs again: nothing flips back and forth.
	if res := mustSync(t, panel.name); res.Updated != 0 {
		t.Errorf("panel resync: %+v", res)
	}
	if res := mustSync(t, cloud.name); res.Updated != 0 {
		t.Errorf("cloud resync: %+v", res)
	}

	// When the cloud stops listing the server, only its snapshot goes; the
	// item stays active through the panel's link.
	cloud.items = cloud.items[1:]
	mustSync(t, cloud.name)
	web1, _ = db.GetInventoryItem(web1.ID)
	if web1.Status != models.InventoryItemStatusActive || len(web1.Links) != 1 || web1.ProviderDataFor(cloud.name) != nil || web1.ProviderDataFor(panel.name) == nil {
		t.Fatalf("after cloud removal: %+v", web1)
	}
}

// Two items with the same IPv4 are ambiguous: no automatic join.
func TestSyncAmbiguousMatchCreatesItem(t *testing.T) {
	setupInventoryTest(t)
	panel := registerMatching(t, "panel")
	cloud := registerMatching(t, "cloud")
	panel.items = []ExternalItem{
		matchServer("1", "a.example.com", "203.0.113.10", nil),
		matchServer("2", "b.example.com", "203.0.113.10", nil),
	}
	mustSync(t, panel.name)
	cloud.items = []ExternalItem{matchServer("101", "hz", "203.0.113.10", nil)}
	if res := mustSync(t, cloud.name); res.Matched != 0 || res.Created != 1 {
		t.Fatalf("ambiguous match: %+v", res)
	}
}
