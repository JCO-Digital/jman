// Package domainlookup finds what a domain's registry publishes about it:
// its delegated nameservers, registrar and expiry date. It asks the
// registry's RDAP server, found through IANA's bootstrap file, and fills in
// what RDAP leaves out from the registry's WHOIS server where one is
// configured (whoisServers). Only the domain name is sent, straight to the
// registry. See docs/inventory.md.
package domainlookup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/idna"

	"github.com/JCO-Digital/jman/internal/utils"
)

// Result is what a registry publishes about one domain. A field the
// registry doesn't publish is empty.
type Result struct {
	Nameservers []string
	Registrar   string
	ExpiresAt   *time.Time
	// Source is "rdap", "whois" or "rdap+whois".
	Source string
}

func (r *Result) complete() bool {
	return len(r.Nameservers) > 0 && r.Registrar != "" && r.ExpiresAt != nil
}

// ErrNotFound means the registry has no such domain.
var ErrNotFound = errors.New("the registry has no record of this domain")

// ErrUnsupported means jman has no RDAP or WHOIS source for the domain's
// top-level domain.
var ErrUnsupported = errors.New("no RDAP or WHOIS source for this top-level domain")

// IANABootstrapURL is IANA's RDAP bootstrap file for domains (RFC 9224).
const IANABootstrapURL = "https://data.iana.org/rdap/dns.json"

// Client looks domains up. Its zero value is not usable; use NewClient.
type Client struct {
	HTTP         *http.Client
	BootstrapURL string
	// BootstrapTTL is how long a fetched bootstrap file is used.
	BootstrapTTL time.Duration
	// Dial opens a WHOIS connection; swappable in tests.
	Dial func(ctx context.Context, address string) (net.Conn, error)
	// WhoisServers maps a top-level domain to its WHOIS configuration.
	WhoisServers map[string]WhoisServer
	// MinInterval is the least time between two requests to one host, so
	// a batch of lookups doesn't trip a registry's rate limit.
	MinInterval time.Duration

	mu          sync.Mutex
	bootstrap   map[string]string // TLD -> RDAP base URL
	bootstrapAt time.Time

	paceMu   sync.Mutex
	lastSent map[string]time.Time
}

// NewClient returns a client for the live registries.
func NewClient() *Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &Client{
		HTTP:         utils.NewHTTPClient(20 * time.Second),
		BootstrapURL: IANABootstrapURL,
		BootstrapTTL: 24 * time.Hour,
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", address)
		},
		WhoisServers: whoisServers,
		MinInterval:  2 * time.Second,
	}
}

// Lookup asks the domain's registry about domain, which should be a
// registrable (apex) domain; an internationalized name may be given in
// Unicode or Punycode.
func (c *Client) Lookup(ctx context.Context, domain string) (*Result, error) {
	ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if err != nil {
		return nil, fmt.Errorf("invalid domain %q: %w", domain, err)
	}
	ascii = strings.ToLower(ascii)
	if !strings.Contains(ascii, ".") {
		return nil, fmt.Errorf("invalid domain %q: no top-level domain", domain)
	}

	result := &Result{}
	var rdapErr error
	base, err := c.rdapBase(ctx, ascii)
	if err != nil {
		return nil, err
	}
	if base != "" {
		rdapErr = c.lookupRDAP(ctx, base, ascii, result)
		if errors.Is(rdapErr, ErrNotFound) {
			return nil, rdapErr
		}
		if rdapErr == nil {
			result.Source = "rdap"
			if result.complete() {
				return result, nil
			}
		}
	}

	server, ok := c.whoisServerFor(ascii)
	if !ok {
		if base == "" {
			return nil, fmt.Errorf("%w (.%s)", ErrUnsupported, tldOf(ascii))
		}
		if rdapErr != nil {
			return nil, rdapErr
		}
		return result, nil
	}
	whoisErr := c.lookupWhois(ctx, server, ascii, result)
	switch {
	case whoisErr == nil && rdapErr == nil && base != "":
		result.Source = "rdap+whois"
	case whoisErr == nil:
		result.Source = "whois"
	case rdapErr == nil && base != "":
		// RDAP answered; WHOIS was only for the fields it left out.
	case rdapErr != nil:
		return nil, fmt.Errorf("%v; WHOIS: %w", rdapErr, whoisErr)
	default:
		return nil, whoisErr
	}
	return result, nil
}

// suffixes returns the domain's possible registry suffixes, longest first:
// "example.co.uk" -> ["co.uk", "uk"].
func suffixes(domain string) []string {
	labels := strings.Split(domain, ".")
	out := make([]string, 0, len(labels)-1)
	for i := 1; i < len(labels); i++ {
		out = append(out, strings.Join(labels[i:], "."))
	}
	return out
}

func tldOf(domain string) string {
	return domain[strings.LastIndex(domain, ".")+1:]
}

// pace waits until host may be sent another request.
func (c *Client) pace(ctx context.Context, host string) error {
	c.paceMu.Lock()
	if c.lastSent == nil {
		c.lastSent = map[string]time.Time{}
	}
	next := c.lastSent[host].Add(c.MinInterval)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	c.lastSent[host] = next
	c.paceMu.Unlock()

	wait := time.Until(next)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// normalizeNameservers lower-cases, strips trailing dots, de-duplicates and
// sorts, so lookups compare equal whatever order a registry lists them in.
func normalizeNameservers(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, ns := range in {
		ns = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(ns), "."))
		if ns == "" || seen[ns] {
			continue
		}
		seen[ns] = true
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}
