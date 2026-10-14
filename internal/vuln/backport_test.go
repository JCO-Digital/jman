package vuln

import (
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupBackportTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := config.RunData
	config.RunData.DataDir = dir
	config.RunData.CacheDir = dir
	if err := db.InitInventory(); err != nil {
		t.Fatalf("init inventory: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		config.RunData = old
	})
}

func strp(s string) *string { return &s }

// seedWooCommerce caches one vulnerability fixed in 11.2.0 that affects 9.8.0
// and up.
func seedWooCommerce(t *testing.T) {
	t.Helper()
	resp := models.VulnResponse{Data: &models.VulnData{
		Name: strp("WooCommerce"),
		Vulnerability: []models.Vulnerability{{
			Uuid: "woo-1",
			Name: "WooCommerce XSS",
			Operator: models.Operator{
				MinVersion: strp("9.8.0"), MinOperator: strp("ge"),
				MaxVersion: strp("11.2.0"), MaxOperator: strp("lt"),
			},
		}},
	}}
	if err := cache.WriteJSONCache("vulnerabilities/woocommerce", resp); err != nil {
		t.Fatalf("seed plugin cache: %v", err)
	}
}

func seedCore(t *testing.T, version string, uuids ...string) {
	t.Helper()
	data := &models.CoreVulnData{Core: version}
	for _, u := range uuids {
		data.Vulnerability = append(data.Vulnerability, models.Vulnerability{Uuid: u, Name: "Core " + u})
	}
	if err := cache.WriteJSONCache("vulnerabilities/core/"+version, models.CoreVulnResponse{Data: data}); err != nil {
		t.Fatalf("seed core cache: %v", err)
	}
}

func addRule(t *testing.T, r models.VulnBackport) {
	t.Helper()
	if err := db.CreateVulnBackport(&r, "test"); err != nil {
		t.Fatalf("create rule: %v", err)
	}
}

func newMatcher(t *testing.T) *db.VulnIgnoreMatcher {
	t.Helper()
	m, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		t.Fatalf("matcher: %v", err)
	}
	return m
}

func TestPluginReportMovesBackportedSitesToPatched(t *testing.T) {
	setupBackportTest(t)
	seedWooCommerce(t)
	addRule(t, models.VulnBackport{Type: "plugin", Slug: "woocommerce", VulnUUID: "woo-1", Branch: "11.1", MinVersion: "11.1.2"})

	sites := []models.PluginSite{
		{SiteID: "a", Version: "11.1.2"}, // patched by the backport
		{SiteID: "b", Version: "11.1.1"}, // same branch, before the backport
		{SiteID: "c", Version: "11.2.0"}, // officially fixed
	}
	report := GetVulnerabilityReportsForPlugin("woocommerce", sites, newMatcher(t))
	if report == nil || len(report.Vulnerabilities) != 1 {
		t.Fatalf("expected one vulnerability, got %+v", report)
	}
	v := report.Vulnerabilities[0]
	if len(v.Sites) != 1 || v.Sites[0].SiteID != "b" {
		t.Errorf("Sites = %+v, want only b", v.Sites)
	}
	if len(v.PatchedSites) != 1 || v.PatchedSites[0].SiteID != "a" {
		t.Errorf("PatchedSites = %+v, want only a", v.PatchedSites)
	}

	// With every affected site patched, the vulnerability disappears.
	if r := GetVulnerabilityReportsForPlugin("woocommerce", sites[:1], newMatcher(t)); r != nil {
		t.Errorf("expected no report when all sites are patched, got %+v", r)
	}
	if IsPluginVulnerableOnSite("a", "woocommerce", "11.1.2", newMatcher(t)) {
		t.Error("IsPluginVulnerableOnSite should be false for a backported version")
	}
	if !IsPluginVulnerableOnSite("b", "woocommerce", "11.1.1", newMatcher(t)) {
		t.Error("IsPluginVulnerableOnSite should be true before the backport")
	}
}

func TestCoreReportSkipsBackportedVersion(t *testing.T) {
	setupBackportTest(t)
	seedCore(t, "6.4.5", "core-1", "core-2")
	seedCore(t, "6.4.4", "core-1", "core-2")
	addRule(t, models.VulnBackport{Type: "core", VulnUUID: "core-1", Branch: "6.4", MinVersion: "6.4.5"})

	sites := []models.PluginSite{{SiteID: "a", Version: "6.4.5"}}
	report := GetVulnerabilityReportForCoreVersion("6.4.5", sites, newMatcher(t))
	if report == nil || len(report.Vulnerabilities) != 1 || report.Vulnerabilities[0].Uuid != "core-2" {
		t.Fatalf("6.4.5: expected only core-2, got %+v", report)
	}

	report = GetVulnerabilityReportForCoreVersion("6.4.4", sites, newMatcher(t))
	if report == nil || len(report.Vulnerabilities) != 2 {
		t.Fatalf("6.4.4: expected both vulnerabilities, got %+v", report)
	}
}

func TestValidateVulnBackport(t *testing.T) {
	setupBackportTest(t)
	seedWooCommerce(t)
	seedCore(t, "6.4.5", "core-1")

	plugin := models.VulnBackport{Type: "plugin", Slug: "woocommerce", VulnUUID: "woo-1", Branch: "11.1", MinVersion: "11.1.2", Reason: "Backported"}
	core := models.VulnBackport{Type: "core", VulnUUID: "core-1", Branch: "6.4", MinVersion: "6.4.5", Reason: "Backported"}

	tests := []struct {
		name     string
		rule     models.VulnBackport
		wantWarn string // substring; empty means no warning
	}{
		{"valid plugin rule", plugin, ""},
		{"valid core rule", core, ""},
		{"unknown plugin vulnerability", withUUID(plugin, "nope"), "not known"},
		{"branch below the affected range", withVersion(plugin, "9.7", "9.7.3"), "outside"},
		{"minimum at the official fix", withVersion(plugin, "11.2", "11.2.0"), "outside"},
		{"core version not affected", withUUID(core, "nope"), "not reported"},
	}
	for _, tt := range tests {
		warnings, err := ValidateVulnBackport(tt.rule)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tt.name, err)
			continue
		}
		if tt.wantWarn == "" && len(warnings) != 0 {
			t.Errorf("%s: unexpected warnings %v", tt.name, warnings)
		}
		if tt.wantWarn != "" && (len(warnings) != 1 || !strings.Contains(warnings[0], tt.wantWarn)) {
			t.Errorf("%s: warnings %v, want one containing %q", tt.name, warnings, tt.wantWarn)
		}
	}

	if _, err := ValidateVulnBackport(withVersion(plugin, "11.1", "11.10.2")); err == nil {
		t.Error("malformed rule should return an error")
	}
}

func withUUID(r models.VulnBackport, uuid string) models.VulnBackport {
	r.VulnUUID = uuid
	return r
}

func withVersion(r models.VulnBackport, branch, minVersion string) models.VulnBackport {
	r.Branch, r.MinVersion = branch, minVersion
	return r
}

func TestListVulnBackportsCountsMatches(t *testing.T) {
	setupBackportTest(t)
	for _, p := range []models.WPPlugin{
		{SiteID: "a", Name: "woocommerce", Status: "active", Version: "11.1.2"},
		{SiteID: "b", Name: "woocommerce", Status: "active", Version: "11.1.1"},
		{SiteID: "c", Name: "woocommerce", Status: "inactive", Version: "11.1.3"},
		{SiteID: "d", Name: "other", Status: "active", Version: "11.1.3"},
	} {
		if err := db.SaveSitePlugin(p); err != nil {
			t.Fatalf("save plugin: %v", err)
		}
	}
	if err := db.SaveSiteCore("a", "6.4.5", "", ""); err != nil {
		t.Fatalf("save core: %v", err)
	}
	addRule(t, models.VulnBackport{Type: "plugin", Slug: "woocommerce", VulnUUID: "woo-1", Branch: "11.1", MinVersion: "11.1.2"})
	addRule(t, models.VulnBackport{Type: "core", VulnUUID: "core-1", Branch: "6.4", MinVersion: "6.4.5"})

	listings, err := ListVulnBackports()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]int{}
	for _, l := range listings {
		got[l.Type] = l.MatchedSites
	}
	if got["plugin"] != 1 || got["core"] != 1 {
		t.Errorf("matched sites = %v, want plugin:1 core:1", got)
	}
}
