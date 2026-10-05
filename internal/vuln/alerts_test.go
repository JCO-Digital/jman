package vuln

import (
	"strings"
	"testing"

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

func TestCollectNewVulnAlerts(t *testing.T) {
	siteMeta := map[string]models.CliSite{
		"s1": {ID: "s1", Name: "Bravo", ServerID: "srv1"},
		"s2": {ID: "s2", Name: "Alpha", ServerID: "srv1"},
		"s3": {ID: "s3", Name: "Ignored", ServerID: "srv2"},
	}
	ignored := func(siteID, serverID string) bool { return serverID == "srv2" }

	reports := []models.VulnReport{
		{
			Slug:       "zeta",
			PluginName: "Zeta",
			Vulnerabilities: []models.Vulnerability{
				// Meets threshold; s3 is ignored, s4 suppressed, s1/s2 kept and sorted by name.
				testVuln("v1", "8.1",
					models.PluginSite{SiteID: "s1", Version: "1.0"},
					models.PluginSite{SiteID: "s2", Version: "1.1"},
					models.PluginSite{SiteID: "s3", Version: "1.0"},
					models.PluginSite{SiteID: "s4", Version: "1.0", Suppressed: true},
				),
				// Below threshold.
				testVuln("v2", "5.0", models.PluginSite{SiteID: "s1", Version: "1.0"}),
				// Only on an ignored site.
				testVuln("v3", "9.0", models.PluginSite{SiteID: "s3", Version: "1.0"}),
			},
		},
		{
			Slug:       "alpha",
			PluginName: "Alpha Plugin",
			Vulnerabilities: []models.Vulnerability{
				testVuln("v4", "7.0", models.PluginSite{SiteID: "s9", Version: "2.0"}),
				func() models.Vulnerability {
					v := testVuln("v5", "9.8", models.PluginSite{SiteID: "s1", Version: "2.0"})
					v.Suppressed = true
					return v
				}(),
			},
		},
		{
			Slug:            "suppressed",
			Suppressed:      true,
			Vulnerabilities: []models.Vulnerability{testVuln("v6", "9.8", models.PluginSite{SiteID: "s1"})},
		},
	}

	got := collectNewVulnAlerts(reports, siteMeta, ignored, 7.0)
	if len(got) != 2 {
		t.Fatalf("got %d alerts, want 2: %+v", len(got), got)
	}

	// Ordered by plugin slug: alpha before zeta.
	if got[0].Vuln.Uuid != "v4" || got[0].Sites[0].Name != "Site ID: s9" {
		t.Errorf("unexpected first alert %+v", got[0])
	}
	if got[1].Vuln.Uuid != "v1" || got[1].Cvss != 8.1 {
		t.Errorf("unexpected second alert %+v", got[1])
	}
	if len(got[1].Sites) != 2 || got[1].Sites[0].Name != "Alpha" || got[1].Sites[1].Name != "Bravo" {
		t.Errorf("unexpected sites %+v", got[1].Sites)
	}

	// A nil ignore func keeps every non-suppressed site.
	if got := collectNewVulnAlerts(reports, siteMeta, nil, 7.0); len(got) != 3 {
		t.Errorf("nil ignore func: got %d alerts, want 3", len(got))
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
		PluginSlug: "zeta",
		PluginName: "Zeta",
		Cvss:       8.1,
		Sites:      []alertSite{{Name: "Alpha", Version: "1.1"}},
	}))

	for _, want := range []string{
		"New vulnerability: Zeta <= 1.2 - Stored XSS",
		"Plugin: Zeta (zeta)",
		"CVSS: 8.1",
		"Link: https://example.com/cve",
		"Installed on 1 site:",
		"- Alpha (1.1)",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}
