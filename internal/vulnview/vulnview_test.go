package vulnview

import (
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func vulnWithCvss(uuid, score string, sites ...models.PluginSite) models.Vulnerability {
	return models.Vulnerability{
		Uuid:   uuid,
		Name:   "Vuln " + uuid,
		Impact: &models.Impact{Cvss: &models.Cvss{Score: score}},
		Sites:  sites,
	}
}

func names(id string) string {
	return map[string]string{"a": "alpha.example.com", "b": "beta.example.com"}[id]
}

func testReports() []models.VulnReport {
	return []models.VulnReport{
		{Slug: "high", PluginName: "High", Vulnerabilities: []models.Vulnerability{
			vulnWithCvss("h1", "9.0", models.PluginSite{SiteID: "a", Version: "1.0"}, models.PluginSite{SiteID: "b", Version: "1.0", Suppressed: true}),
		}},
		{Slug: "low", PluginName: "Low", Vulnerabilities: []models.Vulnerability{
			vulnWithCvss("l1", "3.0", models.PluginSite{SiteID: "b", Version: "2.0"}),
			vulnWithCvss("l2", "2.0", models.PluginSite{SiteID: "b", Version: "2.0"}),
		}},
		{Slug: "ignored", Suppressed: true, Vulnerabilities: []models.Vulnerability{
			vulnWithCvss("i1", "10.0", models.PluginSite{SiteID: "a", Version: "1.0"}),
		}},
	}
}

func TestFilterReports(t *testing.T) {
	got := FilterReports(testReports(), "", 0, names)
	if len(got) != 2 {
		t.Fatalf("expected the ignored plugin dropped, got %d reports", len(got))
	}
	if sites := got[0].Vulnerabilities[0].Sites; len(sites) != 1 || sites[0].SiteID != "a" {
		t.Errorf("expected the suppressed site dropped, got %+v", sites)
	}

	if got := FilterReports(testReports(), "", 7, names); len(got) != 1 || got[0].Slug != "high" {
		t.Errorf("CVSS threshold: got %+v", got)
	}

	// A site search ignores the threshold and trims the site lists.
	got = FilterReports(testReports(), "BETA", 7, names)
	if len(got) != 1 || got[0].Slug != "low" {
		t.Fatalf("site search: got %+v", got)
	}
}

func TestSitesOverThreshold(t *testing.T) {
	// alpha has one 9.0 vulnerability, beta two low ones.
	sites := SitesOverThreshold(testReports(), 7, 10)
	if len(sites) != 1 || sites[0].SiteID != "a" || sites[0].MaxCvss != 9 {
		t.Fatalf("CVSS threshold: got %+v", sites)
	}
	sites = SitesOverThreshold(testReports(), 10, 2)
	if len(sites) != 1 || sites[0].SiteID != "b" || sites[0].TotalVulns != 2 {
		t.Fatalf("count threshold: got %+v", sites)
	}
	if p := sites[0].Plugins["low"]; p == nil || *p.Cvss != 3 || p.Version != "2.0" {
		t.Errorf("plugin summary: %+v", p)
	}
}

func TestFormatReport(t *testing.T) {
	r := FilterReports(testReports(), "", 0, names)[0]
	out := FormatReport(r, true, names)
	if !strings.Contains(out, "alpha.example.com") || strings.Contains(out, "beta.example.com") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestFilterCoreReports(t *testing.T) {
	reports := []models.CoreVulnReport{
		{Version: "6.1", Vulnerabilities: []models.Vulnerability{vulnWithCvss("c1", "8.0", models.PluginSite{SiteID: "a"})}},
		{Version: "6.2", Vulnerabilities: []models.Vulnerability{vulnWithCvss("c2", "5.0", models.PluginSite{SiteID: "b", Suppressed: true})}},
	}
	got := FilterCoreReports(reports, 0)
	if len(got) != 1 || got[0].Version != "6.1" {
		t.Errorf("got %+v", got)
	}
	if got := FilterCoreReports(reports, 9); len(got) != 0 {
		t.Errorf("threshold: got %+v", got)
	}
}
