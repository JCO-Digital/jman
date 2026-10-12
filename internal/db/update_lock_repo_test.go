package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestUpdateLocks(t *testing.T) {
	setupTaskRepoTest(t)

	siteLock := models.UpdateLock{SiteID: testSiteID(1), Comment: "manually managed", CreatedBy: "alice"}
	pluginLock := models.UpdateLock{SiteID: testSiteID(1), Plugin: "woocommerce", Comment: "checkout", CreatedBy: "alice"}
	other := models.UpdateLock{SiteID: testSiteID(2), Plugin: "woocommerce", CreatedBy: "bob"}
	for _, l := range []*models.UpdateLock{&siteLock, &pluginLock, &other} {
		if err := SaveUpdateLock(l); err != nil {
			t.Fatal(err)
		}
	}

	locks, err := GetSiteLocks(testSiteID(1))
	if err != nil {
		t.Fatal(err)
	}
	if locks.Site == nil || locks.Site.Comment != "manually managed" || len(locks.Plugins) != 1 {
		t.Fatalf("site locks = %+v", locks)
	}
	if !locks.CoreLocked() || !locks.PluginLocked("akismet") {
		t.Error("a site lock must lock core and every plugin")
	}

	// Locking again replaces the comment and author instead of adding a lock.
	again := models.UpdateLock{SiteID: testSiteID(1), Plugin: "woocommerce", Comment: "new reason", CreatedBy: "bob"}
	if err := SaveUpdateLock(&again); err != nil {
		t.Fatal(err)
	}
	if again.ID != pluginLock.ID {
		t.Errorf("relocking got ID %d, want existing %d", again.ID, pluginLock.ID)
	}
	if all, _ := ListUpdateLocks(); len(all) != 3 {
		t.Errorf("got %d locks, want 3", len(all))
	}

	// Removing the site lock keeps the plugin lock.
	if found, err := DeleteUpdateLock(siteLock.ID); err != nil || !found {
		t.Fatalf("delete = %v, %v", found, err)
	}
	locks, _ = GetSiteLocks(testSiteID(1))
	if locks.CoreLocked() || locks.PluginLocked("akismet") || !locks.PluginLocked("woocommerce") {
		t.Errorf("after removing the site lock: %+v", locks)
	}
	if locks.Plugins["woocommerce"].Comment != "new reason" || locks.Plugins["woocommerce"].CreatedBy != "bob" {
		t.Errorf("plugin lock = %+v", locks.Plugins["woocommerce"])
	}

	if found, err := DeleteUpdateLock(siteLock.ID); err != nil || found {
		t.Errorf("deleting again = %v, %v; want not found", found, err)
	}
	if l, err := GetUpdateLock(other.ID); err != nil || l == nil || l.CreatedBy != "bob" {
		t.Errorf("GetUpdateLock = %+v, %v", l, err)
	}
}
