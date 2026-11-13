package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/inventory"
	"github.com/JCO-Digital/jman/internal/inventory/domainlookup"
	_ "github.com/JCO-Digital/jman/internal/inventory/hetzner" // register the Hetzner Cloud provider
	"github.com/JCO-Digital/jman/internal/inventory/importer"
	_ "github.com/JCO-Digital/jman/internal/inventory/spinupwp" // register the SpinupWP provider
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/refresh"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// --- Inventory (asset registry) handlers ---
//
// The inventory is a manual-entry core with optional links to external APIs
// (providers). Provider-linked items are read-only for the fields the
// provider owns (type, name); syncs refresh their provider_data. See
// docs/inventory.md.

// inventoryItemRequest is the create/update payload. On an update, a field
// that is absent keeps its value. SiteID/ServerID accept a UUID or a legacy
// SpinupWP integer (FlexID); null or "" clears the link.
type inventoryItemRequest struct {
	Type        *string        `json:"type"`
	Name        *string        `json:"name"`
	Description *string        `json:"description"`
	Tags        []string       `json:"tags"`
	Details     map[string]any `json:"details"`
	SiteID      optionalFlexID `json:"site_id"`
	ServerID    optionalFlexID `json:"server_id"`
}

// optionalFlexID is a FlexID that knows whether the request had the field
// at all: Set is false when it was absent, and true for a value, null or "".
type optionalFlexID struct {
	Set bool
	ID  FlexID
}

// UnmarshalJSON implements json.Unmarshaler; it only runs when the field
// is present.
func (o *optionalFlexID) UnmarshalJSON(data []byte) error {
	o.Set = true
	return o.ID.UnmarshalJSON(data)
}

// InventoryDetailField is one suggested `details` key for an item type.
type InventoryDetailField struct {
	Key string `json:"key"`
	// Kind is how the UI renders the value: "text" or "list"
	// (comma-separated input, stored as a JSON array).
	Kind string `json:"kind"`
	// Hint is a short description shown next to the input.
	Hint string `json:"hint,omitempty"`
}

// InventoryTypeInfo describes one item type for the UI's forms: the template
// of detail fields a new item of this type starts with.
type InventoryTypeInfo struct {
	Type models.InventoryItemType `json:"type"`
	// DetailFields is the type's details template, in display order.
	DetailFields []InventoryDetailField `json:"detail_fields"`
}

// ListInventoryTypesHandler returns the known item types and their details
// templates.
func ListInventoryTypesHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, []InventoryTypeInfo{
		{
			Type: models.InventoryItemTypeServer,
			DetailFields: []InventoryDetailField{
				{Key: "ip_addresses", Kind: "list", Hint: "all IP addresses of the host"},
				{Key: "hostnames", Kind: "list", Hint: "all DNS names pointing at it"},
				{Key: "os", Kind: "text", Hint: "operating system and version"},
				{Key: "access", Kind: "text", Hint: "how to reach it and as which user — no passwords"},
				{Key: "services", Kind: "list", Hint: "services running on it"},
				{Key: "last_updated", Kind: "text", Hint: "when it was last updated (YYYY-MM-DD)"},
			},
		},
		{
			Type: models.InventoryItemTypeDomain,
			DetailFields: []InventoryDetailField{
				{Key: "registrar", Kind: "text", Hint: "where the domain is registered"},
				{Key: "dns_servers", Kind: "list", Hint: "the domain's DNS servers / nameservers"},
				{Key: "access", Kind: "text", Hint: "who manages it, and as which user — no passwords"},
				{Key: "expiry", Kind: "text", Hint: "renewal date (YYYY-MM-DD)"},
			},
		},
	})
}

// ListInventoryProvidersHandler returns the registered provider names.
func ListInventoryProvidersHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"providers": inventory.Names()})
}

// ListInventoryItemsHandler lists items, optionally filtered by type, status
// and a search term over name/description.
func ListInventoryItemsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := db.ListInventoryItems(db.InventoryItemFilter{
		Type:   q.Get("type"),
		Status: q.Get("status"),
		Search: q.Get("search"),
	})
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListInventoryItemsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, items)
}

// inventoryConflict is returned with 409 when a create collides with an
// existing item of the same type+name; the UI can then offer link/dismiss.
type inventoryConflict struct {
	Error          string `json:"error"`
	ExistingItemID string `json:"existing_item_id"`
}

// normalizeItemName validates and normalizes the name for a type: domain
// names are reduced to their apex (registrable) domain.
func normalizeItemName(itemType models.InventoryItemType, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("Name is required")
	}
	if itemType == models.InventoryItemTypeDomain {
		apex, err := utils.ApexDomain(name)
		if err != nil {
			return "", fmt.Errorf("Name must be a domain: %w", err)
		}
		return apex, nil
	}
	return name, nil
}

// applyRequest copies the editable fields that req has onto item, resolving
// the soft links; absent fields are left as they are. Type and name are
// handled by the callers.
func applyRequest(item *models.InventoryItem, req inventoryItemRequest) error {
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.Tags != nil {
		item.Tags = req.Tags
	}
	if req.Details != nil {
		item.Details = req.Details
	}
	if req.SiteID.Set {
		item.SiteID = nil
		if req.SiteID.ID != "" {
			id, err := resolveSiteUUID(string(req.SiteID.ID))
			if err != nil {
				return err
			}
			item.SiteID = &id
		}
	}
	if req.ServerID.Set {
		item.ServerID = nil
		if req.ServerID.ID != "" {
			id, err := resolveServerUUID(string(req.ServerID.ID))
			if err != nil {
				return err
			}
			item.ServerID = &id
		}
	}
	return nil
}

// requestTypeName validates the type and name of req. A field that is
// absent falls back to the given current value (empty on a create, where
// both are required).
func requestTypeName(req inventoryItemRequest, curType models.InventoryItemType, curName string) (models.InventoryItemType, string, error) {
	itemType := curType
	if req.Type != nil {
		itemType = models.InventoryItemType(strings.ToLower(strings.TrimSpace(*req.Type)))
	}
	if !models.IsValidInventoryItemType(itemType) {
		return "", "", fmt.Errorf("Type must be one of: server, domain")
	}
	name := curName
	if req.Name != nil {
		name = *req.Name
	}
	name, err := normalizeItemName(itemType, name)
	if err != nil {
		return "", "", err
	}
	return itemType, name, nil
}

// writeInventoryConflict answers 409 with the ID of the item that already
// has the requested type+name.
func writeInventoryConflict(w http.ResponseWriter, existingID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(inventoryConflict{
		Error:          "An item with this type and name already exists",
		ExistingItemID: existingID,
	})
}

// CreateInventoryItemHandler creates a manual inventory item. When an item
// with the same type+name already exists, it answers 409 with the existing
// item's ID so the client can offer link/dismiss instead of duplicating.
func CreateInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	var req inventoryItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	itemType, name, err := requestTypeName(req, "", "")
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := db.FindInventoryItemsByTypeName(itemType, name)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CreateInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if len(existing) > 0 {
		writeInventoryConflict(w, existing[0].ID)
		return
	}

	item := &models.InventoryItem{Type: itemType, Name: name}
	if err := applyRequest(item, req); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.InsertInventoryItem(item, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "CreateInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	created, err := db.GetInventoryItem(item.ID)
	if err != nil || created == nil {
		WriteJSON(w, http.StatusCreated, item)
		return
	}
	WriteJSON(w, http.StatusCreated, created)
}

// InventoryItemResponse is one item with its undismissed match suggestions
// and, for a domain item, its registry lookup.
type InventoryItemResponse struct {
	models.InventoryItem
	Matches      []models.InventoryItem        `json:"matches,omitempty"`
	DomainLookup *models.InventoryDomainLookup `json:"domain_lookup,omitempty"`
}

// GetInventoryItemHandler returns one item with its links and the other items
// matching its type+name (for the link/dismiss prompt).
func GetInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	item, ok := loadInventoryItem(w, r)
	if !ok {
		return
	}
	matches, err := db.FindInventoryItemMatches(item.ID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "GetInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	resp := InventoryItemResponse{InventoryItem: *item, Matches: matches}
	if item.Type == models.InventoryItemTypeDomain {
		lookup, err := db.GetInventoryDomainLookup(item.ID)
		if err != nil {
			verb.LogPrintf(verb.Normal, "GetInventoryItemHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		resp.DomainLookup = lookup
	}
	WriteJSON(w, http.StatusOK, resp)
}

// LookupInventoryDomainHandler looks a domain item up at its registry now
// and answers with the stored lookup. A lookup less than
// domainlookup.ManualMinInterval old for the same name is returned as it is,
// so repeated clicks don't query the registry again.
func LookupInventoryDomainHandler(w http.ResponseWriter, r *http.Request) {
	item, ok := loadInventoryItem(w, r)
	if !ok {
		return
	}
	if item.Type != models.InventoryItemTypeDomain {
		WriteError(w, http.StatusBadRequest, "Only domain items can be looked up")
		return
	}
	prev, err := db.GetInventoryDomainLookup(item.ID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "LookupInventoryDomainHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if prev != nil && prev.Domain == item.Name && time.Since(prev.CheckedAt) < domainlookup.ManualMinInterval {
		WriteJSON(w, http.StatusOK, prev)
		return
	}
	lookup, err := domainlookup.LookupItem(r.Context(), item.ID, item.Name)
	if err != nil {
		verb.LogPrintf(verb.Normal, "LookupInventoryDomainHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Lookup failed: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, lookup)
}

// loadInventoryItem reads the {id} path value and loads the item, writing the
// error response itself when it isn't found.
func loadInventoryItem(w http.ResponseWriter, r *http.Request) (*models.InventoryItem, bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	item, err := db.GetInventoryItem(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "inventory item load: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return nil, false
	}
	if item == nil {
		WriteError(w, http.StatusNotFound, "Inventory item not found")
		return nil, false
	}
	return item, true
}

// UpdateInventoryItemHandler updates the manual fields of an item; fields
// absent from the request keep their value. On a provider-linked item, type
// and name are read-only (a sync owns them). Renaming an item to the
// type+name of another one answers 409 like a create does.
func UpdateInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	item, ok := loadInventoryItem(w, r)
	if !ok {
		return
	}
	var req inventoryItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	itemType, name, err := requestTypeName(req, item.Type, item.Name)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	renamed := itemType != item.Type || !strings.EqualFold(name, item.Name)
	if len(item.Links) > 0 && (renamed || name != item.Name) {
		WriteError(w, http.StatusConflict, "Provider-linked items are read-only for type and name; changes would be overwritten by sync")
		return
	}
	if renamed {
		// Same rule as on create: no second item with this type+name.
		existing, err := db.FindInventoryItemsByTypeName(itemType, name)
		if err != nil {
			verb.LogPrintf(verb.Normal, "UpdateInventoryItemHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		for _, other := range existing {
			if other.ID != item.ID {
				writeInventoryConflict(w, other.ID)
				return
			}
		}
	}
	item.Type = itemType
	item.Name = name
	if err := applyRequest(item, req); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.UpdateInventoryItem(item, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "UpdateInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	updated, err := db.GetInventoryItem(item.ID)
	if err != nil || updated == nil {
		WriteJSON(w, http.StatusOK, item)
		return
	}
	WriteJSON(w, http.StatusOK, updated)
}

// DeleteInventoryItemHandler deletes a manual item. Provider-linked items
// are refused: a sync owns them and would recreate them.
func DeleteInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	item, ok := loadInventoryItem(w, r)
	if !ok {
		return
	}
	if len(item.Links) > 0 {
		WriteError(w, http.StatusConflict, "Provider-linked items cannot be deleted; unlink them first or wait for the sync to mark them removed")
		return
	}
	if err := db.DeleteInventoryItem(item.ID); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"message": "Inventory item deleted"})
}

// inventoryLinkRequest links an external entity to an item.
type inventoryLinkRequest struct {
	Provider     string `json:"provider"`
	ProviderType string `json:"provider_type"`
	ProviderID   string `json:"provider_id"`
}

// LinkInventoryItemHandler links an external entity to an item. When the
// entity is currently linked to another (provider-created) item, the link is
// moved here and the other item is marked removed if it has no links left.
func LinkInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	item, ok := loadInventoryItem(w, r)
	if !ok {
		return
	}
	var req inventoryLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Provider = strings.TrimSpace(req.Provider)
	req.ProviderType = strings.TrimSpace(req.ProviderType)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	if req.Provider == "" || req.ProviderType == "" || req.ProviderID == "" {
		WriteError(w, http.StatusBadRequest, "provider, provider_type and provider_id are required")
		return
	}
	if _, known := inventory.Get(req.Provider); !known {
		WriteError(w, http.StatusBadRequest, "Unknown provider: "+req.Provider)
		return
	}

	other, err := db.GetInventoryItemByLink(req.Provider, req.ProviderType, req.ProviderID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "LinkInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if other != nil && other.ID != item.ID {
		// Move the link from the auto-created item to this one: the unique
		// provider key means the old link must go first.
		if err := db.DeleteInventoryItemLink(req.Provider, req.ProviderType, req.ProviderID); err != nil {
			verb.LogPrintf(verb.Normal, "LinkInventoryItemHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		// The provider's snapshot moves with the link: the next sync
		// writes it on this item.
		_ = db.DropInventoryItemProviderData(other.ID, req.Provider)
		if remaining, _ := db.ListInventoryItemLinks(other.ID); len(remaining) == 0 && other.Origin == models.InventoryItemOriginProvider {
			_ = db.SetInventoryItemStatus(other.ID, models.InventoryItemStatusRemoved)
		}
	}
	link := models.InventoryItemLink{ItemID: item.ID, Provider: req.Provider, ProviderType: req.ProviderType, ProviderID: req.ProviderID}
	if err := db.AddInventoryItemLink(&link); err != nil {
		verb.LogPrintf(verb.Normal, "LinkInventoryItemHandler: %v", err)
		WriteError(w, http.StatusConflict, "This entity is already linked to the item")
		return
	}

	updated, _ := db.GetInventoryItem(item.ID)
	WriteJSON(w, http.StatusOK, updated)
}

// UnlinkInventoryItemHandler removes one provider link from an item. A
// provider-origin item left with no links is marked removed.
func UnlinkInventoryItemHandler(w http.ResponseWriter, r *http.Request) {
	item, ok := loadInventoryItem(w, r)
	if !ok {
		return
	}
	provider := r.PathValue("provider")
	providerType := r.PathValue("providerType")
	providerID := r.PathValue("providerId")

	linkItem, err := db.GetInventoryItemByLink(provider, providerType, providerID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "UnlinkInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if linkItem == nil || linkItem.ID != item.ID {
		WriteError(w, http.StatusNotFound, "Link not found on this item")
		return
	}
	if err := db.DeleteInventoryItemLink(provider, providerType, providerID); err != nil {
		verb.LogPrintf(verb.Normal, "UnlinkInventoryItemHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	_ = db.DropInventoryItemProviderData(item.ID, provider)
	if remaining, _ := db.ListInventoryItemLinks(item.ID); len(remaining) == 0 && item.Origin == models.InventoryItemOriginProvider {
		_ = db.SetInventoryItemStatus(item.ID, models.InventoryItemStatusRemoved)
	}
	updated, _ := db.GetInventoryItem(item.ID)
	WriteJSON(w, http.StatusOK, updated)
}

// DismissInventoryLinkMatchHandler records that the link/dismiss prompt for
// this item against one provider link was dismissed.
func DismissInventoryLinkMatchHandler(w http.ResponseWriter, r *http.Request) {
	item, ok := loadInventoryItem(w, r)
	if !ok {
		return
	}
	var req inventoryLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Provider == "" || req.ProviderType == "" || req.ProviderID == "" {
		WriteError(w, http.StatusBadRequest, "provider, provider_type and provider_id are required")
		return
	}
	if err := db.DismissInventoryLinkMatch(item.ID, req.Provider, req.ProviderType, req.ProviderID, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "DismissInventoryLinkMatchHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"message": "Match dismissed"})
}

// SyncInventoryProviderHandler runs one provider's sync now. For SpinupWP it
// first requests a servers/sites refresh so the provider reads current
// data. The refresh is skipped when the last one is less than
// refresh.ManualRefreshMinInterval old, so repeated syncs can't use up the
// SpinupWP API rate limit.
func SyncInventoryProviderHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if _, ok := inventory.Get(provider); !ok {
		WriteError(w, http.StatusBadRequest, "Unknown inventory provider: "+provider)
		return
	}
	if provider == "spinupwp" {
		// Best effort: on failure the provider syncs from the last good
		// cache instead, since the sync is idempotent.
		if _, err := refresh.RequestRefresh(); err != nil {
			verb.LogPrintf(verb.Normal, "SyncInventoryProviderHandler: servers/sites refresh failed, syncing from cache: %v", err)
		}
	}
	result, err := inventory.Sync(r.Context(), provider)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SyncInventoryProviderHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Sync failed: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// maxImportBody caps the size of an import request (the CSV plus options).
const maxImportBody = 8 << 20

// inventoryImportRequest is a domain import: the CSV text and how to read
// it. With DryRun nothing is written; the plan says what an import would do.
type inventoryImportRequest struct {
	CSV string `json:"csv"`
	importer.Options
	DryRun bool `json:"dry_run"`
}

// inventoryImportResponse is the plan and, for a real import, what it did.
type inventoryImportResponse struct {
	*importer.Plan
	DryRun  bool `json:"dry_run"`
	Created int  `json:"created"`
	Updated int  `json:"updated"`
}

// ImportInventoryDomainsHandler imports domains from a CSV file (see
// internal/inventory/importer). The client previews with dry_run, then
// sends the same request without it; the plan is made again from the
// inventory as it is then, so nothing stale is applied.
func ImportInventoryDomainsHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBody)
	var req inventoryImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body (at most 8 MB)")
		return
	}
	plan, err := importer.Build(req.CSV, req.Options)
	if errors.Is(err, importer.ErrInput) {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "ImportInventoryDomainsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	resp := inventoryImportResponse{Plan: plan, DryRun: req.DryRun}
	if !req.DryRun {
		resp.Created, resp.Updated, err = importer.Apply(plan, getUsername(r))
		if err != nil {
			verb.LogPrintf(verb.Normal, "ImportInventoryDomainsHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Import stopped after %d created and %d updated: %v", resp.Created, resp.Updated, err))
			return
		}
		verb.LogPrintf(verb.Normal, "%s imported domains: %d created, %d updated", getUsername(r), resp.Created, resp.Updated)
	}
	WriteJSON(w, http.StatusOK, resp)
}
