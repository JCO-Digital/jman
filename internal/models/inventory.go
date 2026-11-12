package models

import "time"

// InventoryItemType defines the kind of an inventory item.
type InventoryItemType string

const (
	InventoryItemTypeServer InventoryItemType = "server"
	InventoryItemTypeDomain InventoryItemType = "domain"
)

// InventoryItemOrigin tells whether an item was created by hand or by a
// provider sync. Syncs only ever auto-remove origin='provider' items.
type InventoryItemOrigin string

const (
	InventoryItemOriginManual   InventoryItemOrigin = "manual"
	InventoryItemOriginProvider InventoryItemOrigin = "provider"
)

// InventoryItemStatus is the lifecycle status of an inventory item.
type InventoryItemStatus string

const (
	InventoryItemStatusActive  InventoryItemStatus = "active"
	InventoryItemStatusRemoved InventoryItemStatus = "removed"
)

// InventoryItem is one tracked asset (a server, a domain, ...) in api.db.
// It is a CMDB-style record, separate from the functional servers/sites in
// inventory.db; SiteID/ServerID are optional soft references to those.
// Details holds the free-form manual fields (no secrets, at most usernames);
// ProviderData holds the latest snapshot from a linked API and is only
// written by syncs. See docs/inventory.md.
type InventoryItem struct {
	ID           string              `json:"id"`
	Type         InventoryItemType   `json:"type"`
	Name         string              `json:"name"`
	Origin       InventoryItemOrigin `json:"origin"`
	Status       InventoryItemStatus `json:"status"`
	Description  string              `json:"description"`
	Tags         []string            `json:"tags"`
	Details      map[string]any      `json:"details"`
	ProviderData map[string]any      `json:"provider_data"`
	SiteID       *string             `json:"site_id,omitempty"`
	ServerID     *string             `json:"server_id,omitempty"`
	Links        []InventoryItemLink `json:"links,omitempty"`
	AuditFields
}

// InventoryItemLink ties an inventory item to one entity of an external API
// (a SpinupWP server, a SpinupWP site's domain, ...). The unique
// (provider, provider_type, provider_id) key is how a sync finds the item an
// external entity belongs to.
type InventoryItemLink struct {
	ID           int       `json:"id"`
	ItemID       string    `json:"item_id"`
	Provider     string    `json:"provider"`
	ProviderType string    `json:"provider_type"`
	ProviderID   string    `json:"provider_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// InventoryLinkDismissal records that the "link / dismiss" prompt for one
// item against one provider link was answered with "dismiss".
type InventoryLinkDismissal struct {
	ItemID       string    `json:"item_id"`
	Provider     string    `json:"provider"`
	ProviderType string    `json:"provider_type"`
	ProviderID   string    `json:"provider_id"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
}
