package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestInventoryItemSaveGetList(t *testing.T) {
	setupTestAPIDB(t)

	item := models.InventoryItem{
		Type:        models.InventoryItemTypeDomain,
		Name:        "example.com",
		Description: "Main domain",
		Tags:        []string{"prod", "dns"},
		Details:     map[string]any{"registrar": "Namecheap", "dns_servers": []any{"ns1.example.com", "ns2.example.com"}},
	}
	if err := InsertInventoryItem(&item, "tester"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if item.ID == "" {
		t.Fatal("save did not assign an ID")
	}
	if item.Origin != models.InventoryItemOriginManual {
		t.Errorf("origin = %q, want manual", item.Origin)
	}
	if item.Status != models.InventoryItemStatusActive {
		t.Errorf("status = %q, want active", item.Status)
	}

	got, err := GetInventoryItem(item.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Name != "example.com" || got.Description != "Main domain" {
		t.Errorf("got %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "prod" {
		t.Errorf("tags = %v", got.Tags)
	}
	if got.Details["registrar"] != "Namecheap" {
		t.Errorf("details = %v", got.Details)
	}
	if got.CreatedBy != "tester" {
		t.Errorf("created_by = %q", got.CreatedBy)
	}

	// Update keeps the origin and updates the manual fields.
	got.Description = "Updated"
	got.ProviderData = map[string]any{"ip": "203.0.113.10"}
	got.Origin = models.InventoryItemOriginProvider // must be ignored on update
	if err := UpdateInventoryItem(got, "tester"); err != nil {
		t.Fatalf("update: %v", err)
	}
	again, err := GetInventoryItem(item.ID)
	if err != nil || again == nil {
		t.Fatalf("get after update: %v %v", again, err)
	}
	if again.Description != "Updated" || again.Origin != models.InventoryItemOriginManual {
		t.Errorf("after update: description=%q origin=%q", again.Description, again.Origin)
	}
	if again.ProviderData["ip"] != "203.0.113.10" {
		t.Errorf("provider_data = %v", again.ProviderData)
	}

	// List with filters.
	server := models.InventoryItem{Type: models.InventoryItemTypeServer, Name: "web1.examplehost.com"}
	if err := InsertInventoryItem(&server, "tester"); err != nil {
		t.Fatalf("save server: %v", err)
	}
	all, err := ListInventoryItems(InventoryItemFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: %d items, %v", len(all), err)
	}
	domains, err := ListInventoryItems(InventoryItemFilter{Type: string(models.InventoryItemTypeDomain)})
	if err != nil || len(domains) != 1 || domains[0].Name != "example.com" {
		t.Fatalf("list domains: %+v %v", domains, err)
	}
	found, err := ListInventoryItems(InventoryItemFilter{Search: "web1"})
	if err != nil || len(found) != 1 {
		t.Fatalf("list search: %+v %v", found, err)
	}
}

func TestInventoryItemLinks(t *testing.T) {
	setupTestAPIDB(t)

	item := models.InventoryItem{Type: models.InventoryItemTypeServer, Name: "lon1.examplehost.com"}
	if err := InsertInventoryItem(&item, "tester"); err != nil {
		t.Fatalf("save: %v", err)
	}

	link := models.InventoryItemLink{
		ItemID:       item.ID,
		Provider:     "spinupwp",
		ProviderType: "server",
		ProviderID:   "678",
	}
	if err := AddInventoryItemLink(&link); err != nil {
		t.Fatalf("add link: %v", err)
	}

	byLink, err := GetInventoryItemByLink("spinupwp", "server", "678")
	if err != nil || byLink == nil || byLink.ID != item.ID {
		t.Fatalf("by link: %+v %v", byLink, err)
	}

	// The same external entity cannot be linked twice.
	dup := models.InventoryItemLink{ItemID: item.ID, Provider: "spinupwp", ProviderType: "server", ProviderID: "678"}
	if err := AddInventoryItemLink(&dup); err == nil {
		t.Fatal("duplicate link was accepted")
	}

	links, err := ListInventoryItemLinks(item.ID)
	if err != nil || len(links) != 1 || links[0].ProviderID != "678" {
		t.Fatalf("links: %+v %v", links, err)
	}
	byProvider, err := ListInventoryLinksByProvider("spinupwp")
	if err != nil || len(byProvider) != 1 {
		t.Fatalf("links by provider: %+v %v", byProvider, err)
	}

	// Deleting the item cascades its links.
	if err := DeleteInventoryItem(item.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if l, err := ListInventoryLinksByProvider("spinupwp"); err != nil || len(l) != 0 {
		t.Fatalf("links after delete: %+v %v", l, err)
	}
	if got, err := GetInventoryItem(item.ID); err != nil || got != nil {
		t.Fatalf("get after delete: %v %v", got, err)
	}

	// DeleteInventoryItemLink by provider key.
	item2 := models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "example.org"}
	if err := InsertInventoryItem(&item2, "tester"); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	if err := AddInventoryItemLink(&models.InventoryItemLink{ItemID: item2.ID, Provider: "spinupwp", ProviderType: "site_domain", ProviderID: "12:example.org"}); err != nil {
		t.Fatalf("add link 2: %v", err)
	}
	if err := DeleteInventoryItemLink("spinupwp", "site_domain", "12:example.org"); err != nil {
		t.Fatalf("delete link: %v", err)
	}
	if l, err := ListInventoryItemLinks(item2.ID); err != nil || len(l) != 0 {
		t.Fatalf("links after unlink: %+v %v", l, err)
	}
}

func TestInventoryItemStatusAndMatches(t *testing.T) {
	setupTestAPIDB(t)

	manual := models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "Example.COM"}
	if err := InsertInventoryItem(&manual, "tester"); err != nil {
		t.Fatalf("save manual: %v", err)
	}
	synced := models.InventoryItem{
		ID:     "11111111-1111-4111-8111-111111111111",
		Type:   models.InventoryItemTypeDomain,
		Name:   "example.com",
		Origin: models.InventoryItemOriginProvider,
	}
	if err := InsertInventoryItem(&synced, "sync"); err != nil {
		t.Fatalf("save synced: %v", err)
	}
	if err := AddInventoryItemLink(&models.InventoryItemLink{ItemID: synced.ID, Provider: "spinupwp", ProviderType: "site_domain", ProviderID: "12:www.example.com"}); err != nil {
		t.Fatalf("link: %v", err)
	}

	// Case-insensitive type+name lookup finds both.
	found, err := FindInventoryItemsByTypeName(models.InventoryItemTypeDomain, "EXAMPLE.com")
	if err != nil || len(found) != 2 {
		t.Fatalf("find by type+name: %d items, %v", len(found), err)
	}

	// The manual item sees the synced one as a match candidate.
	matches, err := FindInventoryItemMatches(manual.ID)
	if err != nil || len(matches) != 1 || matches[0].ID != synced.ID {
		t.Fatalf("matches: %+v %v", matches, err)
	}
	// The synced item sees the manual one only if the manual one has links; it has none.
	if m, err := FindInventoryItemMatches(synced.ID); err != nil || len(m) != 0 {
		t.Fatalf("matches of synced: %+v %v", m, err)
	}

	// Dismissing hides the match; dismissing again is fine.
	if err := DismissInventoryLinkMatch(manual.ID, "spinupwp", "site_domain", "12:www.example.com", "tester"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if err := DismissInventoryLinkMatch(manual.ID, "spinupwp", "site_domain", "12:www.example.com", "tester"); err != nil {
		t.Fatalf("dismiss again: %v", err)
	}
	if m, err := FindInventoryItemMatches(manual.ID); err != nil || len(m) != 0 {
		t.Fatalf("matches after dismiss: %+v %v", m, err)
	}

	// Status transitions.
	if err := SetInventoryItemStatus(synced.ID, models.InventoryItemStatusRemoved); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, err := GetInventoryItem(synced.ID)
	if err != nil || got == nil || got.Status != models.InventoryItemStatusRemoved {
		t.Fatalf("status: %+v %v", got, err)
	}
	active, err := ListInventoryItems(InventoryItemFilter{Status: string(models.InventoryItemStatusActive)})
	if err != nil || len(active) != 1 || active[0].ID != manual.ID {
		t.Fatalf("active items: %+v %v", active, err)
	}
}
