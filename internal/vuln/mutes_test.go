package vuln

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestSnapshotMute(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	uuids := []string{"a"}
	m := models.VulnMute{Status: models.VulnMuteNeedsReview}
	SnapshotMute(&m, models.VulnExposure{FixState: models.VulnNoFix, VulnUUIDs: uuids, PossibleFixVersion: "2.0"}, now)
	uuids[0] = "changed"
	if m.Status != models.VulnMuteActive || m.FixStateAtMute != models.VulnNoFix || m.VulnUUIDsAtMute[0] != "a" ||
		m.PossibleFixAtMute != "2.0" || !m.ReviewAt.Equal(now.AddDate(0, 0, 90)) {
		t.Errorf("snapshot = %+v", m)
	}
}

func TestPlanMuteLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	mute := func(id int64, slug, fixState string, uuids []string, possible string, review time.Time) models.VulnMute {
		return models.VulnMute{ID: id, Kind: "plugin", Slug: slug, SiteID: "s", Status: models.VulnMuteActive,
			FixStateAtMute: fixState, VulnUUIDsAtMute: uuids, PossibleFixAtMute: possible, ReviewAt: review}
	}
	exposure := func(slug, fixState string, uuids []string, possible string) models.VulnExposure {
		return models.VulnExposure{Kind: "plugin", Slug: slug, SiteID: "s", FixState: fixState, FixedIn: "3.0",
			VulnUUIDs: uuids, PossibleFixVersion: possible}
	}

	mutes := []models.VulnMute{
		mute(1, "same", models.VulnNoFix, []string{"a"}, "", later),
		mute(2, "newvuln", models.VulnFixAvailable, []string{"a"}, "", later),
		mute(3, "fixed", models.VulnNoFix, []string{"a"}, "", later),
		mute(4, "possible", models.VulnNoFix, []string{"a"}, "1.1", later),
		// A fix-available mute isn't lifted by a newer possible fix.
		mute(5, "fixable", models.VulnFixAvailable, []string{"a"}, "", later),
		mute(6, "gone", models.VulnNoFix, []string{"a"}, "", later),
		mute(7, "flapping", models.VulnNoFix, []string{"a"}, "", later),
		mute(8, "review", models.VulnNoFix, []string{"a"}, "", past),
		{ID: 9, Kind: "plugin", Slug: "lifted", SiteID: "s", Status: models.VulnMuteLifted},
	}
	open := map[exposureKey]models.VulnExposure{}
	for _, e := range []models.VulnExposure{
		exposure("same", models.VulnNoFix, []string{"a"}, ""),
		exposure("newvuln", models.VulnFixAvailable, []string{"a", "b"}, ""),
		exposure("fixed", models.VulnFixAvailable, []string{"a"}, ""),
		exposure("possible", models.VulnNoFix, []string{"a"}, "1.2"),
		exposure("fixable", models.VulnFixAvailable, []string{"a"}, "9.9"),
		exposure("review", models.VulnNoFix, []string{"a"}, ""),
	} {
		open[keyOf(e)] = e
	}
	recent := map[exposureKey]bool{{"plugin", "flapping", "s"}: true}

	changed := map[int64]models.VulnMute{}
	for _, m := range planMuteLifecycle(mutes, open, recent, now) {
		changed[m.ID] = m
	}

	want := map[int64]struct{ status, reason string }{
		2: {models.VulnMuteLifted, "A new vulnerability applies"},
		3: {models.VulnMuteLifted, "A fix is available (3.0)"},
		4: {models.VulnMuteLifted, "Version 1.2 may fix it"},
		6: {models.VulnMuteLifted, "No longer affected"},
		8: {models.VulnMuteNeedsReview, ""},
	}
	if len(changed) != len(want) {
		t.Errorf("changed mutes = %v, want %v", keysOf(changed), want)
	}
	for id, w := range want {
		m, ok := changed[id]
		if !ok {
			t.Errorf("mute %d unchanged, want %s", id, w.status)
			continue
		}
		if m.Status != w.status || m.LiftedReason != w.reason || m.UpdatedBy != "system" {
			t.Errorf("mute %d = %s %q by %q, want %s %q", id, m.Status, m.LiftedReason, m.UpdatedBy, w.status, w.reason)
		}
		if w.status == models.VulnMuteLifted && (m.LiftedAt == nil || !m.LiftedAt.Equal(now)) {
			t.Errorf("mute %d lifted_at = %v, want now", id, m.LiftedAt)
		}
	}
}

func keysOf(m map[int64]models.VulnMute) []int64 {
	var ids []int64
	for id := range m {
		ids = append(ids, id)
	}
	return ids
}

func TestGroupAndSummaryWithMutes(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	longAgo := now.AddDate(0, 0, -60)
	exposures := []models.VulnExposure{
		{Kind: "plugin", Slug: "p", SiteID: "a", MaxCvss: fp(8), FixState: models.VulnFixAvailable, FixAvailableSince: &longAgo},
		{Kind: "plugin", Slug: "p", SiteID: "b", MaxCvss: fp(8), FixState: models.VulnFixAvailable, FixAvailableSince: &longAgo},
		{Kind: "plugin", Slug: "p", SiteID: "c", FixState: models.VulnNoFix},
	}
	mutes := []models.VulnMute{
		{Kind: "plugin", Slug: "p", SiteID: "a", Status: models.VulnMuteActive},
		{Kind: "plugin", Slug: "p", SiteID: "c", Status: models.VulnMuteNeedsReview},
	}
	byKey := map[exposureKey]models.VulnMute{}
	for _, m := range mutes {
		byKey[exposureKey{m.Kind, m.Slug, m.SiteID}] = m
	}
	details := func(kind, slug, coreVersion string) groupDetails { return groupDetails{Name: slug} }

	groups := groupExposures(exposures, map[string]string{"a": "Alpha", "b": "Bravo", "c": "Charlie"}, byKey, details, now)
	if len(groups) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	g := groups[0]
	if g.OverdueCount != 1 || g.MutedCount != 2 {
		t.Errorf("overdue %d muted %d, want 1 and 2", g.OverdueCount, g.MutedCount)
	}
	var order []string
	for _, s := range g.Sites {
		order = append(order, s.SiteName)
	}
	if order[0] != "Bravo" || g.Sites[0].Mute != nil || g.Sites[1].Mute == nil || g.Sites[2].Mute == nil {
		t.Errorf("site order = %v, want the unmuted Bravo first and muted sites last", order)
	}

	s := SummarizeExposures(exposures, mutes, now)
	if s != (models.VulnExposureSummary{Groups: 1, Exposures: 3, Overdue: 1, NoFix: 0, Muted: 2, NeedsReview: 1}) {
		t.Errorf("summary = %+v", s)
	}
}
