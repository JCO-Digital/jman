package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/inventory"
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

// inventoryItemRequest is the create/update payload. SiteID/ServerID accept
// a UUID or a legacy SpinupWP integer (FlexID).
type inventoryItemRequest struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Tags        []string       `json:"tags"`
	Details     map[string]any `json:"details"`
	SiteID      FlexID         `json:"site_id"`
	ServerID    FlexID         `json:"server_id"`
}

// InventoryTypeInfo describes one item type for the UI's forms.
type InventoryTypeInfo struct {
	Type models.InventoryItemType `json:"type"`
	// DetailFields hints which keys `details` usually holds for this type.
	DetailFields []string `json:"detail_fields"`
}

// ListInventoryTypesHandler returns the known item types and their detail
// field hints.
func ListInventoryTypesHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, []InventoryTypeInfo{
		{
			Type:         models.InventoryItemTypeServer,
			DetailFields: []string{"ip_addresses", "hostnames", "os", "access", "services", "last_updated", "notes"},
		},
		{
			Type:         models.InventoryItemTypeDomain,
			DetailFields: []string{"registrar", "dns_servers", "access", "expiry", "notes"},
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

// applyRequest copies the editable fields of req onto item, resolving the
// soft links. Type and name changes on a provider-linked item are rejected by
// the callers.
func applyRequest(item *models.InventoryItem, req inventoryItemRequest) error {
	item.Description = req.Description
	if req.Tags != nil {
		item.Tags = req.Tags
	}
	if req.Details != nil {
		item.Details = req.Details
	}
	if req.SiteID != "" {
		id, err := resolveSiteUUID(string(req.SiteID))
		if err != nil {
			return err
		}
		item.SiteID = &id
	} else {
		item.SiteID = nil
	}
	if req.ServerID != "" {
		id, err := resolveServerUUID(string(req.ServerID))
		if err != nil {
			return err
		}
		item.ServerID = &id
	} else {
		item.ServerID = nil
	}
	return nil
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
	itemType := models.InventoryItemType(strings.ToLower(strings.TrimSpace(req.Type)))
	if !models.IsValidInventoryItemType(itemType) {
		WriteError(w, http.StatusBadRequest, "Type must be one of: server, domain")
		return
	}
	name, err := normalizeItemName(itemType, req.Name)
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(inventoryConflict{
			Error:          "An item with this type and name already exists",
			ExistingItemID: existing[0].ID,
		})
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

// InventoryItemResponse is one item with its undismissed match suggestions.
type InventoryItemResponse struct {
	models.InventoryItem
	Matches []models.InventoryItem `json:"matches,omitempty"`
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
	WriteJSON(w, http.StatusOK, InventoryItemResponse{InventoryItem: *item, Matches: matches})
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

// UpdateInventoryItemHandler updates the manual fields of an item. On a
// provider-linked item, type and name are read-only (a sync owns them).
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
	itemType := models.InventoryItemType(strings.ToLower(strings.TrimSpace(req.Type)))
	if !models.IsValidInventoryItemType(itemType) {
		WriteError(w, http.StatusBadRequest, "Type must be one of: server, domain")
		return
	}
	name, err := normalizeItemName(itemType, req.Name)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(item.Links) > 0 && (itemType != item.Type || name != item.Name) {
		WriteError(w, http.StatusConflict, "Provider-linked items are read-only for type and name; changes would be overwritten by sync")
		return
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
// first forces a fresh servers/sites refresh so the provider reads current
// data, mirroring SyncSpinupWPHandler.
func SyncInventoryProviderHandler(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if _, ok := inventory.Get(provider); !ok {
		WriteError(w, http.StatusBadRequest, "Unknown inventory provider: "+provider)
		return
	}
	if provider == "spinupwp" {
		if _, err := refresh.RefreshServersAndSites(); err != nil {
			verb.LogPrintf(verb.Normal, "SyncInventoryProviderHandler: %v", err)
			WriteError(w, http.StatusBadGateway, "Refresh failed: "+err.Error())
			return
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
