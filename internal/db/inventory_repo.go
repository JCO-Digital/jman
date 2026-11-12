package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// InventoryItemFilter narrows ListInventoryItems. Empty fields match everything.
type InventoryItemFilter struct {
	Type   string
	Status string
	Search string
}

// marshalJSON encodes v, using fallback for nil values (including typed nil
// maps/slices, which are not == nil) so the NOT NULL JSON columns never get
// "null" or an empty string.
func marshalJSON(v any, fallback string) (string, error) {
	switch t := v.(type) {
	case nil:
		return fallback, nil
	case map[string]any:
		if t == nil {
			return fallback, nil
		}
	case []string:
		if t == nil {
			return fallback, nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// unmarshalJSON decodes s into v, tolerating empty/NULL columns and the
// literal "null" (which would otherwise nil out the caller's map/slice).
func unmarshalJSON(s string, v any) error {
	if s == "" || s == "null" {
		return nil
	}
	return json.Unmarshal([]byte(s), v)
}

// inventoryItemJSON encodes the three JSON columns of an item.
func inventoryItemJSON(item *models.InventoryItem) (tags, details, providerData string, err error) {
	tags, err = marshalJSON(item.Tags, "[]")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to encode tags: %w", err)
	}
	details, err = marshalJSON(item.Details, "{}")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to encode details: %w", err)
	}
	providerData, err = marshalJSON(item.ProviderData, "{}")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to encode provider_data: %w", err)
	}
	return tags, details, providerData, nil
}

// InsertInventoryItem creates an inventory item. A new item gets a UUIDv7
// when it has no ID yet; provider-created items arrive with their
// deterministic UUID already set.
func InsertInventoryItem(item *models.InventoryItem, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	tags, details, providerData, err := inventoryItemJSON(item)
	if err != nil {
		return err
	}
	if item.ID == "" {
		item.ID = utils.NewV7UUID()
	}
	if item.Origin == "" {
		item.Origin = models.InventoryItemOriginManual
	}
	if item.Status == "" {
		item.Status = models.InventoryItemStatusActive
	}
	now := time.Now()
	query := `
	INSERT INTO inventory_items (id, type, name, origin, status, description, tags, details, provider_data, site_id, server_id, created_at, created_by, updated_at, updated_by)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = db.Exec(query,
		item.ID, item.Type, item.Name, item.Origin, item.Status, item.Description,
		tags, details, providerData, item.SiteID, item.ServerID,
		now, username, now, username,
	)
	if err != nil {
		return fmt.Errorf("failed to insert inventory item: %w", err)
	}
	item.CreatedAt = now
	item.CreatedBy = username
	item.UpdatedAt = now
	item.UpdatedBy = username
	return nil
}

// UpdateInventoryItem saves a manual edit: the fields a user owns. It does
// not write status or provider_data, which belong to the provider sync
// (UpdateInventoryItemProviderFields), so an edit can't overwrite a sync
// that ran since the item was loaded. The origin is never changed.
func UpdateInventoryItem(item *models.InventoryItem, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	tags, details, _, err := inventoryItemJSON(item)
	if err != nil {
		return err
	}
	now := time.Now()
	query := `
	UPDATE inventory_items SET type = ?, name = ?, description = ?, tags = ?, details = ?, site_id = ?, server_id = ?, updated_at = ?, updated_by = ?
	WHERE id = ?
	`
	res, err := db.Exec(query,
		item.Type, item.Name, item.Description,
		tags, details, item.SiteID, item.ServerID,
		now, username, item.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update inventory item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("inventory item %s not found", item.ID)
	}
	item.UpdatedAt = now
	item.UpdatedBy = username
	return nil
}

// UpdateInventoryItemProviderFields saves what a provider sync owns: name,
// status, provider_data and the site link. It leaves updated_at/updated_by
// alone, so they keep showing the last manual edit.
func UpdateInventoryItemProviderFields(item *models.InventoryItem) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	providerData, err := marshalJSON(item.ProviderData, "{}")
	if err != nil {
		return fmt.Errorf("failed to encode provider_data: %w", err)
	}
	res, err := db.Exec(
		`UPDATE inventory_items SET name = ?, status = ?, provider_data = ?, site_id = ? WHERE id = ?`,
		item.Name, item.Status, providerData, item.SiteID, item.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update inventory item provider fields: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("inventory item %s not found", item.ID)
	}
	return nil
}

const inventoryItemColumns = `id, type, name, origin, status, description, tags, details, provider_data, site_id, server_id, created_at, created_by, updated_at, updated_by`

// scanInventoryItem reads one inventory_items row in inventoryItemColumns order.
func scanInventoryItem(scanner interface{ Scan(...any) error }) (*models.InventoryItem, error) {
	var it models.InventoryItem
	var tags, details, providerData string
	var siteID, serverID, createdBy, updatedBy sql.NullString
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(
		&it.ID, &it.Type, &it.Name, &it.Origin, &it.Status, &it.Description,
		&tags, &details, &providerData, &siteID, &serverID,
		&createdAt, &createdBy, &updatedAt, &updatedBy,
	); err != nil {
		return nil, err
	}
	it.Tags = []string{}
	if err := unmarshalJSON(tags, &it.Tags); err != nil {
		return nil, fmt.Errorf("failed to decode tags of item %s: %w", it.ID, err)
	}
	it.Details = map[string]any{}
	if err := unmarshalJSON(details, &it.Details); err != nil {
		return nil, fmt.Errorf("failed to decode details of item %s: %w", it.ID, err)
	}
	it.ProviderData = map[string]any{}
	if err := unmarshalJSON(providerData, &it.ProviderData); err != nil {
		return nil, fmt.Errorf("failed to decode provider_data of item %s: %w", it.ID, err)
	}
	if siteID.Valid {
		it.SiteID = &siteID.String
	}
	if serverID.Valid {
		it.ServerID = &serverID.String
	}
	it.CreatedAt = createdAt
	it.CreatedBy = createdBy.String
	it.UpdatedAt = updatedAt
	it.UpdatedBy = updatedBy.String
	return &it, nil
}

// itemPtrs returns pointers into items' backing array, built after the
// slice is final so appends can't invalidate them.
func itemPtrs(items []models.InventoryItem) []*models.InventoryItem {
	ptrs := make([]*models.InventoryItem, len(items))
	for i := range items {
		ptrs[i] = &items[i]
	}
	return ptrs
}

// attachLinks loads the links of each item with one query. items must be
// pointers into the caller's slice so the links land on the returned items.
func attachLinks(items []*models.InventoryItem) error {
	if len(items) == 0 {
		return nil
	}
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	placeholders := make([]string, len(items))
	args := make([]any, len(items))
	byID := make(map[string]*models.InventoryItem, len(items))
	for i, it := range items {
		placeholders[i] = "?"
		args[i] = it.ID
		byID[it.ID] = it
	}
	rows, err := db.Query(
		`SELECT id, item_id, provider, provider_type, provider_id, created_at FROM inventory_item_links WHERE item_id IN (`+
			strings.Join(placeholders, ",")+`) ORDER BY id`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("failed to query inventory item links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var l models.InventoryItemLink
		var createdAt time.Time
		if err := rows.Scan(&l.ID, &l.ItemID, &l.Provider, &l.ProviderType, &l.ProviderID, &createdAt); err != nil {
			return err
		}
		l.CreatedAt = createdAt
		if it, ok := byID[l.ItemID]; ok {
			it.Links = append(it.Links, l)
		}
	}
	return rows.Err()
}

// GetInventoryItem retrieves an item by ID, with its links.
func GetInventoryItem(id string) (*models.InventoryItem, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	it, err := scanInventoryItem(db.QueryRow(
		`SELECT `+inventoryItemColumns+` FROM inventory_items WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory item: %w", err)
	}
	if err := attachLinks([]*models.InventoryItem{it}); err != nil {
		return nil, err
	}
	return it, nil
}

// ListInventoryItems lists items matching the filter, each with its links,
// ordered by type then name.
func ListInventoryItems(filter InventoryItemFilter) ([]models.InventoryItem, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT ` + inventoryItemColumns + ` FROM inventory_items WHERE 1=1`
	var args []any
	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Search != "" {
		query += " AND (LOWER(name) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?))"
		term := "%" + filter.Search + "%"
		args = append(args, term, term)
	}
	query += " ORDER BY type ASC, name ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory items: %w", err)
	}
	defer rows.Close()

	items := []models.InventoryItem{}
	for rows.Next() {
		it, err := scanInventoryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachLinks(itemPtrs(items)); err != nil {
		return nil, err
	}
	return items, nil
}

// FindInventoryItemsByTypeName returns the items of one type whose name
// equals name case-insensitively. Domain names are expected to already be
// normalized to their apex form.
func FindInventoryItemsByTypeName(itemType models.InventoryItemType, name string) ([]models.InventoryItem, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT `+inventoryItemColumns+` FROM inventory_items WHERE type = ? AND LOWER(name) = LOWER(?) ORDER BY created_at ASC`,
		itemType, name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find inventory items: %w", err)
	}
	defer rows.Close()
	items := []models.InventoryItem{}
	for rows.Next() {
		it, err := scanInventoryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachLinks(itemPtrs(items)); err != nil {
		return nil, err
	}
	return items, nil
}

// DeleteInventoryItem removes an item; its links go with it (ON DELETE
// CASCADE).
func DeleteInventoryItem(id string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM inventory_items WHERE id = ?", id)
	return err
}

// SetInventoryItemStatus updates only an item's status, e.g. to mark a
// provider-linked item removed when its external entity disappeared.
func SetInventoryItemStatus(id string, status models.InventoryItemStatus) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("UPDATE inventory_items SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", status, id)
	return err
}

// --- Links ---

// GetInventoryItemByLink returns the item a provider link belongs to.
func GetInventoryItemByLink(provider, providerType, providerID string) (*models.InventoryItem, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	it, err := scanInventoryItem(db.QueryRow(
		`SELECT i.id, i.type, i.name, i.origin, i.status, i.description, i.tags, i.details, i.provider_data, i.site_id, i.server_id, i.created_at, i.created_by, i.updated_at, i.updated_by
		FROM inventory_items i
		JOIN inventory_item_links l ON l.item_id = i.id
		WHERE l.provider = ? AND l.provider_type = ? AND l.provider_id = ?`,
		provider, providerType, providerID,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory item by link: %w", err)
	}
	return it, nil
}

// ListInventoryItemLinks returns one item's links.
func ListInventoryItemLinks(itemID string) ([]models.InventoryItemLink, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT id, item_id, provider, provider_type, provider_id, created_at FROM inventory_item_links WHERE item_id = ? ORDER BY id`,
		itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInventoryItemLinks(rows)
}

// ListInventoryLinksByProvider returns every link of one provider, for a
// sync to detect which external entities disappeared.
func ListInventoryLinksByProvider(provider string) ([]models.InventoryItemLink, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT id, item_id, provider, provider_type, provider_id, created_at FROM inventory_item_links WHERE provider = ? ORDER BY id`,
		provider,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInventoryItemLinks(rows)
}

func scanInventoryItemLinks(rows *sql.Rows) ([]models.InventoryItemLink, error) {
	links := []models.InventoryItemLink{}
	for rows.Next() {
		var l models.InventoryItemLink
		var createdAt time.Time
		if err := rows.Scan(&l.ID, &l.ItemID, &l.Provider, &l.ProviderType, &l.ProviderID, &createdAt); err != nil {
			return nil, err
		}
		l.CreatedAt = createdAt
		links = append(links, l)
	}
	return links, rows.Err()
}

// AddInventoryItemLink links an external entity to an item. It fails on the
// unique (provider, provider_type, provider_id) key when the entity is
// already linked.
func AddInventoryItemLink(link *models.InventoryItemLink) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	query := `INSERT INTO inventory_item_links (item_id, provider, provider_type, provider_id, created_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP) RETURNING id`
	err := db.QueryRow(query, link.ItemID, link.Provider, link.ProviderType, link.ProviderID).Scan(&link.ID)
	if err != nil {
		return fmt.Errorf("failed to add inventory item link: %w", err)
	}
	return nil
}

// DeleteInventoryItemLink removes the link of one external entity.
func DeleteInventoryItemLink(provider, providerType, providerID string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM inventory_item_links WHERE provider = ? AND provider_type = ? AND provider_id = ?", provider, providerType, providerID)
	return err
}

// --- Match suggestions and dismissals ---

// FindInventoryItemMatches returns the other items that share the given
// item's type and name (case-insensitive) and have at least one provider
// link — the candidates for the "link / dismiss" prompt — excluding matches
// already dismissed for itemID.
func FindInventoryItemMatches(itemID string) ([]models.InventoryItem, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT `+inventoryItemColumns+` FROM inventory_items i
		WHERE i.id != ? AND i.type = (SELECT type FROM inventory_items WHERE id = ?)
		  AND LOWER(i.name) = (SELECT LOWER(name) FROM inventory_items WHERE id = ?)
		  AND EXISTS (SELECT 1 FROM inventory_item_links l WHERE l.item_id = i.id)
		  AND NOT EXISTS (
		    SELECT 1 FROM inventory_link_dismissals d
		    JOIN inventory_item_links l ON l.provider = d.provider AND l.provider_type = d.provider_type AND l.provider_id = d.provider_id
		    WHERE d.item_id = ? AND l.item_id = i.id
		  )
		ORDER BY i.created_at ASC`,
		itemID, itemID, itemID, itemID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find inventory item matches: %w", err)
	}
	defer rows.Close()
	items := []models.InventoryItem{}
	for rows.Next() {
		it, err := scanInventoryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := attachLinks(itemPtrs(items)); err != nil {
		return nil, err
	}
	return items, nil
}

// DismissInventoryLinkMatch records that the match prompt for itemID against
// one provider link was dismissed.
func DismissInventoryLinkMatch(itemID, provider, providerType, providerID, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(
		`INSERT OR IGNORE INTO inventory_link_dismissals (item_id, provider, provider_type, provider_id, created_at, created_by) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
		itemID, provider, providerType, providerID, username,
	)
	return err
}
