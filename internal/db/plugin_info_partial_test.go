package db

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestSavePluginInfoPartialKeepsFetchedAt(t *testing.T) {
	old := config.RunData
	config.RunData.DataDir = t.TempDir()
	if err := InitInventory(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(func() { _ = Close(); config.RunData = old })

	// A plugin first seen in a site's plugin list is due for a full fetch.
	if err := SavePluginInfoPartial(models.PluginInfo{Slug: "foo", Name: "foo", Version: "1.0"}); err != nil {
		t.Fatalf("partial insert: %v", err)
	}
	_, fetchedAt, err := GetPluginInfo("foo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetchedAt.Year() != 1970 {
		t.Errorf("new partial row fetched_at = %v, want the Unix epoch", fetchedAt)
	}

	// After a full fetch, partial updates keep its time.
	if err := SavePluginInfo(models.PluginInfo{Slug: "foo", Name: "Foo", Version: "1.1", Author: "Ann"}); err != nil {
		t.Fatalf("full save: %v", err)
	}
	_, fullAt, _ := GetPluginInfo("foo")
	if time.Since(fullAt) > time.Hour {
		t.Fatalf("full save fetched_at = %v, want now", fullAt)
	}
	if _, err := GetInventoryDB().Exec(`UPDATE plugin_info SET fetched_at = '2026-01-02 03:04:05' WHERE slug = 'foo'`); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if err := SavePluginInfoPartial(models.PluginInfo{Slug: "foo", Name: "Foo", Version: "1.2", Author: "Ann"}); err != nil {
		t.Fatalf("partial update: %v", err)
	}
	info, at, _ := GetPluginInfo("foo")
	if want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC); !at.Equal(want) {
		t.Errorf("fetched_at after partial update = %v, want %v", at, want)
	}
	if info.Version != "1.2" || info.Author != "Ann" {
		t.Errorf("partial update saved %+v", info)
	}
}
