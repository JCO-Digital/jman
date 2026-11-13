package domainlookup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/utils"
)

// rdapBase returns the RDAP base URL of the domain's registry, or "" when
// the bootstrap file lists none for it.
func (c *Client) rdapBase(ctx context.Context, domain string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bootstrap == nil || time.Since(c.bootstrapAt) > c.BootstrapTTL {
		m, err := c.fetchBootstrap(ctx)
		if err != nil {
			if c.bootstrap == nil {
				return "", fmt.Errorf("RDAP bootstrap: %w", err)
			}
			// Keep using the old file; try again on the next lookup.
		} else {
			c.bootstrap = m
			c.bootstrapAt = time.Now()
		}
	}
	for _, suffix := range suffixes(domain) {
		if base, ok := c.bootstrap[suffix]; ok {
			return base, nil
		}
	}
	return "", nil
}

func (c *Client) fetchBootstrap(ctx context.Context) (map[string]string, error) {
	var file struct {
		Services [][][]string `json:"services"`
	}
	if err := c.getJSON(ctx, c.BootstrapURL, "application/json", &file); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, svc := range file.Services {
		if len(svc) < 2 || len(svc[1]) == 0 {
			continue
		}
		// Prefer an https URL when a registry lists several.
		base := svc[1][0]
		for _, u := range svc[1] {
			if strings.HasPrefix(u, "https://") {
				base = u
				break
			}
		}
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		for _, tld := range svc[0] {
			m[strings.ToLower(tld)] = base
		}
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("bootstrap file lists no registries")
	}
	return m, nil
}

// errHTTPStatus is an unexpected HTTP status from a registry.
type errHTTPStatus struct {
	URL    string
	Status int
}

func (e *errHTTPStatus) Error() string {
	return fmt.Sprintf("%s answered HTTP %d", e.URL, e.Status)
}

func (c *Client) getJSON(ctx context.Context, rawURL, accept string, v any) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if err := c.pace(ctx, u.Host); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", accept)
	utils.SetStandardHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &errHTTPStatus{URL: u.Host, Status: resp.StatusCode}
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// rdapDomain is the part of an RDAP domain object (RFC 9083) used here.
type rdapDomain struct {
	Events []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	Nameservers []struct {
		LDHName string `json:"ldhName"`
	} `json:"nameservers"`
	Entities []rdapEntity `json:"entities"`
}

type rdapEntity struct {
	Roles     []string          `json:"roles"`
	Handle    string            `json:"handle"`
	VCard     []json.RawMessage `json:"vcardArray"`
	Entities  []rdapEntity      `json:"entities"`
	PublicIDs []struct {
		Identifier string `json:"identifier"`
	} `json:"publicIds"`
}

// lookupRDAP fills result from the registry's RDAP server.
func (c *Client) lookupRDAP(ctx context.Context, base, domain string, result *Result) error {
	var d rdapDomain
	err := c.getJSON(ctx, base+"domain/"+url.PathEscape(domain), "application/rdap+json", &d)
	if status, ok := err.(*errHTTPStatus); ok && status.Status == http.StatusNotFound {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("RDAP: %w", err)
	}
	parseRDAP(&d, result)
	return nil
}

func parseRDAP(d *rdapDomain, result *Result) {
	for _, e := range d.Events {
		if e.Action != "expiration" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
			t = t.UTC()
			result.ExpiresAt = &t
		}
	}
	ns := make([]string, 0, len(d.Nameservers))
	for _, n := range d.Nameservers {
		ns = append(ns, n.LDHName)
	}
	result.Nameservers = normalizeNameservers(ns)
	if reg := findRegistrar(d.Entities); reg != nil {
		result.Registrar = entityName(reg)
	}
}

func findRegistrar(entities []rdapEntity) *rdapEntity {
	for i := range entities {
		for _, role := range entities[i].Roles {
			if role == "registrar" {
				return &entities[i]
			}
		}
	}
	return nil
}

// entityName is an entity's vCard "fn" (formatted name), else its handle.
func entityName(e *rdapEntity) string {
	if len(e.VCard) == 2 {
		var props [][]json.RawMessage
		if json.Unmarshal(e.VCard[1], &props) == nil {
			for _, p := range props {
				if len(p) < 4 {
					continue
				}
				var name, value string
				if json.Unmarshal(p[0], &name) == nil && name == "fn" &&
					json.Unmarshal(p[3], &value) == nil && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return strings.TrimSpace(e.Handle)
}
