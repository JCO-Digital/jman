package utils

import (
	"testing"
)

func TestDeterministicUUID(t *testing.T) {
	id1 := SpinupWPSiteUUID(1234)
	id2 := SpinupWPSiteUUID(1234)
	if id1 != id2 {
		t.Fatalf("expected deterministic UUID, got %s != %s", id1, id2)
	}

	if !IsValidUUID(id1) {
		t.Fatalf("expected valid UUID, got %s", id1)
	}

	idOther := SpinupWPSiteUUID(5678)
	if id1 == idOther {
		t.Fatalf("different site IDs should produce different UUIDs")
	}

	serverID := SpinupWPServerUUID(1234)
	if id1 == serverID {
		t.Fatalf("site and server namespaces should produce different UUIDs for same numeric ID")
	}
}

func TestNewV7UUID(t *testing.T) {
	id1 := NewV7UUID()
	id2 := NewV7UUID()
	if id1 == id2 {
		t.Fatalf("consecutive UUIDv7s should be unique")
	}
	if !IsValidUUID(id1) || !IsValidUUID(id2) {
		t.Fatalf("expected valid UUIDs: %s, %s", id1, id2)
	}
}
