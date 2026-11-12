// Package inventory implements the asset inventory's provider framework:
// external APIs ("providers") are registered here, and Sync upserts the
// entities they expose as inventory items in api.db, linked through
// inventory_item_links. Manual items (no links) are the core of the system
// and are never touched by syncs. See docs/inventory.md.
package inventory

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/JCO-Digital/jman/internal/models"
)

// ErrNotConfigured is returned by a Provider's Fetch when it has no
// credentials configured. Sync treats it as "nothing to do" and changes
// nothing — in particular it does not remove any links.
var ErrNotConfigured = errors.New("provider not configured")

// ExternalItem is one entity an external API exposes, in inventory terms.
type ExternalItem struct {
	// Type is the inventory item type ('server', 'domain').
	Type models.InventoryItemType
	// ProviderType is the entity kind at the provider ('server',
	// 'site_domain', ...), and ProviderID its stable ID there. Together
	// with the provider name they key inventory_item_links.
	ProviderType string
	ProviderID   string
	// Name is the item's name; for domains the apex (registrable) domain.
	Name string
	// Data becomes the item's provider_data, refreshed on every sync.
	Data map[string]any
	// SiteID optionally soft-links the item to a functional site UUID.
	SiteID string
}

// Provider is an external API that inventory items can be linked to.
type Provider interface {
	Name() string
	Fetch(ctx context.Context) ([]ExternalItem, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Provider{}
)

// Register adds a provider to the registry. It panics on a duplicate name:
// providers are registered at init time, so a clash is a programming error.
func Register(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	name := p.Name()
	if _, exists := registry[name]; exists {
		panic("inventory: provider registered twice: " + name)
	}
	registry[name] = p
}

// Get returns a registered provider by name.
func Get(name string) (Provider, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[name]
	return p, ok
}

// Names returns the registered provider names, sorted.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
