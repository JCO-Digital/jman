package db

import "testing"

func TestSaveAndGetSiteAdminUser(t *testing.T) {
	setupTaskRepoTest(t)

	if _, _, found, err := GetSiteAdminUser(1); err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	} else if found {
		t.Fatalf("expected no cached admin user before saving")
	}

	if err := SaveSiteAdminUser(1, 5); err != nil {
		t.Fatalf("failed to save admin user: %v", err)
	}

	userID, updatedAt, found, err := GetSiteAdminUser(1)
	if err != nil {
		t.Fatalf("failed to get admin user: %v", err)
	}
	if !found {
		t.Fatalf("expected a cached admin user after saving")
	}
	if userID != 5 {
		t.Fatalf("expected user ID 5, got %d", userID)
	}
	if updatedAt == "" {
		t.Fatalf("expected a non-empty updated_at timestamp")
	}

	if err := SaveSiteAdminUser(1, 7); err != nil {
		t.Fatalf("failed to update admin user: %v", err)
	}
	userID, _, found, err = GetSiteAdminUser(1)
	if err != nil {
		t.Fatalf("failed to get admin user after update: %v", err)
	}
	if !found || userID != 7 {
		t.Fatalf("expected updated user ID 7, got %d (found=%v)", userID, found)
	}
}
