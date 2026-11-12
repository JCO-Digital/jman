package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// syncUser is the audit user recorded for changes a sync makes.
const syncUser = "jman-api"

// SyncResult summarizes one provider sync.
type SyncResult struct {
	Provider     string `json:"provider"`
	Fetched      int    `json:"fetched"`
	Created      int    `json:"created"`
	Updated      int    `json:"updated"`
	LinksAdded   int    `json:"links_added"`
	LinksRemoved int    `json:"links_removed"`
	// Removed counts items marked status='removed' because their last
	// provider link disappeared.
	Removed int `json:"removed"`
	// Skipped is true when the sync changed nothing because the provider
	// is not configured or returned an empty list (which more likely means
	// a failed fetch than no entities at all — the same guard
	// markRemovedSpinupWPSites applies).
	Skipped bool `json:"skipped,omitempty"`
}

// syncMu serializes syncs: the scheduler and manual sync requests must not
// run concurrently.
var syncMu sync.Mutex

// Sync fetches one provider's entities and upserts them as inventory items:
//
//   - an external entity with an existing link gets its item's name and
//     provider_data refreshed (manual fields are kept),
//   - a new entity creates an item (origin='provider', deterministic UUID)
//     plus its link; entities of one sync sharing (type, name) share one
//     item with several links,
//   - links whose entity the provider no longer lists are deleted; an
//     origin='provider' item left with no links is marked removed.
//
// Manual items are never created, linked or removed by a sync.
func Sync(ctx context.Context, providerName string) (SyncResult, error) {
	syncMu.Lock()
	defer syncMu.Unlock()

	result := SyncResult{Provider: providerName}
	p, ok := Get(providerName)
	if !ok {
		return result, fmt.Errorf("unknown inventory provider %q", providerName)
	}

	items, err := p.Fetch(ctx)
	if errors.Is(err, ErrNotConfigured) {
		result.Skipped = true
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("inventory provider %s fetch failed: %w", providerName, err)
	}
	if len(items) == 0 {
		result.Skipped = true
		return result, nil
	}
	result.Fetched = len(items)

	seen := make(map[string]bool, len(items))
	// byName maps "type\x00lower(name)" to the item ID created or updated
	// for it during this sync, so entities sharing a name share one item.
	byName := make(map[string]string, len(items))

	for _, ext := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		linkKey := ext.ProviderType + "\x00" + ext.ProviderID
		seen[linkKey] = true

		existing, err := db.GetInventoryItemByLink(providerName, ext.ProviderType, ext.ProviderID)
		if err != nil {
			return result, err
		}
		if existing != nil {
			existing.Name = ext.Name
			existing.ProviderData = ext.Data
			if ext.SiteID != "" {
				existing.SiteID = &ext.SiteID
			}
			if err := db.UpdateInventoryItem(existing, syncUser); err != nil {
				return result, err
			}
			result.Updated++
			byName[string(ext.Type)+"\x00"+strings.ToLower(ext.Name)] = existing.ID
			continue
		}

		nameKey := string(ext.Type) + "\x00" + strings.ToLower(ext.Name)
		if itemID, ok := byName[nameKey]; ok {
			if err := db.AddInventoryItemLink(&models.InventoryItemLink{
				ItemID: itemID, Provider: providerName, ProviderType: ext.ProviderType, ProviderID: ext.ProviderID,
			}); err != nil {
				return result, err
			}
			result.LinksAdded++
			continue
		}

		item := &models.InventoryItem{
			ID:           utils.InventoryItemUUID(providerName, ext.ProviderType, ext.ProviderID),
			Type:         ext.Type,
			Name:         ext.Name,
			Origin:       models.InventoryItemOriginProvider,
			Status:       models.InventoryItemStatusActive,
			ProviderData: ext.Data,
		}
		if ext.SiteID != "" {
			item.SiteID = &ext.SiteID
		}
		if err := db.InsertInventoryItem(item, syncUser); err != nil {
			return result, err
		}
		if err := db.AddInventoryItemLink(&models.InventoryItemLink{
			ItemID: item.ID, Provider: providerName, ProviderType: ext.ProviderType, ProviderID: ext.ProviderID,
		}); err != nil {
			return result, err
		}
		result.Created++
		result.LinksAdded++
		byName[nameKey] = item.ID
	}

	// Remove links whose external entity the provider no longer lists.
	links, err := db.ListInventoryLinksByProvider(providerName)
	if err != nil {
		return result, err
	}
	for _, link := range links {
		if seen[link.ProviderType+"\x00"+link.ProviderID] {
			continue
		}
		if err := db.DeleteInventoryItemLink(providerName, link.ProviderType, link.ProviderID); err != nil {
			return result, err
		}
		result.LinksRemoved++
		remaining, err := db.ListInventoryItemLinks(link.ItemID)
		if err != nil {
			return result, err
		}
		if len(remaining) > 0 {
			continue
		}
		item, err := db.GetInventoryItem(link.ItemID)
		if err != nil {
			return result, err
		}
		if item != nil && item.Origin == models.InventoryItemOriginProvider {
			if err := db.SetInventoryItemStatus(item.ID, models.InventoryItemStatusRemoved); err != nil {
				return result, err
			}
			result.Removed++
		}
	}

	return result, nil
}

// SyncAll runs Sync for every registered provider and returns one result
// per provider. A provider's failure is reported in the error but does not
// stop the others.
func SyncAll(ctx context.Context) ([]SyncResult, error) {
	var results []SyncResult
	var errs []error
	for _, name := range Names() {
		res, err := Sync(ctx, name)
		results = append(results, res)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return results, errors.Join(errs...)
}
