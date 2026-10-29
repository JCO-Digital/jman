package vuln

import (
	"reflect"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

// rangeVuln is a vulnerability affecting versions op max (op "lt" or "le").
func rangeVuln(uuid, score, op, max string, unfixed bool, sites ...models.PluginSite) models.Vulnerability {
	v := testVuln(uuid, score, sites...)
	v.Operator = models.Operator{MaxVersion: strp(max), MaxOperator: strp(op)}
	if unfixed {
		v.Operator.Unfixed = "1"
	}
	return v
}

func fp(f float64) *float64 { return &f }

func TestPluginObservations(t *testing.T) {
	reports := []models.VulnReport{
		{Slug: "fixable", Vulnerabilities: []models.Vulnerability{
			rangeVuln("a2", "5.0", "lt", "2.0", false, models.PluginSite{SiteID: "s1", Version: "1.0"}),
			rangeVuln("a1", "8.0", "lt", "1.5", false, models.PluginSite{SiteID: "s1", Version: "1.0"},
				models.PluginSite{SiteID: "s2", Version: "1.0", Suppressed: true}),
		}},
		{Slug: "nofix", Vulnerabilities: []models.Vulnerability{
			rangeVuln("b1", "", "le", "3.0", true, models.PluginSite{SiteID: "s1", Version: "3.0"},
				models.PluginSite{SiteID: "s2", Version: "2.0"}),
		}},
		{Slug: "suppressed", Suppressed: true, Vulnerabilities: []models.Vulnerability{
			rangeVuln("c1", "9.0", "lt", "2.0", false, models.PluginSite{SiteID: "s1", Version: "1.0"}),
		}},
	}
	siteUpdates := map[string]string{"s1\x00nofix": "3.1", "s2\x00nofix": "3.0"}
	latest := map[string]string{"nofix": "2.5"}

	got := pluginObservations(reports, siteUpdates, latest)
	want := map[exposureKey]observedExposure{
		{"plugin", "fixable", "s1"}: {Version: "1.0", VulnUUIDs: []string{"a1", "a2"}, MaxCvss: fp(8),
			FixState: models.VulnFixAvailable, FixedIn: "2.0"},
		// 3.1 is outside b1's <= 3.0 range.
		{"plugin", "nofix", "s1"}: {Version: "3.0", VulnUUIDs: []string{"b1"},
			FixState: models.VulnNoFix, PossibleFixVersion: "3.1"},
		// 3.0 and 2.5 are both still affected.
		{"plugin", "nofix", "s2"}: {Version: "2.0", VulnUUIDs: []string{"b1"}, FixState: models.VulnNoFix},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pluginObservations() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestPluginObservationsUnfixedWithLtBound(t *testing.T) {
	reports := []models.VulnReport{{Slug: "p", Vulnerabilities: []models.Vulnerability{
		rangeVuln("a", "5.0", "lt", "2.0", true, models.PluginSite{SiteID: "s", Version: "1.0"}),
	}}}
	got := pluginObservations(reports, nil, nil)[exposureKey{"plugin", "p", "s"}]
	if got.FixState != models.VulnNoFix || got.FixedIn != "" {
		t.Errorf("unfixed vulnerability: got %+v, want no fix", got)
	}
}

func TestCoreObservations(t *testing.T) {
	reports := []models.CoreVulnReport{{Version: "6.4.1", Vulnerabilities: []models.Vulnerability{
		testVuln("c1", "6.1", models.PluginSite{SiteID: "minor"}, models.PluginSite{SiteID: "major"},
			models.PluginSite{SiteID: "none"}, models.PluginSite{SiteID: "ignored", Suppressed: true}),
	}}}
	cores := map[string]models.SiteCore{
		"minor": {SiteID: "minor", MinorUpdate: "6.4.7", MajorUpdate: "6.8.3"},
		"major": {SiteID: "major", MajorUpdate: "6.8.3"},
	}
	got := coreObservations(reports, cores)
	if len(got) != 3 {
		t.Fatalf("got %d observations, want 3: %+v", len(got), got)
	}
	check := func(site, state, fixedIn string) {
		t.Helper()
		o := got[exposureKey{"core", "wordpress", site}]
		if o.FixState != state || o.FixedIn != fixedIn || o.Version != "6.4.1" {
			t.Errorf("%s: got %+v, want %s %q", site, o, state, fixedIn)
		}
	}
	check("minor", models.VulnFixAvailable, "6.4.7")
	check("major", models.VulnFixAvailable, "6.8.3")
	check("none", models.VulnNoFix, "")
}

func TestPlanExposureSync(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-48 * time.Hour)
	resolvedAt := now.Add(-2 * time.Hour)

	open := []models.VulnExposure{
		// Still affected, still fixable: the clock keeps running.
		{ID: 1, Kind: "plugin", Slug: "a", SiteID: "s1", FixState: models.VulnFixAvailable, FixAvailableSince: &earlier, FirstSeenAt: earlier},
		// Was no-fix, now fixable: the clock starts.
		{ID: 2, Kind: "plugin", Slug: "b", SiteID: "s1", FixState: models.VulnNoFix, FirstSeenAt: earlier},
		// Gone, data readable: resolved.
		{ID: 3, Kind: "plugin", Slug: "gone", SiteID: "s1", FixState: models.VulnNoFix},
		// Gone, data unreadable: left open.
		{ID: 4, Kind: "plugin", Slug: "unreadable", SiteID: "s1", FixState: models.VulnNoFix},
		// Gone, but core wasn't evaluated: left open.
		{ID: 5, Kind: "core", Slug: "wordpress", SiteID: "s1", FixState: models.VulnNoFix},
	}
	recent := []models.VulnExposure{
		{ID: 6, Kind: "plugin", Slug: "flap", SiteID: "s1", FixState: models.VulnFixAvailable,
			FixAvailableSince: &earlier, FirstSeenAt: earlier, ResolvedAt: &resolvedAt},
	}
	observed := map[exposureKey]observedExposure{
		{"plugin", "a", "s1"}:    {Version: "1.1", FixState: models.VulnFixAvailable, FixedIn: "2.0"},
		{"plugin", "b", "s1"}:    {Version: "1.0", FixState: models.VulnFixAvailable, FixedIn: "1.2"},
		{"plugin", "flap", "s1"}: {Version: "1.0", FixState: models.VulnFixAvailable, FixedIn: "1.2"},
		{"plugin", "new", "s2"}:  {Version: "1.0", FixState: models.VulnNoFix},
	}
	readable := func(e models.VulnExposure) bool { return e.Slug != "unreadable" }

	plan := planExposureSync(open, recent, observed, map[string]bool{"plugin": true}, readable, now)

	if !reflect.DeepEqual(plan.Resolves, []int64{3}) {
		t.Errorf("resolves = %v, want [3]", plan.Resolves)
	}
	if len(plan.Inserts) != 1 || plan.Inserts[0].Slug != "new" || !plan.Inserts[0].FirstSeenAt.Equal(now) ||
		plan.Inserts[0].FixAvailableSince != nil {
		t.Errorf("inserts = %+v, want the new no-fix exposure", plan.Inserts)
	}
	updates := map[int64]models.VulnExposure{}
	for _, e := range plan.Updates {
		updates[e.ID] = e
	}
	if len(updates) != 3 {
		t.Fatalf("updates = %+v, want 1, 2 and 6", plan.Updates)
	}
	if e := updates[1]; !e.FixAvailableSince.Equal(earlier) || e.Version != "1.1" || !e.LastSeenAt.Equal(now) {
		t.Errorf("exposure 1 = %+v, want the clock kept and version updated", e)
	}
	if e := updates[2]; e.FixAvailableSince == nil || !e.FixAvailableSince.Equal(now) {
		t.Errorf("exposure 2 = %+v, want the clock started now", e)
	}
	if e := updates[6]; e.ResolvedAt != nil || !e.FirstSeenAt.Equal(earlier) || !e.FixAvailableSince.Equal(earlier) {
		t.Errorf("exposure 6 = %+v, want it reopened with its clock kept", e)
	}
}

func TestApplyObservationClearsClockWhenNoFix(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	e := models.VulnExposure{FixState: models.VulnFixAvailable, FixAvailableSince: &since}
	applyObservation(&e, observedExposure{FixState: models.VulnNoFix}, time.Now())
	if e.FixAvailableSince != nil {
		t.Errorf("FixAvailableSince = %v, want nil", e.FixAvailableSince)
	}
}

func TestExposureDue(t *testing.T) {
	old := config.Cfg
	t.Cleanup(func() { config.Cfg = old })
	config.Cfg.VulnDueCriticalDays = 2
	config.Cfg.VulnDueHighDays = 7
	config.Cfg.VulnDueMediumDays = 0 // falls back to 14
	config.Cfg.VulnDueLowDays = 30

	for _, tt := range []struct {
		cvss *float64
		want int
	}{{fp(9.8), 2}, {fp(9), 2}, {fp(7.5), 7}, {fp(4), 14}, {nil, 14}, {fp(3.9), 30}} {
		if got := ExposureDueDays(tt.cvss); got != tt.want {
			t.Errorf("ExposureDueDays(%v) = %d, want %d", tt.cvss, got, tt.want)
		}
	}

	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	e := models.VulnExposure{FixState: models.VulnFixAvailable, FixAvailableSince: &since, MaxCvss: fp(7.5)}
	due, overdue := ExposureDue(e, since.AddDate(0, 0, 6))
	if due == nil || !due.Equal(since.AddDate(0, 0, 7)) || overdue {
		t.Errorf("day 6: due %v overdue %v, want due on day 7, not overdue", due, overdue)
	}
	if _, overdue := ExposureDue(e, since.AddDate(0, 0, 8)); !overdue {
		t.Error("day 8: want overdue")
	}
	e.FixState = models.VulnNoFix
	if due, overdue := ExposureDue(e, since.AddDate(0, 0, 30)); due != nil || overdue {
		t.Error("no-fix exposure must never be due")
	}
}

func TestGroupExposures(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	longAgo := now.AddDate(0, 0, -60)
	exposures := []models.VulnExposure{
		{Kind: "plugin", Slug: "low", SiteID: "s1", VulnUUIDs: []string{"l1"}, MaxCvss: fp(3), FixState: models.VulnNoFix},
		{Kind: "plugin", Slug: "high", SiteID: "s2", VulnUUIDs: []string{"h1"}, MaxCvss: fp(8), FixState: models.VulnNoFix},
		{Kind: "plugin", Slug: "late", SiteID: "s1", VulnUUIDs: []string{"x1"}, MaxCvss: fp(2), FixState: models.VulnFixAvailable, FixAvailableSince: &now},
		{Kind: "plugin", Slug: "late", SiteID: "s2", VulnUUIDs: []string{"x1", "x2"}, MaxCvss: fp(5), FixState: models.VulnFixAvailable, FixAvailableSince: &longAgo},
		{Kind: "core", Slug: "wordpress", SiteID: "s1", Version: "6.4.1", VulnUUIDs: []string{"c1"}, FixState: models.VulnNoFix},
		{Kind: "core", Slug: "wordpress", SiteID: "s2", Version: "6.5.0", VulnUUIDs: []string{"c1"}, FixState: models.VulnNoFix},
	}
	details := func(kind, slug, coreVersion string) groupDetails {
		d := groupDetails{Name: slug, Vulns: map[string]models.Vulnerability{
			"x2": rangeVuln("x2", "5.0", "lt", "2.0", false),
		}}
		if kind == "core" {
			d.Name = "WordPress " + coreVersion
		}
		if slug == "late" {
			d.Closed, d.ClosedReason = true, "security-issue"
		}
		return d
	}

	groups := groupExposures(exposures, map[string]string{"s1": "Bravo", "s2": "Alpha"}, details, now)

	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}
	want := []string{"late", "high", "low", "WordPress 6.4.1", "WordPress 6.5.0"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("group order = %v, want %v", names, want)
	}

	late := groups[0]
	if late.OverdueCount != 1 || late.FixAvailableCount != 2 || *late.MaxCvss != 5 || !late.Closed || late.ClosedReason != "security-issue" {
		t.Errorf("late group = %+v", late)
	}
	if late.Sites[0].SiteName != "Alpha" || !late.Sites[0].Overdue || late.Sites[1].Overdue {
		t.Errorf("late sites = %+v, want the overdue Alpha first", late.Sites)
	}
	if len(late.Vulnerabilities) != 2 || late.Vulnerabilities[0].UUID != "x2" || late.Vulnerabilities[0].FixedIn != "2.0" ||
		late.Vulnerabilities[1].Name != "x1" {
		t.Errorf("late vulnerabilities = %+v, want x2 (known, scored) then x1 (unknown)", late.Vulnerabilities)
	}

	summary := SummarizeExposures(exposures, now)
	if summary != (models.VulnExposureSummary{Groups: 5, Exposures: 6, Overdue: 1, NoFix: 4}) {
		t.Errorf("summary = %+v", summary)
	}
}
