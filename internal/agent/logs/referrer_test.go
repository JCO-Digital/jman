package logs

import "testing"

func TestIsInternalReferrer(t *testing.T) {
	tests := []struct {
		name       string
		referer    string
		siteDomain string
		want       bool
	}{
		{"exact match", "https://example.com/about", "example.com", true},
		{"referrer has www, domain doesn't", "https://www.example.com/about", "example.com", true},
		{"domain has www, referrer doesn't", "https://example.com/about", "www.example.com", true},
		{"referrer has port", "https://example.com:8443/about", "example.com", true},
		{"external host", "https://other.com/", "example.com", false},
		{"subdomain is treated as external", "https://blog.example.com/", "example.com", false},
		{"empty site domain", "https://example.com/", "", false},
		{"malformed referer", "://not a url", "example.com", false},
		{"opaque referer with no host", "not-a-url", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInternalReferrer(tt.referer, tt.siteDomain); got != tt.want {
				t.Errorf("isInternalReferrer(%q, %q) = %v, want %v", tt.referer, tt.siteDomain, got, tt.want)
			}
		})
	}
}
