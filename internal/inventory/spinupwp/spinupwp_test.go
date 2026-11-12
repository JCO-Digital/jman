package spinupwp

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func TestItemsMapsServersAndSiteDomains(t *testing.T) {
	servers := []models.Server{{
		ID: 678, Name: "lon1.examplehost.com", ProviderName: "Hetzner",
		IPAddress: "203.0.113.10", SSHPort: 22, Region: "fsn1", Size: "cx22",
		UbuntuVersion: "24.04", ConnectionStatus: "connected", Status: "active",
	}}
	sites := []models.Site{{
		ID: 12, ServerID: 678, Domain: "www.example.com", SiteUser: "example",
		PHPVersion: "8.3", IsWordpress: true, Status: "active",
		AdditionalDomains: []models.AdditionalDomain{
			{Domain: "example.com", Redirect: models.Enabled{Enabled: true}},
			{Domain: "blog.example.co.uk"},
		},
		HTTPS: models.HTTPS{Enabled: true, CertificateExpires: "2027-01-01"},
	}}

	items := Items(servers, sites)
	if len(items) != 4 {
		t.Fatalf("got %d items, want 4 (1 server + 3 domain entities): %+v", len(items), items)
	}

	srv := items[0]
	if srv.Type != models.InventoryItemTypeServer || srv.ProviderType != "server" || srv.ProviderID != "678" {
		t.Errorf("server item: %+v", srv)
	}
	if srv.Name != "lon1.examplehost.com" || srv.Data["ip_address"] != "203.0.113.10" || srv.Data["region"] != "fsn1" {
		t.Errorf("server data: %+v", srv.Data)
	}

	// All three site domains reduce to their apex domain; www.example.com
	// and example.com share the name example.com but keep distinct
	// provider IDs, so a sync groups them into one item with two links.
	wantDomains := []struct{ providerID, apex string }{
		{"12:www.example.com", "example.com"},
		{"12:example.com", "example.com"},
		{"12:blog.example.co.uk", "example.co.uk"},
	}
	for i, want := range wantDomains {
		d := items[i+1]
		if d.Type != models.InventoryItemTypeDomain || d.ProviderType != "site_domain" {
			t.Errorf("domain item %d: %+v", i, d)
			continue
		}
		if d.ProviderID != want.providerID || d.Name != want.apex {
			t.Errorf("domain item %d = %q / %q, want %q / %q", i, d.ProviderID, d.Name, want.providerID, want.apex)
		}
		if d.SiteID != utils.SpinupWPSiteUUID(12) {
			t.Errorf("domain item %d site link = %q", i, d.SiteID)
		}
	}
	if items[1].Data["additional"] != false || items[2].Data["additional"] != true || items[2].Data["redirect"] != true {
		t.Errorf("additional/redirect flags: %+v %+v", items[1].Data, items[2].Data)
	}
	if items[1].Data["certificate_expires"] != "2027-01-01" {
		t.Errorf("https data: %+v", items[1].Data)
	}
}

func TestItemsSkipsNonRegistrableDomains(t *testing.T) {
	sites := []models.Site{{
		ID: 1, Domain: "localhost",
		AdditionalDomains: []models.AdditionalDomain{{Domain: "203.0.113.10"}, {Domain: "shop.example.org"}},
	}}
	items := Items(nil, sites)
	if len(items) != 1 || items[0].Name != "example.org" {
		t.Fatalf("items: %+v", items)
	}
}
