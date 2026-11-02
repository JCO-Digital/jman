package vuln

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func testVuln(uuid, score string, sites ...models.PluginSite) models.Vulnerability {
	return models.Vulnerability{
		Uuid:   uuid,
		Name:   "Vuln " + uuid,
		Impact: &models.Impact{Cvss: &models.Cvss{Score: score}},
		Sites:  sites,
	}
}

func TestPlanVulnAlerts(t *testing.T) {
	names := map[string]string{"s1": "Bravo", "s2": "Alpha", "s3": "Charlie"}
	details := func(kind, slug, coreVersion string) groupDetails {
		d := groupDetails{Name: "Zeta", Vulns: map[string]models.Vulnerability{}}
		for _, v := range []models.Vulnerability{
			testVuln("new-high", "8.1"), testVuln("new-low", "5.0"), testVuln("lifting", "4.0"),
			testVuln("known-high", "9.0"), testVuln("known-low", "3.0"), testVuln("done", "9.9"),
		} {
			d.Vulns[v.Uuid] = v
		}
		if kind == models.VulnExposureCore {
			d.Name = "WordPress " + coreVersion
		}
		return d
	}
	exposures := []models.VulnExposure{
		{Kind: "plugin", Slug: "zeta", SiteID: "s1", Version: "1.0", VulnUUIDs: []string{"done", "known-high", "new-high", "new-low"},
			FixState: models.VulnFixAvailable, FixedIn: "2.0"},
		{Kind: "plugin", Slug: "zeta", SiteID: "s2", Version: "1.1", VulnUUIDs: []string{"known-high", "new-high"},
			FixState: models.VulnNoFix},
		{Kind: "plugin", Slug: "zeta", SiteID: "s3", Version: "1.0", VulnUUIDs: []string{"known-low", "lifting"}},
		{Kind: "core", Slug: "wordpress", SiteID: "s1", Version: "6.4.1", VulnUUIDs: []string{"unknown-core", "new-low"},
			FixState: models.VulnFixAvailable, FixedIn: "6.4.7"},
		{Kind: "core", Slug: "wordpress", SiteID: "s2", Version: "6.4.1", VulnUUIDs: []string{"unknown-core"}},
	}
	alertedUUIDs := map[string]bool{"known-high": true, "known-low": true, "done": true}
	alertedSites := map[db.VulnAlertSite]bool{{Kind: "plugin", Slug: "zeta", SiteID: "s1", UUID: "done"}: true}
	lifted := map[exposureKey]bool{{"plugin", "zeta", "s3"}: true}

	plan := planVulnAlerts(exposures, alertedUUIDs, alertedSites, lifted, names, details, 7.0)

	var newIDs []string
	for _, a := range plan.New {
		newIDs = append(newIDs, a.Vuln.Uuid)
	}
	// new-low is below the threshold; lifting is too, but lifted a mute.
	if !reflect.DeepEqual(newIDs, []string{"new-high", "lifting"}) {
		t.Fatalf("new alerts = %v, want [new-high lifting]", newIDs)
	}
	high := plan.New[0]
	if len(high.Sites) != 2 || high.Sites[0].Name != "Alpha" || high.Sites[1].Name != "Bravo" || high.Cvss != 8.1 ||
		high.PluginName != "Zeta" || len(high.Records) != 2 {
		t.Errorf("new-high alert = %+v, want sites Alpha and Bravo", high)
	}
	if lift := plan.New[1]; len(lift.Sites) != 1 || !lift.Sites[0].MuteLifted || lift.Sites[0].Name != "Charlie" {
		t.Errorf("lifting alert = %+v, want Charlie marked mute lifted", lift)
	}

	if len(plan.NewSites) != 1 {
		t.Fatalf("newly-affected alerts = %+v, want one for zeta", plan.NewSites)
	}
	ns := plan.NewSites[0]
	if ns.PluginSlug != "zeta" || len(ns.Sites) != 2 || ns.Sites[0].Name != "Alpha" || ns.Sites[0].FixState != models.VulnNoFix ||
		ns.Sites[1].Name != "Bravo" || ns.Sites[1].FixedIn != "2.0" || ns.Sites[1].Cvss != 9 || ns.Sites[1].Vulns != 1 {
		t.Errorf("newly-affected alert = %+v", ns)
	}

	silent := map[string]bool{}
	for _, r := range plan.Silent {
		silent[r.SiteID+"/"+r.UUID] = true
	}
	// new-low (below threshold) and known-low on Charlie (below threshold).
	// Core is alerted whatever the score.
	want := map[string]bool{"s1/new-low": true, "s3/known-low": true}
	if !reflect.DeepEqual(silent, want) {
		t.Errorf("silent = %v, want %v", silent, want)
	}

	// One core alert for 6.4.1: both vulnerabilities (one with no data, one
	// below the threshold) once, both sites once, a record per site and
	// vulnerability.
	if len(plan.NewCore) != 1 {
		t.Fatalf("core alerts = %+v, want one", plan.NewCore)
	}
	c := plan.NewCore[0]
	var coreIDs, coreSites []string
	for _, v := range c.Vulns {
		coreIDs = append(coreIDs, v.Uuid)
	}
	for _, site := range c.Sites {
		coreSites = append(coreSites, site.Name)
	}
	if c.Version != "6.4.1" || !reflect.DeepEqual(coreIDs, []string{"unknown-core", "new-low"}) ||
		!reflect.DeepEqual(coreSites, []string{"Alpha", "Bravo"}) || len(c.Records) != 3 {
		t.Errorf("core alert = %+v", c)
	}
	if c.Sites[1].FixedIn != "6.4.7" {
		t.Errorf("Bravo's fix = %+v, want 6.4.7", c.Sites[1])
	}
}

func TestFormatNewCoreAlert(t *testing.T) {
	core := func(id, score, name, link string) models.Vulnerability {
		v := testVuln(id, score)
		if score == "" {
			v.Impact = nil
		}
		v.Name = name
		v.Source = []models.Source{{Id: id, Name: name, Link: link}}
		return v
	}
	var vulns []models.Vulnerability
	vulns = append(vulns,
		core("a", "", "WordPress Core 6.6.0 - 7.1.3 - Authenticated (Contributor+) Stored Cross-Site Scripting via Blocks", "https://example.com/a"),
		core("b", "8.1", "CVE-2026-1", "https://example.com/b"),
		core("c", "", "WordPress Core 4.9.0 - 7.1.3 - Unauthenticated Information Exposure via Customizer", "https://example.com/c"),
	)
	for _, id := range []string{"d", "e", "f", "g"} {
		vulns = append(vulns, core(id, "", "WordPress Core 6.0 - 7.1.3 - Authenticated (Editor+) Thing "+id, ""))
	}
	msg := utils.StripANSI(formatNewCoreAlert(newCoreAlert{
		Version: "6.9.10",
		Vulns:   vulns,
		Sites: []newCoreSite{
			{Name: "Alpha", FixState: models.VulnFixAvailable, FixedIn: "6.9.11"},
			{Name: "Bravo", FixState: models.VulnNoFix, MuteLifted: true},
		},
	}))
	for _, want := range []string{
		"New core vulnerabilities: 7 new vulnerabilities (max CVSS 8.1, 1 unauthenticated)",
		"Core: WordPress 6.9.10",
		// Scored first, then unauthenticated, then by role.
		"- CVSS 8.1 CVE-2026-1 https://example.com/b\n    - Unauthenticated: Information Exposure via Customizer https://example.com/c\n    - Contributor+: Stored Cross-Site Scripting via Blocks https://example.com/a",
		"- …and 2 more",
		"Installed on 2 sites:",
		"- Alpha: update to 6.9.11\n",
		"- Bravo: no update available mute lifted",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestFormatNewVulnAlert(t *testing.T) {
	v := testVuln("v1", "8.1")
	v.Source = []models.Source{
		{Name: "CVE-2026-0001", Link: "https://example.com/cve"},
		{Name: "Zeta <= 1.2 - Stored XSS"},
	}
	msg := utils.StripANSI(formatNewVulnAlert(newVulnAlert{
		Vuln:       v,
		Kind:       models.VulnExposurePlugin,
		PluginSlug: "zeta",
		PluginName: "Zeta",
		Cvss:       8.1,
		Sites:      []alertSite{{Name: "Alpha", Version: "1.1"}, {Name: "Bravo", Version: "1.0", MuteLifted: true}},
	}))

	for _, want := range []string{
		"New vulnerability: Zeta <= 1.2 - Stored XSS",
		"Plugin: Zeta (zeta)",
		"CVSS: 8.1",
		"Link: https://example.com/cve",
		"Installed on 2 sites:",
		"- Alpha (1.1)\n",
		"- Bravo (1.0) mute lifted",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}

	core := utils.StripANSI(formatNewVulnAlert(newVulnAlert{
		Vuln: testVuln("c1", "6.1"), Kind: models.VulnExposureCore, PluginSlug: "wordpress",
		PluginName: "WordPress 6.4.1", Cvss: 6.1, Sites: []alertSite{{Name: "Alpha", Version: "6.4.1"}},
	}))
	if !strings.Contains(core, "Core: WordPress 6.4.1") || !strings.Contains(core, "Installed on 1 site:") {
		t.Errorf("core message:\n%s", core)
	}
}

func TestFormatNewSitesAlert(t *testing.T) {
	msg := utils.StripANSI(formatNewSitesAlert(newSitesAlert{
		Kind: models.VulnExposurePlugin, PluginSlug: "zeta", PluginName: "Zeta",
		Sites: []newSite{
			{Name: "Alpha", Version: "1.1", Vulns: 2, Cvss: 9.0, FixState: models.VulnFixAvailable, FixedIn: "2.0"},
			{Name: "Bravo", Version: "1.0", Vulns: 1, Cvss: 7.5, FixState: models.VulnNoFix},
		},
	}))
	for _, want := range []string{
		"Newly affected: known vulnerabilities on 2 more sites",
		"Plugin: Zeta (zeta)",
		"- Alpha (1.1): 2 vulnerabilities, CVSS 9.0, update to 2.0",
		"- Bravo (1.0): 1 vulnerability, CVSS 7.5, no fix",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestAlertNewVulnerabilitiesFlow(t *testing.T) {
	setupBackportTest(t)
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}
	// No Slack token: every send fails, so nothing sent is recorded.
	old := config.Cfg
	t.Cleanup(func() { config.Cfg = old })
	config.Cfg.TokenSlack = ""
	config.Cfg.CVSSThreshold = 7.0

	resp := models.VulnResponse{Data: &models.VulnData{Name: strp("Zeta"), Vulnerability: []models.Vulnerability{
		testVuln("old", "9.0"), testVuln("high", "8.0"), testVuln("low", "3.0"),
	}}}
	if err := cache.WriteJSONCache("vulnerabilities/zeta", resp); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	insert := func(site string, uuids ...string) models.VulnExposure {
		t.Helper()
		e := models.VulnExposure{Kind: "plugin", Slug: "zeta", SiteID: site, Version: "1.0", VulnUUIDs: uuids,
			FixState: models.VulnNoFix, FirstSeenAt: now, LastSeenAt: now}
		if err := db.InsertVulnExposure(&e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	recorded := func() map[string]bool {
		t.Helper()
		all, err := db.ListVulnAlertSites()
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for r := range all {
			got[r.SiteID+"/"+r.UUID] = true
		}
		return got
	}

	gone := insert("gone", "old")
	insert("s1", "old")
	// First run: seeds what exists, sends nothing.
	if err := AlertNewVulnerabilities(); err != nil {
		t.Fatal(err)
	}
	if got := recorded(); !reflect.DeepEqual(got, map[string]bool{"gone/old": true, "s1/old": true}) {
		t.Fatalf("seeded = %v", got)
	}

	// "gone" stops being affected long ago; s2 gets the known vulnerability
	// and two new ones.
	if err := db.ResolveVulnExposures([]int64{gone.ID}, now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	insert("s2", "high", "low", "old")
	if err := AlertNewVulnerabilities(); err != nil {
		t.Fatal(err)
	}
	// The sends failed, so only the silent (low) record is kept: the
	// others are retried. "gone" is forgotten.
	if got := recorded(); !reflect.DeepEqual(got, map[string]bool{"s1/old": true, "s2/low": true}) {
		t.Errorf("after second run = %v", got)
	}
	if alerted, _ := db.GetAlertedVulnUUIDs(); alerted["high"] {
		t.Error("high was recorded as alerted although the send failed")
	}
}
