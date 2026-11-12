package utils

import (
	"fmt"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// ApexDomain reduces a host name to its registrable (apex) domain:
// "www.example.com" -> "example.com", "example.co.uk" -> "example.co.uk".
// It lowercases and strips a trailing dot first. An error is returned for
// input that has no registrable domain (an IP address, "localhost", ...).
func ApexDomain(host string) (string, error) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return "", fmt.Errorf("empty host")
	}
	apex, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return "", fmt.Errorf("%q has no registrable domain: %w", host, err)
	}
	return apex, nil
}
