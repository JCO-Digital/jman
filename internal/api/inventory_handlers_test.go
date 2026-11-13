package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/inventory"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupInventoryHandlerTest(t *testing.T) context.Context {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "jman-inventory-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	oldDataDir := config.RunData.DataDir
	oldCacheDir := config.RunData.CacheDir
	config.RunData.DataDir = tempDir
	config.RunData.CacheDir = tempDir
	if err := db.InitInventory(); err != nil {
		t.Fatalf("init inventory: %v", err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatalf("init api: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tempDir)
		config.RunData.DataDir = oldDataDir
		config.RunData.CacheDir = oldCacheDir
	})
	return contextWithClaims(context.Background(), &AuthClaims{Username: "tester", Level: config.LevelEdit})
}

func doInventoryRequest(t *testing.T, ctx context.Context, method, target, body string, handler http.HandlerFunc, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, rdr).WithContext(ctx)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestInventoryItemCRUD(t *testing.T) {
	ctx := setupInventoryHandlerTest(t)

	// Create a server item.
	w := doInventoryRequest(t, ctx, "POST", "/api/inventory/items",
		`{"type":"server","name":"db1.internal","description":"MariaDB host","tags":["infra"],"details":{"ip_addresses":["10.0.0.5"],"access":"ops user: admin"}}`,
		CreateInventoryItemHandler, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created models.InventoryItem
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Origin != models.InventoryItemOriginManual || created.Status != models.InventoryItemStatusActive {
		t.Fatalf("created: %+v", created)
	}
	if created.Details["access"] != "ops user: admin" || len(created.Tags) != 1 {
		t.Fatalf("details/tags: %+v", created)
	}

	// Domain names are reduced to the apex domain.
	w = doInventoryRequest(t, ctx, "POST", "/api/inventory/items",
		`{"type":"domain","name":"WWW.Example.CO.uk"}`, CreateInventoryItemHandler, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create domain: %d %s", w.Code, w.Body.String())
	}
	var domain models.InventoryItem
	json.Unmarshal(w.Body.Bytes(), &domain)
	if domain.Name != "example.co.uk" {
		t.Fatalf("domain name = %q, want example.co.uk", domain.Name)
	}

	// Invalid type and non-domain name are rejected.
	if w := doInventoryRequest(t, ctx, "POST", "/api/inventory/items", `{"type":"vm","name":"x"}`, CreateInventoryItemHandler, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad type: %d", w.Code)
	}
	if w := doInventoryRequest(t, ctx, "POST", "/api/inventory/items", `{"type":"domain","name":"not a domain"}`, CreateInventoryItemHandler, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad domain: %d", w.Code)
	}

	// Duplicate type+name -> 409 with the existing item's ID.
	w = doInventoryRequest(t, ctx, "POST", "/api/inventory/items", `{"type":"domain","name":"example.co.uk"}`, CreateInventoryItemHandler, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body.String())
	}
	var conflict inventoryConflict
	json.Unmarshal(w.Body.Bytes(), &conflict)
	if conflict.ExistingItemID != domain.ID {
		t.Fatalf("conflict: %+v", conflict)
	}

	// List with filters.
	w = doInventoryRequest(t, ctx, "GET", "/api/inventory/items?type=domain", "", ListInventoryItemsHandler, nil)
	var items []models.InventoryItem
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 || items[0].Name != "example.co.uk" {
		t.Fatalf("list: %+v", items)
	}

	// Get one item.
	w = doInventoryRequest(t, ctx, "GET", "/api/inventory/items/"+created.ID, "", GetInventoryItemHandler, map[string]string{"id": created.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("get: %d", w.Code)
	}

	// Update description/details.
	w = doInventoryRequest(t, ctx, "PATCH", "/api/inventory/items/"+created.ID,
		`{"type":"server","name":"db1.internal","description":"updated","details":{"services":["mariadb"]}}`,
		UpdateInventoryItemHandler, map[string]string{"id": created.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	var updated models.InventoryItem
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Description != "updated" || updated.Details["services"] == nil {
		t.Fatalf("updated: %+v", updated)
	}

	// A partial update keeps the fields it doesn't name.
	w = doInventoryRequest(t, ctx, "PATCH", "/api/inventory/items/"+created.ID,
		`{"tags":["db"]}`, UpdateInventoryItemHandler, map[string]string{"id": created.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("partial update: %d %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Name != "db1.internal" || updated.Description != "updated" || len(updated.Tags) != 1 || updated.Details["services"] == nil {
		t.Fatalf("partial update: %+v", updated)
	}

	// Renaming an item to another item's type+name -> 409, like a create.
	w = doInventoryRequest(t, ctx, "PATCH", "/api/inventory/items/"+created.ID,
		`{"type":"domain","name":"www.example.co.uk"}`, UpdateInventoryItemHandler, map[string]string{"id": created.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("rename onto existing: %d %s", w.Code, w.Body.String())
	}
	conflict = inventoryConflict{}
	json.Unmarshal(w.Body.Bytes(), &conflict)
	if conflict.ExistingItemID != domain.ID {
		t.Fatalf("rename conflict: %+v", conflict)
	}

	// Delete works for a manual item.
	w = doInventoryRequest(t, ctx, "DELETE", "/api/inventory/items/"+created.ID, "", DeleteInventoryItemHandler, map[string]string{"id": created.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d", w.Code)
	}
	w = doInventoryRequest(t, ctx, "GET", "/api/inventory/items/"+created.ID, "", GetInventoryItemHandler, map[string]string{"id": created.ID})
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d", w.Code)
	}
}

func TestInventoryLinkFlow(t *testing.T) {
	ctx := setupInventoryHandlerTest(t)

	// A provider-created item with a link (as a sync would make it).
	synced := models.InventoryItem{
		ID:     "22222222-2222-4222-8222-222222222222",
		Type:   models.InventoryItemTypeDomain,
		Name:   "example.com",
		Origin: models.InventoryItemOriginProvider,
	}
	if err := db.InsertInventoryItem(&synced, "jman-api"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInventoryItemLink(&models.InventoryItemLink{ItemID: synced.ID, Provider: "spinupwp", ProviderType: "site_domain", ProviderID: "12:example.com"}); err != nil {
		t.Fatal(err)
	}

	// A manual item with the same name.
	w := doInventoryRequest(t, ctx, "POST", "/api/inventory/items", `{"type":"domain","name":"example.com"}`, CreateInventoryItemHandler, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("create manual duplicate: %d %s", w.Code, w.Body.String())
	}
	var conflict inventoryConflict
	json.Unmarshal(w.Body.Bytes(), &conflict)

	// The get of the existing item shows the manual match... create the
	// manual item directly to simulate "dismiss" of the create conflict.
	manual := models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "example.com"}
	if err := db.InsertInventoryItem(&manual, "tester"); err != nil {
		t.Fatal(err)
	}
	w = doInventoryRequest(t, ctx, "GET", "/api/inventory/items/"+manual.ID, "", GetInventoryItemHandler, map[string]string{"id": manual.ID})
	var resp InventoryItemResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Matches) != 1 || resp.Matches[0].ID != synced.ID {
		t.Fatalf("matches: %+v", resp.Matches)
	}

	// Dismiss the match prompt.
	w = doInventoryRequest(t, ctx, "POST", "/api/inventory/items/"+manual.ID+"/dismiss",
		`{"provider":"spinupwp","provider_type":"site_domain","provider_id":"12:example.com"}`,
		DismissInventoryLinkMatchHandler, map[string]string{"id": manual.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("dismiss: %d %s", w.Code, w.Body.String())
	}
	w = doInventoryRequest(t, ctx, "GET", "/api/inventory/items/"+manual.ID, "", GetInventoryItemHandler, map[string]string{"id": manual.ID})
	var resp2 InventoryItemResponse
	json.Unmarshal(w.Body.Bytes(), &resp2)
	if len(resp2.Matches) != 0 {
		t.Fatalf("matches after dismiss: %+v", resp2.Matches)
	}

	// Link the spinupwp entity to the manual item: the link moves, the
	// provider item is marked removed.
	w = doInventoryRequest(t, ctx, "POST", "/api/inventory/items/"+manual.ID+"/links",
		`{"provider":"spinupwp","provider_type":"site_domain","provider_id":"12:example.com"}`,
		LinkInventoryItemHandler, map[string]string{"id": manual.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("link: %d %s", w.Code, w.Body.String())
	}
	var linked models.InventoryItem
	json.Unmarshal(w.Body.Bytes(), &linked)
	if len(linked.Links) != 1 || linked.Links[0].Provider != "spinupwp" {
		t.Fatalf("linked: %+v", linked)
	}
	old, _ := db.GetInventoryItem(synced.ID)
	if old.Status != models.InventoryItemStatusRemoved || len(old.Links) != 0 {
		t.Fatalf("old provider item: %+v", old)
	}

	// Provider-linked items reject name changes and deletion.
	w = doInventoryRequest(t, ctx, "PATCH", "/api/inventory/items/"+manual.ID,
		`{"type":"domain","name":"other.com"}`, UpdateInventoryItemHandler, map[string]string{"id": manual.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("rename linked: %d", w.Code)
	}
	w = doInventoryRequest(t, ctx, "DELETE", "/api/inventory/items/"+manual.ID, "", DeleteInventoryItemHandler, map[string]string{"id": manual.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("delete linked: %d", w.Code)
	}

	// Unlink restores manual ownership; then delete works.
	w = doInventoryRequest(t, ctx, "DELETE", "/api/inventory/items/"+manual.ID+"/links/spinupwp/site_domain/12:example.com", "",
		UnlinkInventoryItemHandler, map[string]string{"id": manual.ID, "provider": "spinupwp", "providerType": "site_domain", "providerId": "12:example.com"})
	if w.Code != http.StatusOK {
		t.Fatalf("unlink: %d %s", w.Code, w.Body.String())
	}
	w = doInventoryRequest(t, ctx, "DELETE", "/api/inventory/items/"+manual.ID, "", DeleteInventoryItemHandler, map[string]string{"id": manual.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("delete after unlink: %d", w.Code)
	}
}

type fakeInventoryProvider struct{ name string }

func (f fakeInventoryProvider) Name() string { return f.name }
func (f fakeInventoryProvider) Fetch(context.Context) ([]inventory.ExternalItem, error) {
	return []inventory.ExternalItem{{
		Type:         models.InventoryItemTypeServer,
		ProviderType: "server",
		ProviderID:   "srv-1",
		Name:         "web1.examplehost.com",
		Data:         map[string]any{"ip_address": "203.0.113.10"},
	}}, nil
}

func TestSyncInventoryProviderHandler(t *testing.T) {
	ctx := setupInventoryHandlerTest(t)

	// Unknown provider -> 400.
	w := doInventoryRequest(t, ctx, "POST", "/api/inventory/sync/nope", "", SyncInventoryProviderHandler, map[string]string{"provider": "nope"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown provider: %d", w.Code)
	}

	// Register a fake provider and sync it.
	inventory.Register(fakeInventoryProvider{name: "fake-api-test"})
	w = doInventoryRequest(t, ctx, "POST", "/api/inventory/sync/fake-api-test", "", SyncInventoryProviderHandler, map[string]string{"provider": "fake-api-test"})
	if w.Code != http.StatusOK {
		t.Fatalf("sync: %d %s", w.Code, w.Body.String())
	}
	var res inventory.SyncResult
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Created != 1 || res.Fetched != 1 {
		t.Fatalf("sync result: %+v", res)
	}
	item, err := db.GetInventoryItemByLink("fake-api-test", "server", "srv-1")
	if err != nil || item == nil || item.Origin != models.InventoryItemOriginProvider {
		t.Fatalf("synced item: %+v %v", item, err)
	}
	if item.ProviderData["ip_address"] != "203.0.113.10" {
		t.Fatalf("provider_data: %+v", item.ProviderData)
	}
}

func TestListInventoryTypesHandler(t *testing.T) {
	ctx := setupInventoryHandlerTest(t)
	w := doInventoryRequest(t, ctx, "GET", "/api/inventory/types", "", ListInventoryTypesHandler, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("types: %d", w.Code)
	}
	var types []InventoryTypeInfo
	if err := json.Unmarshal(w.Body.Bytes(), &types); err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 {
		t.Fatalf("types: %+v", types)
	}
	for _, ti := range types {
		if len(ti.DetailFields) == 0 {
			t.Errorf("type %s has no template fields", ti.Type)
		}
		for _, f := range ti.DetailFields {
			if f.Key == "" || (f.Kind != "text" && f.Kind != "list") {
				t.Errorf("type %s field: %+v", ti.Type, f)
			}
		}
	}
}

func TestNotesAcceptInventoryItemParent(t *testing.T) {
	ctx := setupInventoryHandlerTest(t)

	w := doInventoryRequest(t, ctx, "POST", "/api/inventory/items",
		`{"type":"server","name":"db1.internal"}`, CreateInventoryItemHandler, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var item models.InventoryItem
	json.Unmarshal(w.Body.Bytes(), &item)

	// Notes on the item go through the generic notes endpoints.
	w = doInventoryRequest(t, ctx, "POST", "/api/notes",
		`{"parent_type":"InventoryItem","parent_id":"`+item.ID+`","content":"Runs MariaDB 11"}`,
		CreateNoteHandler, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create note: %d %s", w.Code, w.Body.String())
	}
	w = doInventoryRequest(t, ctx, "GET", "/api/notes?type=InventoryItem&id="+item.ID, "", ListNotesHandler, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list notes: %d", w.Code)
	}
	var notes []models.Note
	json.Unmarshal(w.Body.Bytes(), &notes)
	if len(notes) != 1 || notes[0].Content != "Runs MariaDB 11" || notes[0].ParentType != models.NoteParentTypeInventoryItem {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestInventoryDomainLookupHandlers(t *testing.T) {
	ctx := setupInventoryHandlerTest(t)

	server := models.InventoryItem{Type: models.InventoryItemTypeServer, Name: "web1.internal"}
	if err := db.InsertInventoryItem(&server, "tester"); err != nil {
		t.Fatal(err)
	}
	w := doInventoryRequest(t, ctx, "POST", "/api/inventory/items/"+server.ID+"/lookup", "", LookupInventoryDomainHandler, map[string]string{"id": server.ID})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("lookup of a server: %d %s", w.Code, w.Body.String())
	}

	domain := models.InventoryItem{Type: models.InventoryItemTypeDomain, Name: "example.com"}
	if err := db.InsertInventoryItem(&domain, "tester"); err != nil {
		t.Fatal(err)
	}
	// A fresh stored lookup is returned as it is, without asking a registry.
	now := time.Now().UTC()
	stored := &models.InventoryDomainLookup{ItemID: domain.ID, Domain: "example.com", Registrar: "Example Registrar Inc.", Nameservers: []string{"ns1.example.com"}, Source: "rdap", CheckedAt: now, LastSuccessAt: &now}
	if err := db.SaveInventoryDomainLookup(stored); err != nil {
		t.Fatal(err)
	}
	w = doInventoryRequest(t, ctx, "POST", "/api/inventory/items/"+domain.ID+"/lookup", "", LookupInventoryDomainHandler, map[string]string{"id": domain.ID})
	var got models.InventoryDomainLookup
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != http.StatusOK || got.Registrar != "Example Registrar Inc." {
		t.Fatalf("recent lookup: %d %s", w.Code, w.Body.String())
	}

	// The item response carries the lookup.
	w = doInventoryRequest(t, ctx, "GET", "/api/inventory/items/"+domain.ID, "", GetInventoryItemHandler, map[string]string{"id": domain.ID})
	var resp InventoryItemResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.DomainLookup == nil || resp.DomainLookup.Nameservers[0] != "ns1.example.com" {
		t.Fatalf("item response: %s", w.Body.String())
	}
}
