package vuln

import (
	"reflect"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestPlanCoreVulnIDMigration(t *testing.T) {
	core := models.VulnExposureCore
	slug := models.CoreExposureSlug
	versions := map[string]coreVersionVulns{
		"6.9.10": {FeedUUID: "feed-6910", IDs: []string{"a", "b"}, Cvss: map[string]float64{"b": 8.1}},
		// 7.0.7's exposure still lists the feed UUID, which the feed no
		// longer reports.
		"7.0.7": {IDs: []string{"b", "c"}, Cvss: map[string]float64{"b": 8.1}},
	}
	exposures := []models.VulnExposure{
		{Kind: core, Slug: slug, SiteID: "s1", Version: "6.9.10", VulnUUIDs: []string{"feed-6910"}},
		{Kind: core, Slug: slug, SiteID: "s2", Version: "7.0.7", VulnUUIDs: []string{"feed-707"}},
		{Kind: "plugin", Slug: "akismet", SiteID: "s1", Version: "1.0", VulnUUIDs: []string{"plugin-uuid"}},
	}
	mutes := []models.VulnMute{
		{ID: 1, Kind: core, Slug: slug, SiteID: "s1", VulnUUIDsAtMute: []string{"feed-6910"}},
		{ID: 2, Kind: core, Slug: slug, SiteID: "s2", VulnUUIDsAtMute: []string{"feed-707"}},
		// Already on the new UUIDs: unchanged.
		{ID: 3, Kind: core, Slug: slug, SiteID: "s3", VulnUUIDsAtMute: []string{"a"}},
		{ID: 4, Kind: "plugin", Slug: "akismet", SiteID: "s1", VulnUUIDsAtMute: []string{"plugin-uuid"}},
	}
	alerted := map[string]bool{"feed-6910": true, "plugin-uuid": true}
	alertSites := map[db.VulnAlertSite]bool{
		{Kind: core, Slug: slug, SiteID: "s1", UUID: "feed-6910"}:            true,
		{Kind: core, Slug: slug, SiteID: "s2", UUID: "feed-707"}:             true,
		{Kind: core, Slug: slug, SiteID: "s2", UUID: "c"}:                    true,
		{Kind: "plugin", Slug: "akismet", SiteID: "s1", UUID: "plugin-uuid"}: true,
	}

	plan := planCoreVulnIDMigration(exposures, versions, mutes, alerted, alertSites)

	gotMutes := map[int64][]string{}
	for _, m := range plan.Mutes {
		gotMutes[m.ID] = m.VulnUUIDsAtMute
	}
	wantMutes := map[int64][]string{1: {"a", "b"}, 2: {"b", "c"}}
	if !reflect.DeepEqual(gotMutes, wantMutes) {
		t.Errorf("mutes = %v, want %v", gotMutes, wantMutes)
	}

	// Only 6.9.10's feed UUID was alerted.
	wantAlerts := map[string]float64{"a": 0, "b": 8.1}
	if !reflect.DeepEqual(plan.Alerts, wantAlerts) {
		t.Errorf("alerts = %v, want %v", plan.Alerts, wantAlerts)
	}

	// s2 already had "c" recorded.
	wantSites := []db.VulnAlertSite{
		{Kind: core, Slug: slug, SiteID: "s1", UUID: "a"},
		{Kind: core, Slug: slug, SiteID: "s1", UUID: "b"},
		{Kind: core, Slug: slug, SiteID: "s2", UUID: "b"},
	}
	if !reflect.DeepEqual(plan.AlertSites, wantSites) {
		t.Errorf("alert sites = %v, want %v", plan.AlertSites, wantSites)
	}

	// After the migration the muted exposures no longer lift their mutes.
	for _, m := range plan.Mutes {
		e := models.VulnExposure{Kind: core, Slug: slug, SiteID: m.SiteID, VulnUUIDs: versions[map[string]string{"s1": "6.9.10", "s2": "7.0.7"}[m.SiteID]].IDs}
		if reason := muteLiftReason(m, e); reason != "" {
			t.Errorf("mute %d would be lifted: %s", m.ID, reason)
		}
	}
}

func TestPlanCoreVulnIDMigrationNothingStored(t *testing.T) {
	plan := planCoreVulnIDMigration(nil, map[string]coreVersionVulns{}, nil, map[string]bool{}, map[db.VulnAlertSite]bool{})
	if len(plan.Mutes) != 0 || len(plan.Alerts) != 0 || len(plan.AlertSites) != 0 {
		t.Errorf("plan = %+v, want nothing", plan)
	}
}
