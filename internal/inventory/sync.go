package inventory

import (
	"bytes"
	"context"
	"encoding/json"
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
	Provider string `json:"provider"`
	Fetched  int    `json:"fetched"`
	Created  int    `json:"created"`
	// Updated counts existing items whose name, status, provider_data or
	// site link the sync changed.
	Updated      int `json:"updated"`
	LinksAdded   int `json:"links_added"`
	LinksRemoved int `json:"links_removed"`
	// Matched counts new entities that joined another provider's item
	// through a match key (see Matcher).
	Matched int `json:"matched"`
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

// syncTarget is one item a sync run found entities for.
type syncTarget struct {
	item    *models.InventoryItem
	exts    []ExternalItem
	created bool
}

// Sync fetches one provider's entities and upserts them as inventory items:
//
//   - an external entity with an existing link refreshes its item's name,
//     provider_data and status (manual fields are kept),
//   - a new entity is linked to the item another entity of the same
//     (type, name) is linked to, or else gets its own item
//     (origin='provider', deterministic UUID). An item that still exists
//     under that UUID, e.g. one marked removed earlier, is reused and
//     reactivated instead of inserted again,
//   - links whose entity the provider no longer lists are deleted; an
//     origin='provider' item left with no links is marked removed.
//
// A new entity first joins the item another entity of this provider with
// the same (type, name) is linked to; else, for a provider with match keys
// (Matcher), the item of another provider whose snapshot has one of its
// keys, e.g. the SpinupWP server with a Hetzner server's IPv4 address; else
// it gets its own item.
//
// Each provider writes only its own entry of provider_data, so an item
// linked to several providers keeps a snapshot from each. With several
// links of one provider, the snapshot and the site link come from one of
// them, chosen the same way on every run (primaryEntity). The name is
// written only by the provider that owns it (nameOwner). The site link is
// only filled in when the item has none, so one set by hand stays. Items
// are written only when something changed, and without touching
// updated_at/updated_by.
//
// Manual items are never created or removed by a sync, and only linked by
// hand.
//
// The run is not one transaction, but every step is idempotent: a sync
// that fails halfway is completed by the next one.
func Sync(ctx context.Context, providerName string) (SyncResult, error) {
	syncMu.Lock()
	defer syncMu.Unlock()

	result := SyncResult{Provider: providerName}
	p, ok := Get(providerName)
	if !ok {
		return result, fmt.Errorf("unknown inventory provider %q", providerName)
	}

	fetched, err := p.Fetch(ctx)
	if errors.Is(err, ErrNotConfigured) {
		result.Skipped = true
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("inventory provider %s fetch failed: %w", providerName, err)
	}
	if len(fetched) == 0 {
		result.Skipped = true
		return result, nil
	}
	result.Fetched = len(fetched)

	links, err := db.ListInventoryLinksByProvider(providerName)
	if err != nil {
		return result, err
	}
	linkedItem := make(map[string]string, len(links)) // link key -> item ID
	for _, link := range links {
		linkedItem[link.ProviderType+"\x00"+link.ProviderID] = link.ItemID
	}

	targets := map[string]*syncTarget{}
	var order []string // item IDs in first-seen order
	target := func(itemID string) (*syncTarget, error) {
		if t, ok := targets[itemID]; ok {
			return t, nil
		}
		item, err := db.GetInventoryItem(itemID)
		if err != nil || item == nil {
			return nil, err
		}
		t := &syncTarget{item: item}
		targets[itemID] = t
		order = append(order, itemID)
		return t, nil
	}

	seen := make(map[string]bool, len(fetched))
	fetchedTypes := map[string]bool{}
	// byName maps "type\x00lower(name)" to the item its entities share.
	byName := make(map[string]string, len(fetched))
	var unlinked []ExternalItem

	// First the entities that are already linked, so that a new entity finds
	// the item of its name whatever the order of the fetch.
	for _, ext := range fetched {
		linkKey := ext.ProviderType + "\x00" + ext.ProviderID
		if seen[linkKey] {
			continue
		}
		seen[linkKey] = true
		fetchedTypes[ext.ProviderType] = true

		itemID, ok := linkedItem[linkKey]
		if !ok {
			unlinked = append(unlinked, ext)
			continue
		}
		t, err := target(itemID)
		if err != nil {
			return result, err
		}
		if t == nil {
			// The link's item is gone (deleted since the links were read).
			unlinked = append(unlinked, ext)
			continue
		}
		t.exts = append(t.exts, ext)
		// A provider-created item wins over a manual one that was linked by
		// hand to an entity of the same name.
		key := nameKey(ext)
		if cur, ok := byName[key]; !ok ||
			(targets[cur].item.Origin != models.InventoryItemOriginProvider && t.item.Origin == models.InventoryItemOriginProvider) {
			byName[key] = itemID
		}
	}

	var matches map[string]string
	matcher, canMatch := p.(Matcher)
	if canMatch && len(unlinked) > 0 {
		if matches, err = matchIndex(providerName); err != nil {
			return result, err
		}
	}

	for _, ext := range unlinked {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		link := &models.InventoryItemLink{Provider: providerName, ProviderType: ext.ProviderType, ProviderID: ext.ProviderID}
		key := nameKey(ext)

		itemID, shared := byName[key]
		if !shared && canMatch {
			for _, k := range matcher.MatchKeys(ext.Type, ext.Data) {
				if id := matches[string(ext.Type)+"\x00"+k]; id != "" {
					itemID, shared = id, true
					result.Matched++
					break
				}
			}
		}
		if !shared {
			itemID = utils.InventoryItemUUID(providerName, ext.ProviderType, ext.ProviderID)
		}
		t, err := target(itemID)
		if err != nil {
			return result, err
		}
		if t == nil {
			item := &models.InventoryItem{
				ID:           itemID,
				Type:         ext.Type,
				Name:         ext.Name,
				Origin:       models.InventoryItemOriginProvider,
				Status:       models.InventoryItemStatusActive,
				ProviderData: map[string]any{providerName: ext.Data},
			}
			if ext.SiteID != "" {
				siteID := ext.SiteID
				item.SiteID = &siteID
			}
			if err := db.InsertInventoryItem(item, syncUser); err != nil {
				return result, err
			}
			t = &syncTarget{item: item, created: true}
			targets[itemID] = t
			order = append(order, itemID)
			result.Created++
		}
		link.ItemID = itemID
		if err := db.AddInventoryItemLink(link); err != nil {
			return result, err
		}
		t.item.Links = append(t.item.Links, *link)
		result.LinksAdded++
		t.exts = append(t.exts, ext)
		byName[key] = itemID
	}

	// Refresh the provider-owned fields of every item that has entities,
	// writing only the items that changed.
	for _, itemID := range order {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		t := targets[itemID]
		if len(t.exts) == 0 {
			continue
		}
		primary := primaryEntity(providerName, t.item.ID, t.exts)
		changed := false
		if nameOwner(t.item) == providerName && t.item.Name != primary.Name {
			t.item.Name = primary.Name
			changed = true
		}
		if t.item.Status != models.InventoryItemStatusActive {
			t.item.Status = models.InventoryItemStatusActive
			changed = true
		}
		if t.item.SiteID == nil && primary.SiteID != "" {
			siteID := primary.SiteID
			t.item.SiteID = &siteID
			changed = true
		}
		if !sameJSON(t.item.ProviderDataFor(providerName), primary.Data) {
			changed = true
		}
		if !changed {
			continue
		}
		if err := db.UpdateInventoryItemProviderFields(t.item, providerName, primary.Data); err != nil {
			return result, err
		}
		if !t.created {
			result.Updated++
		}
	}

	// Remove links whose external entity the provider no longer lists. A
	// provider type the fetch returned nothing at all for is left alone:
	// that is more likely a partial fetch (e.g. no sites but all servers)
	// than every entity of the type disappearing at once.
	for _, link := range links {
		if seen[link.ProviderType+"\x00"+link.ProviderID] || !fetchedTypes[link.ProviderType] {
			continue
		}
		if err := db.DeleteInventoryItemLink(providerName, link.ProviderType, link.ProviderID); err != nil {
			return result, err
		}
		result.LinksRemoved++
		if err := db.DropInventoryItemProviderData(link.ItemID, providerName); err != nil {
			return result, err
		}
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
		if item != nil && item.Origin == models.InventoryItemOriginProvider && item.Status != models.InventoryItemStatusRemoved {
			if err := db.SetInventoryItemStatus(item.ID, models.InventoryItemStatusRemoved); err != nil {
				return result, err
			}
			result.Removed++
		}
	}

	return result, nil
}

// Matcher is implemented by providers whose entities can be recognized in
// another provider's snapshot of the same thing, such as a server by its
// IPv4 address. MatchKeys returns the keys of one snapshot (the Data of an
// ExternalItem); a new entity joins the item of another provider whose
// snapshot has one of its keys.
type Matcher interface {
	MatchKeys(itemType models.InventoryItemType, data map[string]any) []string
}

// matchIndex maps "type\x00key" to the item whose snapshot from a provider
// other than providerName has that match key. Items are only indexed for a
// provider they still have a link of. A key found on two items is left out,
// so an ambiguous match creates a new item instead of picking one.
func matchIndex(providerName string) (map[string]string, error) {
	items, err := db.ListInventoryItems(db.InventoryItemFilter{Status: string(models.InventoryItemStatusActive)})
	if err != nil {
		return nil, err
	}
	index := map[string]string{}
	ambiguous := map[string]bool{}
	for _, it := range items {
		linked := map[string]bool{}
		for _, l := range it.Links {
			linked[l.Provider] = true
		}
		for other := range it.ProviderData {
			if other == providerName || !linked[other] {
				continue
			}
			p, ok := Get(other)
			if !ok {
				continue
			}
			m, ok := p.(Matcher)
			if !ok {
				continue
			}
			for _, k := range m.MatchKeys(it.Type, it.ProviderDataFor(other)) {
				key := string(it.Type) + "\x00" + k
				if cur, ok := index[key]; ok && cur != it.ID {
					ambiguous[key] = true
				}
				index[key] = it.ID
			}
		}
	}
	for key := range ambiguous {
		delete(index, key)
	}
	return index, nil
}

// nameOwner is the provider whose sync writes the item's name: the one that
// created it (its deterministic UUID is the item's ID), else the first by
// name among the item's links. Without it, two providers naming the same
// server differently would rename it on every sync.
func nameOwner(item *models.InventoryItem) string {
	owner := ""
	for _, l := range item.Links {
		if utils.InventoryItemUUID(l.Provider, l.ProviderType, l.ProviderID) == item.ID {
			return l.Provider
		}
		if owner == "" || l.Provider < owner {
			owner = l.Provider
		}
	}
	return owner
}

// nameKey is the key entities share an item by: type and lower-cased name.
func nameKey(ext ExternalItem) string {
	return string(ext.Type) + "\x00" + strings.ToLower(ext.Name)
}

// primaryEntity picks the entity an item with several links takes its name,
// provider_data and site link from: the one the item was created for (its
// deterministic UUID is the item's ID), else the one with the smallest
// (provider type, provider ID). The choice doesn't depend on fetch order.
func primaryEntity(providerName, itemID string, exts []ExternalItem) ExternalItem {
	best := exts[0]
	for _, ext := range exts {
		if utils.InventoryItemUUID(providerName, ext.ProviderType, ext.ProviderID) == itemID {
			return ext
		}
		if ext.ProviderType < best.ProviderType ||
			(ext.ProviderType == best.ProviderType && ext.ProviderID < best.ProviderID) {
			best = ext
		}
	}
	return best
}

// sameJSON reports whether two provider_data values encode to the same
// JSON, which is how a stored snapshot compares to a freshly fetched one
// (the stored one has been through a JSON round trip).
func sameJSON(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
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
