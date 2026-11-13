// Package hetzner is the Hetzner Cloud provider of the asset inventory: it
// lists the cloud servers of every configured Hetzner project
// (config hetznerApiTokens) as inventory server items. See
// docs/inventory.md.
package hetzner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/inventory"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// Name is the provider's registry name.
const Name = "hetzner"

// APIBaseURL is the Hetzner Cloud API.
const APIBaseURL = "https://api.hetzner.cloud/v1"

// maxPages bounds the pages read per project (50 servers each), so a
// misbehaving API can't keep a sync paging forever.
const maxPages = 100

// Provider implements inventory.Provider for Hetzner Cloud.
type Provider struct {
	BaseURL string
	HTTP    *http.Client
	// Tokens returns the API tokens to list servers with; the configured
	// ones by default.
	Tokens func() []string
}

func init() {
	inventory.Register(&Provider{
		BaseURL: APIBaseURL,
		HTTP:    utils.NewHTTPClient(30 * time.Second),
		Tokens:  func() []string { return config.Cfg.HetznerAPITokens },
	})
}

// Name returns the provider name.
func (p *Provider) Name() string { return Name }

// Fetch lists the servers of every configured project. Any failure fails
// the whole fetch, so a sync never takes a project it couldn't read for
// one whose servers were all deleted.
func (p *Provider) Fetch(ctx context.Context) ([]inventory.ExternalItem, error) {
	var tokens []string
	for _, t := range p.Tokens() {
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		return nil, inventory.ErrNotConfigured
	}
	var items []inventory.ExternalItem
	seen := map[int64]bool{}
	for i, token := range tokens {
		servers, err := p.listServers(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("hetzner project %d of %d: %w", i+1, len(tokens), err)
		}
		for _, s := range servers {
			// The same project configured twice would list its servers
			// twice; server IDs are unique across Hetzner.
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			items = append(items, Item(s))
		}
	}
	return items, nil
}

// Server is the part of a Hetzner Cloud server object used here.
type Server struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Created string `json:"created"`

	PublicNet struct {
		IPv4 *struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
		IPv6 *struct {
			IP string `json:"ip"`
		} `json:"ipv6"`
	} `json:"public_net"`

	ServerType struct {
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		Cores        float64 `json:"cores"`
		Memory       float64 `json:"memory"` // GB
		Disk         float64 `json:"disk"`   // GB
		CPUType      string  `json:"cpu_type"`
		Architecture string  `json:"architecture"`
	} `json:"server_type"`

	Location struct {
		Name    string `json:"name"`
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"location"`

	Image *struct {
		Name        *string `json:"name"`
		Description string  `json:"description"`
		OSFlavor    string  `json:"os_flavor"`
		OSVersion   *string `json:"os_version"`
	} `json:"image"`

	PrimaryDiskSize float64           `json:"primary_disk_size"` // GB
	BackupWindow    *string           `json:"backup_window"`
	Labels          map[string]string `json:"labels"`
	Protection      struct {
		Delete bool `json:"delete"`
	} `json:"protection"`
}

type serversPage struct {
	Servers []Server `json:"servers"`
	Meta    struct {
		Pagination struct {
			NextPage *int `json:"next_page"`
		} `json:"pagination"`
	} `json:"meta"`
}

// listServers reads every page of one project's servers.
func (p *Provider) listServers(ctx context.Context, token string) ([]Server, error) {
	var all []Server
	page := 1
	for n := 0; n < maxPages; n++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"50"}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/servers?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		utils.SetStandardHeaders(req)
		resp, err := p.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		var body serversPage
		err = func() error {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				var apiErr struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&apiErr)
				if apiErr.Error.Code != "" {
					return fmt.Errorf("Hetzner API answered HTTP %d: %s (%s)", resp.StatusCode, apiErr.Error.Message, apiErr.Error.Code)
				}
				return fmt.Errorf("Hetzner API answered HTTP %d", resp.StatusCode)
			}
			return json.NewDecoder(resp.Body).Decode(&body)
		}()
		if err != nil {
			return nil, err
		}
		all = append(all, body.Servers...)
		next := body.Meta.Pagination.NextPage
		if next == nil || *next <= page {
			return all, nil
		}
		page = *next
	}
	return nil, fmt.Errorf("more than %d pages of servers", maxPages)
}

// Item maps a Hetzner server to an inventory external item. Only what
// describes the server goes into provider_data: not its ID or name (the
// link and the item have them), nor values that change all the time, such
// as traffic counters.
func Item(s Server) inventory.ExternalItem {
	data := map[string]any{
		"status":            s.Status,
		"server_type":       s.ServerType.Name,
		"cores":             s.ServerType.Cores,
		"memory_gb":         s.ServerType.Memory,
		"disk_gb":           s.PrimaryDiskSize,
		"cpu_type":          s.ServerType.CPUType,
		"architecture":      s.ServerType.Architecture,
		"location":          s.Location.Name,
		"city":              s.Location.City,
		"country":           s.Location.Country,
		"created_at":        s.Created,
		"delete_protection": s.Protection.Delete,
	}
	if s.PublicNet.IPv4 != nil {
		data["ipv4"] = s.PublicNet.IPv4.IP
	}
	if s.PublicNet.IPv6 != nil {
		data["ipv6"] = s.PublicNet.IPv6.IP
	}
	if s.Image != nil {
		data["image"] = s.Image.Description
		if s.Image.Name != nil && *s.Image.Name != "" {
			data["image"] = *s.Image.Name
		}
	}
	if s.BackupWindow != nil && *s.BackupWindow != "" {
		data["backup_window"] = *s.BackupWindow
	}
	if len(s.Labels) > 0 {
		labels := make(map[string]any, len(s.Labels))
		for k, v := range s.Labels {
			labels[k] = v
		}
		data["labels"] = labels
	}
	return inventory.ExternalItem{
		Type:         models.InventoryItemTypeServer,
		ProviderType: "cloud_server",
		ProviderID:   strconv.FormatInt(s.ID, 10),
		Name:         s.Name,
		Data:         data,
	}
}

// MatchKeys implements inventory.Matcher: a server is recognized by its
// IPv4 address, so it joins the SpinupWP server it runs.
func (p *Provider) MatchKeys(itemType models.InventoryItemType, data map[string]any) []string {
	if itemType != models.InventoryItemTypeServer {
		return nil
	}
	return inventory.IPv4MatchKeys(data["ipv4"])
}
