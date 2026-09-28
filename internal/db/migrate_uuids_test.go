package db

import (
	"encoding/json"
	"testing"

	"github.com/JCO-Digital/jman/internal/utils"
)

func TestMigrateLegacyInventoryIDs(t *testing.T) {
	setupTaskRepoTest(t)
	invDB := GetInventoryDB()

	// Insert legacy rows with integer site_ids
	legacySiteID := 42
	expectedUUID := utils.SpinupWPSiteUUID(legacySiteID)

	_, err := invDB.Exec(`
		INSERT INTO site_core (site_id, version, minor_update, major_update)
		VALUES (?, '6.5.0', '6.5.1', '')
	`, legacySiteID)
	if err != nil {
		t.Fatalf("failed to insert legacy site_core row: %v", err)
	}

	_, err = invDB.Exec(`
		INSERT INTO site_environment (site_id, environment)
		VALUES (?, 'production')
	`, legacySiteID)
	if err != nil {
		t.Fatalf("failed to insert legacy site_environment row: %v", err)
	}

	_, err = invDB.Exec(`
		INSERT INTO ignore_entries (type, target, reason, negated_site_ids)
		VALUES ('site', '42', 'test', '[42, 99]')
	`)
	if err != nil {
		t.Fatalf("failed to insert legacy ignore_entries row: %v", err)
	}

	// Insert duplicate in site_plugins: one legacy and one already-migrated UUID
	_, err = invDB.Exec(`
		INSERT INTO site_plugins (site_id, slug, status, version)
		VALUES (?, 'akismet', 'active', '5.0')
	`, legacySiteID)
	if err != nil {
		t.Fatalf("failed to insert legacy site_plugins row: %v", err)
	}
	_, err = invDB.Exec(`
		INSERT INTO site_plugins (site_id, slug, status, version)
		VALUES (?, 'akismet', 'active', '5.1')
	`, expectedUUID)
	if err != nil {
		t.Fatalf("failed to insert migrated site_plugins row: %v", err)
	}
	_, err = invDB.Exec(`
		INSERT INTO site_plugins (site_id, slug, status, version)
		VALUES (?, 'yoast', 'active', '20.0')
	`, legacySiteID)
	if err != nil {
		t.Fatalf("failed to insert second legacy site_plugins row: %v", err)
	}

	// Run migration
	if err := MigrateLegacyInventoryIDs(invDB); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify site_plugins: akismet should have version 5.1 and yoast should be migrated
	var akismetVer, yoastSiteID string
	err = invDB.QueryRow(`SELECT version FROM site_plugins WHERE site_id = ? AND slug = 'akismet'`, expectedUUID).Scan(&akismetVer)
	if err != nil || akismetVer != "5.1" {
		t.Fatalf("expected akismet version 5.1, got %s (err: %v)", akismetVer, err)
	}
	err = invDB.QueryRow(`SELECT site_id FROM site_plugins WHERE slug = 'yoast'`).Scan(&yoastSiteID)
	if err != nil || yoastSiteID != expectedUUID {
		t.Fatalf("expected yoast site_id %s, got %s (err: %v)", expectedUUID, yoastSiteID, err)
	}
	var remainingLegacy int
	_ = invDB.QueryRow(`SELECT count(*) FROM site_plugins WHERE site_id = ?`, legacySiteID).Scan(&remainingLegacy)
	if remainingLegacy != 0 {
		t.Fatalf("expected 0 remaining legacy site_plugins rows, got %d", remainingLegacy)
	}

	// Verify site_core
	var coreSiteID string
	err = invDB.QueryRow(`SELECT site_id FROM site_core WHERE version = '6.5.0'`).Scan(&coreSiteID)
	if err != nil || coreSiteID != expectedUUID {
		t.Fatalf("expected site_core site_id %s, got %s (err: %v)", expectedUUID, coreSiteID, err)
	}

	// Verify site_environment
	var envSiteID string
	err = invDB.QueryRow(`SELECT site_id FROM site_environment WHERE environment = 'production'`).Scan(&envSiteID)
	if err != nil || envSiteID != expectedUUID {
		t.Fatalf("expected site_environment site_id %s, got %s (err: %v)", expectedUUID, envSiteID, err)
	}

	// Verify ignore_entries
	var target, negIDs string
	err = invDB.QueryRow(`SELECT target, negated_site_ids FROM ignore_entries WHERE type = 'site'`).Scan(&target, &negIDs)
	if err != nil || target != expectedUUID {
		t.Fatalf("expected ignore target %s, got %s (err: %v)", expectedUUID, target, err)
	}

	var parsedNegIDs []string
	if err := json.Unmarshal([]byte(negIDs), &parsedNegIDs); err != nil {
		t.Fatalf("failed to parse negated_site_ids JSON: %v", err)
	}
	if len(parsedNegIDs) != 2 || parsedNegIDs[0] != expectedUUID || parsedNegIDs[1] != utils.SpinupWPSiteUUID(99) {
		t.Fatalf("unexpected migrated negated_site_ids: %+v", parsedNegIDs)
	}

	// Idempotency: running it a second time changes nothing
	if err := MigrateLegacyInventoryIDs(invDB); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}
}

func TestMigrateLegacyAPIIDs(t *testing.T) {
	setupTaskRepoTest(t)
	apiDB := GetAPIDB()

	legacySiteID := 55
	legacyServerID := 12
	expectedSiteUUID := utils.SpinupWPSiteUUID(legacySiteID)
	expectedServerUUID := utils.SpinupWPServerUUID(legacyServerID)

	_, err := apiDB.Exec(`
		INSERT INTO site_wp_flags (site_id, is_multisite, disallow_file_mods)
		VALUES (?, 0, 1)
	`, legacySiteID)
	if err != nil {
		t.Fatalf("failed to insert legacy site_wp_flags row: %v", err)
	}

	_, err = apiDB.Exec(`
		INSERT INTO tasks (title, site_id, server_id, type, status, priority)
		VALUES ('Test task', ?, ?, 'one_off', 'pending', 'medium')
	`, legacySiteID, legacyServerID)
	if err != nil {
		t.Fatalf("failed to insert legacy tasks row: %v", err)
	}

	// Run migration
	if err := MigrateLegacyAPIIDs(apiDB); err != nil {
		t.Fatalf("API migration failed: %v", err)
	}

	// Verify site_wp_flags
	var flagsSiteID string
	err = apiDB.QueryRow(`SELECT site_id FROM site_wp_flags`).Scan(&flagsSiteID)
	if err != nil || flagsSiteID != expectedSiteUUID {
		t.Fatalf("expected site_wp_flags site_id %s, got %s (err: %v)", expectedSiteUUID, flagsSiteID, err)
	}

	// Verify tasks
	var taskSiteID, taskServerID string
	err = apiDB.QueryRow(`SELECT site_id, server_id FROM tasks WHERE title = 'Test task'`).Scan(&taskSiteID, &taskServerID)
	if err != nil || taskSiteID != expectedSiteUUID || taskServerID != expectedServerUUID {
		t.Fatalf("expected tasks site_id %s server_id %s, got site_id %s server_id %s (err: %v)",
			expectedSiteUUID, expectedServerUUID, taskSiteID, taskServerID, err)
	}
}
