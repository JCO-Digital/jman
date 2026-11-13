package utils

import (
	"fmt"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// ApexDomain reduces a host name to its registrable (apex) domain:
// "www.example.com" -> "example.com", "example.co.uk" -> "example.co.uk".
// It lowercases and strips a trailing dot first, and returns an
// internationalized name in its ASCII (Punycode) form ("www.billnäs.fi" ->
// "xn--billns-gua.fi"), so a name compares equal whichever form it came in.
// An error is returned for input that is not a domain name or has no
// registrable domain (an IP address, "localhost", ...).
func ApexDomain(host string) (string, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return "", fmt.Errorf("empty host")
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", fmt.Errorf("%q is not a domain name: %w", host, err)
	}
	ascii = strings.ToLower(ascii)
	if !strings.Contains(ascii, ".") {
		return "", fmt.Errorf("%q has no top-level domain", host)
	}
	apex, err := publicsuffix.EffectiveTLDPlusOne(ascii)
	if err != nil {
		return "", fmt.Errorf("%q has no registrable domain: %w", host, err)
	}
	return apex, nil
}
