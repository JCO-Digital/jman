// Package spinupwp is the SpinupWP provider of the asset inventory: it maps
// SpinupWP servers and site domains to inventory items. It reads through the
// shared JSON cache (internal/cache), so the refresh scheduler's fast tick
// doesn't make the SpinupWP API fetch twice. See docs/inventory.md.
package spinupwp

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/inventory"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Name is the provider's registry name.
const Name = "spinupwp"

// Provider implements inventory.Provider for SpinupWP.
type Provider struct{}

func init() {
	inventory.Register(Provider{})
}

// Name returns the provider name.
func (Provider) Name() string { return Name }

// Fetch returns the inventory items for SpinupWP's current servers and
// sites, read from the shared cache (refreshed from the API when stale).
func (Provider) Fetch(ctx context.Context) ([]inventory.ExternalItem, error) {
	if config.Cfg.TokenSpinup == "" {
		return nil, inventory.ErrNotConfigured
	}
	servers, err := cache.GetCachedServers()
	if err != nil {
		return nil, err
	}
	sites, err := cache.GetCachedSites()
	if err != nil {
		return nil, err
	}
	return Items(servers, sites), nil
}

// Items maps SpinupWP servers and sites to inventory external items: every
// server becomes a 'server' item, and every site's primary domain plus its
// additional domains become 'domain' items named by their apex
// (registrable) domain, so www.example.com and example.com are one item.
func Items(servers []models.Server, sites []models.Site) []inventory.ExternalItem {
	items := make([]inventory.ExternalItem, 0, len(servers)+len(sites))

	for _, srv := range servers {
		items = append(items, inventory.ExternalItem{
			Type:         models.InventoryItemTypeServer,
			ProviderType: "server",
			ProviderID:   strconv.Itoa(srv.ID),
			Name:         srv.Name,
			Data: map[string]any{
				"server_id":         srv.ID,
				"name":              srv.Name,
				"provider_name":     srv.ProviderName,
				"ubuntu_version":    srv.UbuntuVersion,
				"ip_address":        srv.IPAddress,
				"ssh_port":          srv.SSHPort,
				"timezone":          srv.Timezone,
				"region":            srv.Region,
				"size":              srv.Size,
				"disk_space":        srv.DiskSpace,
				"database":          srv.Database,
				"connection_status": srv.ConnectionStatus,
				"reboot_required":   srv.RebootRequired,
				"upgrade_required":  srv.UpgradeRequired,
				"status":            srv.Status,
				"created_at":        srv.CreatedAt,
			},
		})
	}

	for _, site := range sites {
		siteUUID := utils.SpinupWPSiteUUID(site.ID)
		domains := make([]models.AdditionalDomain, 0, len(site.AdditionalDomains)+1)
		domains = append(domains, models.AdditionalDomain{Domain: site.Domain})
		domains = append(domains, site.AdditionalDomains...)
		for _, d := range domains {
			apex, err := utils.ApexDomain(d.Domain)
			if err != nil {
				verb.Printf(verb.Verbose, "inventory: skipping site %d domain %q: %v\n", site.ID, d.Domain, err)
				continue
			}
			items = append(items, inventory.ExternalItem{
				Type:         models.InventoryItemTypeDomain,
				ProviderType: "site_domain",
				ProviderID:   fmt.Sprintf("%d:%s", site.ID, d.Domain),
				Name:         apex,
				SiteID:       siteUUID,
				Data: map[string]any{
					"site_id":             site.ID,
					"site_uuid":           siteUUID,
					"domain":              d.Domain,
					"apex_domain":         apex,
					"additional":          site.Domain != d.Domain,
					"redirect":            d.Redirect.Enabled,
					"environment":         string(site.Environment),
					"php_version":         site.PHPVersion,
					"is_wordpress":        site.IsWordpress,
					"https_enabled":       site.HTTPS.Enabled,
					"certificate_expires": site.HTTPS.CertificateExpires,
					"status":              site.Status,
				},
			})
		}
	}

	return items
}
