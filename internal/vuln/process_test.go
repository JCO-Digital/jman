package vuln

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestProcessVulnerabilitiesForOneSite(t *testing.T) {
	setupBackportTest(t)
	seedWooCommerce(t)
	for _, p := range []models.WPPlugin{
		{SiteID: "a", Name: "woocommerce", Status: "active", Version: "10.0.0"},
		{SiteID: "b", Name: "woocommerce", Status: "active", Version: "10.1.0"},
		{SiteID: "b", Name: "no-data", Status: "active", Version: "1.0.0"},
	} {
		if err := db.SaveSitePlugin(p); err != nil {
			t.Fatalf("save plugin: %v", err)
		}
	}

	all, err := ProcessVulnerabilities(newMatcher(t))
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(all) != 1 || len(all[0].Vulnerabilities) != 1 || len(all[0].Vulnerabilities[0].Sites) != 2 {
		t.Fatalf("all sites = %+v, want woocommerce on a and b", all)
	}

	one, err := processVulnerabilities(newMatcher(t), "a")
	if err != nil {
		t.Fatalf("process site: %v", err)
	}
	if len(one) != 1 || len(one[0].Vulnerabilities) != 1 {
		t.Fatalf("site a = %+v, want one report", one)
	}
	if sites := one[0].Vulnerabilities[0].Sites; len(sites) != 1 || sites[0].SiteID != "a" {
		t.Errorf("site a sites = %+v, want only a", sites)
	}

	// The cached feed data is shared: reports must not have added sites to it.
	again, _ := ProcessVulnerabilities(nil)
	if n := len(again[0].Vulnerabilities[0].Sites); n != 2 {
		t.Errorf("second pass sites = %d, want 2", n)
	}
}

func TestProcessCoreVulnerabilitiesForOneSite(t *testing.T) {
	setupBackportTest(t)
	seedCore(t, "6.4.1", "core-1")
	for _, id := range []string{"a", "b"} {
		if err := db.SaveSiteCore(id, "6.4.1", "", ""); err != nil {
			t.Fatalf("save core: %v", err)
		}
	}

	one, err := processCoreVulnerabilities(nil, "b")
	if err != nil {
		t.Fatalf("process site: %v", err)
	}
	if len(one) != 1 || len(one[0].Vulnerabilities) != 1 {
		t.Fatalf("site b = %+v, want one report", one)
	}
	if sites := one[0].Vulnerabilities[0].Sites; len(sites) != 1 || sites[0].SiteID != "b" {
		t.Errorf("site b sites = %+v, want only b", sites)
	}
	if none, _ := processCoreVulnerabilities(nil, "c"); len(none) != 0 {
		t.Errorf("site c = %+v, want none", none)
	}
}
