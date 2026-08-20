package logs

import "testing"

func TestNormalizeReferrerHost(t *testing.T) {
	tests := []struct {
		name    string
		referer string
		want    string
	}{
		{"https scheme", "https://example.com/about", "example.com"},
		{"http scheme", "http://example.com/about", "example.com"},
		{"bare domain, no scheme", "example.com", "example.com"},
		{"bare domain with path", "example.com/about", "example.com"},
		{"www prefix stripped", "https://www.example.com/about", "example.com"},
		{"bare www prefix stripped", "www.example.com", "example.com"},
		{"port stripped", "https://example.com:8443/about", "example.com"},
		{"query and fragment stripped", "https://example.com?x=1#y", "example.com"},
		{"mixed case normalized", "HTTPS://Example.COM/About", "example.com"},
		{"userinfo stripped", "https://user:pass@example.com/", "example.com"},
		{"empty", "", ""},
		{"scheme only", "https://", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeReferrerHost(tt.referer); got != tt.want {
				t.Errorf("normalizeReferrerHost(%q) = %q, want %q", tt.referer, got, tt.want)
			}
		})
	}
}

func TestNormalizeReferrerHost_UnifiesVariants(t *testing.T) {
	variants := []string{
		"https://example.com",
		"http://example.com",
		"example.com",
		"https://www.example.com",
		"www.example.com",
		"https://EXAMPLE.com/some/page?x=1",
	}
	for _, v := range variants {
		if got := normalizeReferrerHost(v); got != "example.com" {
			t.Errorf("normalizeReferrerHost(%q) = %q, want %q", v, got, "example.com")
		}
	}
}

func TestIsInternalReferrer(t *testing.T) {
	tests := []struct {
		name       string
		refHost    string
		siteDomain string
		want       bool
	}{
		{"exact match", "example.com", "example.com", true},
		{"site domain has www", "example.com", "www.example.com", true},
		{"external host", "other.com", "example.com", false},
		{"subdomain is treated as external", "blog.example.com", "example.com", false},
		{"empty site domain", "example.com", "", false},
		{"empty ref host", "", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInternalReferrer(tt.refHost, tt.siteDomain); got != tt.want {
				t.Errorf("isInternalReferrer(%q, %q) = %v, want %v", tt.refHost, tt.siteDomain, got, tt.want)
			}
		})
	}
}
