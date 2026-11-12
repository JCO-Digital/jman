package utils

import (
	"strconv"

	"github.com/google/uuid"
)

var (
	// SpinupWPSiteNamespace is the deterministic UUIDv5 namespace for SpinupWP site IDs.
	SpinupWPSiteNamespace = uuid.MustParse("e0f4f9a0-6b6f-4a0e-9e7f-8c3b2e5a1d01")
	// SpinupWPServerNamespace is the deterministic UUIDv5 namespace for SpinupWP server IDs.
	SpinupWPServerNamespace = uuid.MustParse("e0f4f9a0-6b6f-4a0e-9e7f-8c3b2e5a1d02")
	// InventoryItemNamespace is the deterministic UUIDv5 namespace for provider-linked inventory items.
	InventoryItemNamespace = uuid.MustParse("e0f4f9a0-6b6f-4a0e-9e7f-8c3b2e5a1d03")
)

// SpinupWPSiteUUID generates a deterministic UUIDv5 for a SpinupWP site ID.
func SpinupWPSiteUUID(spinupID int) string {
	return uuid.NewSHA1(SpinupWPSiteNamespace, []byte(strconv.Itoa(spinupID))).String()
}

// SpinupWPServerUUID generates a deterministic UUIDv5 for a SpinupWP server ID.
func SpinupWPServerUUID(spinupID int) string {
	return uuid.NewSHA1(SpinupWPServerNamespace, []byte(strconv.Itoa(spinupID))).String()
}

// InventoryItemUUID generates a deterministic UUIDv5 for the inventory item
// linked to one external entity (provider + provider type + provider ID).
func InventoryItemUUID(provider, providerType, providerID string) string {
	return uuid.NewSHA1(InventoryItemNamespace, []byte(provider+"\x00"+providerType+"\x00"+providerID)).String()
}

// NewV7UUID generates a new time-ordered UUIDv7 as a string.
// If UUIDv7 generation fails, it falls back to UUIDv4.
func NewV7UUID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

// IsValidUUID checks if a string is a valid UUID format.
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
