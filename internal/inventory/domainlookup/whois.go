package domainlookup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
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

// whoisServers are the registries WHOIS is used for: those whose RDAP
// leaves a field out, and those without RDAP. Add a TLD here with the keys
// of its WHOIS answer to support it.
var whoisServers = map[string]WhoisServer{
	// Traficom's RDAP (rdap.fi) has no registrar entity; its WHOIS does.
	"fi": {
		Address:        "whois.fi:43",
		Registrar:      []string{"registrar"},
		Expires:        []string{"expires"},
		Nameservers:    []string{"nserver"},
		ExpiresLayouts: []string{"2.1.2006 15:04:05", "2.1.2006"},
	},
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

// parseWhois reads a WHOIS answer's "key: value" lines into the fields
// result is missing. An answer with none of the configured keys (a rate
// limit notice, "not found", ...) is an error.
func parseWhois(body string, server WhoisServer, result *Result) error {
	fields := map[string][]string{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(strings.TrimRight(strings.TrimSpace(key), ".")))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		fields[key] = append(fields[key], value)
	}

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
		return fmt.Errorf("WHOIS answer had none of the expected fields")
	}
	return nil
}
