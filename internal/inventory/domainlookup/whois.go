package domainlookup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// WhoisServer is how to read one registry's WHOIS answers: its server and,
// for each field, the keys of the "key: value" lines that hold it (compared
// lower-case, with the dots some registries pad keys with removed).
type WhoisServer struct {
	Address     string // host:port
	Registrar   []string
	Expires     []string
	Nameservers []string
	// ExpiresLayouts are the time layouts the expiry date may be in. A date
	// without a zone is read as UTC; only the day matters.
	ExpiresLayouts []string
}

// Expiry date layouts WHOIS servers use.
var (
	isoDate = []string{"2006-01-02"}
	// gTLD-style answers: "Registry Expiry Date: 2027-05-01T00:00:02Z".
	rfc3339 = []string{time.RFC3339}
)

// gtldStyle is the WHOIS answer format of ICANN's gTLD contracts, which
// several ccTLD registries use too.
func gtldStyle(address string) WhoisServer {
	return WhoisServer{
		Address:        address,
		Registrar:      []string{"registrar"},
		Expires:        []string{"registry expiry date"},
		Nameservers:    []string{"name server"},
		ExpiresLayouts: rfc3339,
	}
}

// whoisServers are the registries WHOIS is used for: those whose RDAP
// leaves a field out, and those without RDAP. Each entry's keys come from a
// real answer of that server. A key "section.key" is a key inside a
// section (a line "Registrar:" followed by "name: ..."); a section's own
// key holds its lines without a key (".eu" lists nameservers that way).
// Add a TLD here with the keys of its WHOIS answer to support it.
var whoisServers = map[string]WhoisServer{
	// Traficom's RDAP (rdap.fi) has no registrar entity; its WHOIS does.
	"fi": {
		Address:        "whois.fi:43",
		Registrar:      []string{"registrar"},
		Expires:        []string{"expires"},
		Nameservers:    []string{"nserver"},
		ExpiresLayouts: []string{"2.1.2006 15:04:05", "2.1.2006"},
	},
	"se": {
		Address:        "whois.iis.se:43",
		Registrar:      []string{"registrar"},
		Expires:        []string{"expires"},
		Nameservers:    []string{"nserver"},
		ExpiresLayouts: isoDate,
	},
	// Sections: "Registrar:" then "name: ...", next to the registrant's
	// and contacts' own "name:" lines.
	"ee": {
		Address:        "whois.tld.ee:43",
		Registrar:      []string{"registrar.name"},
		Expires:        []string{"expire"},
		Nameservers:    []string{"nserver"},
		ExpiresLayouts: isoDate,
	},
	// Punktum dk publishes no registrar for domains registered directly
	// with it; "registrar" is read when there is one.
	"dk": {
		Address:        "whois.punktum.dk:43",
		Registrar:      []string{"registrar"},
		Expires:        []string{"expires"},
		Nameservers:    []string{"hostname"},
		ExpiresLayouts: isoDate,
	},
	"io": gtldStyle("whois.nic.io:43"),
	"me": gtldStyle("whois.nic.me:43"),
	"ie": gtldStyle("whois.weare.ie:43"),
	// EURid publishes no expiry date. Indented sections: "Registrar:" then
	// "Name: ...", and "Name servers:" then one hostname (with its
	// addresses) per line.
	"eu": {
		Address:     "whois.eu:43",
		Registrar:   []string{"registrar.name"},
		Nameservers: []string{"name servers"},
	},
}

// rdapServers are registries with RDAP that IANA's bootstrap file doesn't
// list. SWITCH answers no port-43 WHOIS for .ch, but its RDAP has the
// nameservers and registrar (it publishes no expiry date).
var rdapServers = map[string]string{
	"ch": "https://rdap.nic.ch/",
}

func (c *Client) whoisServerFor(domain string) (WhoisServer, bool) {
	for _, suffix := range suffixes(domain) {
		if s, ok := c.WhoisServers[suffix]; ok {
			return s, true
		}
	}
	return WhoisServer{}, false
}

// maxWhoisResponse caps how much of a WHOIS answer is read.
const maxWhoisResponse = 64 * 1024

// lookupWhois fills the fields result is missing from the WHOIS server.
func (c *Client) lookupWhois(ctx context.Context, server WhoisServer, domain string, result *Result) error {
	host, _, _ := net.SplitHostPort(server.Address)
	if err := c.pace(ctx, host); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	conn, err := c.Dial(ctx, server.Address)
	if err != nil {
		return fmt.Errorf("WHOIS %s: %w", host, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := io.WriteString(conn, domain+"\r\n"); err != nil {
		return fmt.Errorf("WHOIS %s: %w", host, err)
	}
	body, err := io.ReadAll(io.LimitReader(conn, maxWhoisResponse))
	if err != nil {
		return fmt.Errorf("WHOIS %s: %w", host, err)
	}
	return parseWhois(decodeWhois(body), server, result)
}

// decodeWhois turns a WHOIS answer into a string. WHOIS has no declared
// encoding: UTF-8 is used as it is, anything else is read as Latin-1, which
// is what servers such as whois.fi send.
func decodeWhois(body []byte) string {
	if utf8.Valid(body) {
		return string(body)
	}
	runes := make([]rune, len(body))
	for i, b := range body {
		runes[i] = rune(b)
	}
	return string(runes)
}

// whoisKey matches the key part of a "key: value" line. It excludes
// parentheses, so "ns1.example.eu (2001:db8::1)" isn't read as a key.
var whoisKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 ._/-]*$`)

// notFound matches the answers registries give for a domain they don't have.
var notFound = regexp.MustCompile(`(?i)(not found|no match|no entries found|no data found|status:\s*free|is free|available for registration)`)

// whoisFields reads a WHOIS answer's lines into lower-cased keys. A line
// "Key:" with no value starts a section: the "key: value" lines after it
// are also stored as "section.key", and its lines without a key under the
// section's own key. A blank line ends a section.
func whoisFields(body string) map[string][]string {
	fields := map[string][]string{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			section = ""
			continue
		}
		if strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ">>>") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(key), "."))
		if !ok || !whoisKey.MatchString(key) {
			if section != "" {
				fields[section] = append(fields[section], line)
			}
			continue
		}
		key = strings.ToLower(key)
		value = strings.TrimSpace(value)
		if value == "" {
			section = key
			continue
		}
		fields[key] = append(fields[key], value)
		if section != "" {
			fields[section+"."+key] = append(fields[section+"."+key], value)
		}
	}
	return fields
}

// parseWhois reads a WHOIS answer into the fields result is missing. An
// answer saying the domain doesn't exist is ErrNotFound; one with none of
// the configured keys (a rate limit notice, ...) is an error.
func parseWhois(body string, server WhoisServer, result *Result) error {
	fields := whoisFields(body)

	first := func(keys []string) string {
		for _, k := range keys {
			if v := fields[k]; len(v) > 0 {
				return v[0]
			}
		}
		return ""
	}

	found := false
	if reg := first(server.Registrar); reg != "" {
		found = true
		if result.Registrar == "" {
			result.Registrar = reg
		}
	}
	if exp := first(server.Expires); exp != "" {
		found = true
		if result.ExpiresAt == nil {
			for _, layout := range server.ExpiresLayouts {
				if t, err := time.Parse(layout, exp); err == nil {
					t = t.UTC()
					result.ExpiresAt = &t
					break
				}
			}
		}
	}
	var ns []string
	for _, k := range server.Nameservers {
		for _, v := range fields[k] {
			// "ns1.example.com [OK]", "ns1.example.com 192.0.2.1"
			if host, _, _ := strings.Cut(v, " "); host != "" {
				ns = append(ns, host)
			}
		}
	}
	if len(ns) > 0 {
		found = true
		if len(result.Nameservers) == 0 {
			result.Nameservers = normalizeNameservers(ns)
		}
	}
	if !found {
		if notFound.MatchString(body) {
			return ErrNotFound
		}
		return fmt.Errorf("WHOIS answer had none of the expected fields")
	}
	return nil
}
