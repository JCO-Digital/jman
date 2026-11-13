package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const inventoryDomainLookupColumns = `item_id, domain, nameservers, registrar, expires_at, source, error, checked_at, last_success_at`

func scanInventoryDomainLookup(scanner interface{ Scan(...any) error }) (*models.InventoryDomainLookup, error) {
	var l models.InventoryDomainLookup
	var nameservers string
	var expiresAt, lastSuccessAt sql.NullTime
	if err := scanner.Scan(
		&l.ItemID, &l.Domain, &nameservers, &l.Registrar, &expiresAt,
		&l.Source, &l.Error, &l.CheckedAt, &lastSuccessAt,
	); err != nil {
		return nil, err
	}
	l.Nameservers = []string{}
	if err := unmarshalJSON(nameservers, &l.Nameservers); err != nil {
		return nil, fmt.Errorf("failed to decode nameservers of item %s: %w", l.ItemID, err)
	}
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		l.ExpiresAt = &t
	}
	if lastSuccessAt.Valid {
		t := lastSuccessAt.Time.UTC()
		l.LastSuccessAt = &t
	}
	l.CheckedAt = l.CheckedAt.UTC()
	return &l, nil
}

// GetInventoryDomainLookup returns the registry lookup of one domain item,
// or nil when it has not been looked up yet.
func GetInventoryDomainLookup(itemID string) (*models.InventoryDomainLookup, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	l, err := scanInventoryDomainLookup(db.QueryRow(
		`SELECT `+inventoryDomainLookupColumns+` FROM inventory_domain_lookups WHERE item_id = ?`, itemID,
	))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get domain lookup: %w", err)
	}
	return l, nil
}

// SaveInventoryDomainLookup inserts or replaces the lookup of one item.
func SaveInventoryDomainLookup(l *models.InventoryDomainLookup) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	nameservers, err := marshalJSON(l.Nameservers, "[]")
	if err != nil {
		return fmt.Errorf("failed to encode nameservers: %w", err)
	}
	_, err = db.Exec(
		`INSERT INTO inventory_domain_lookups (`+inventoryDomainLookupColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_id) DO UPDATE SET
			domain = excluded.domain, nameservers = excluded.nameservers,
			registrar = excluded.registrar, expires_at = excluded.expires_at,
			source = excluded.source, error = excluded.error,
			checked_at = excluded.checked_at, last_success_at = excluded.last_success_at`,
		l.ItemID, l.Domain, nameservers, l.Registrar, l.ExpiresAt,
		l.Source, l.Error, l.CheckedAt, l.LastSuccessAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save domain lookup: %w", err)
	}
	return nil
}

// DueDomainLookup is an active domain item whose registry lookup is due.
type DueDomainLookup struct {
	ItemID string
	Domain string
}

// ListDueInventoryDomainLookups returns up to limit active domain items
// whose lookup is due: never looked up, looked up under another name, last
// successful before okBefore, or last failed before failedBefore. The ones
// waiting longest come first.
func ListDueInventoryDomainLookups(okBefore, failedBefore time.Time, limit int) ([]DueDomainLookup, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT i.id, i.name FROM inventory_items i
		LEFT JOIN inventory_domain_lookups l ON l.item_id = i.id
		WHERE i.type = ? AND i.status = ?
		  AND (l.item_id IS NULL
		    OR l.domain != i.name
		    OR (l.error = '' AND l.checked_at < ?)
		    OR (l.error != '' AND l.checked_at < ?))
		ORDER BY l.checked_at IS NOT NULL, l.checked_at ASC, i.name ASC
		LIMIT ?`,
		models.InventoryItemTypeDomain, models.InventoryItemStatusActive,
		okBefore, failedBefore, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list due domain lookups: %w", err)
	}
	defer rows.Close()
	due := []DueDomainLookup{}
	for rows.Next() {
		var d DueDomainLookup
		if err := rows.Scan(&d.ItemID, &d.Domain); err != nil {
			return nil, err
		}
		due = append(due, d)
	}
	return due, rows.Err()
}

// ListInventoryDomainLookups returns every stored registry lookup, keyed by
// item ID, for the inventory list.
func ListInventoryDomainLookups() (map[string]*models.InventoryDomainLookup, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(`SELECT ` + inventoryDomainLookupColumns + ` FROM inventory_domain_lookups`)
	if err != nil {
		return nil, fmt.Errorf("failed to list domain lookups: %w", err)
	}
	defer rows.Close()
	lookups := map[string]*models.InventoryDomainLookup{}
	for rows.Next() {
		l, err := scanInventoryDomainLookup(rows)
		if err != nil {
			return nil, err
		}
		lookups[l.ItemID] = l
	}
	return lookups, rows.Err()
}
