package client

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := map[string]string{
		"https://jman.example.com":       "https://jman.example.com",
		"https://jman.example.com/":      "https://jman.example.com",
		"https://jman.example.com/api":   "https://jman.example.com",
		"https://jman.example.com/api/":  "https://jman.example.com",
		"jman.example.com":               "https://jman.example.com",
		"http://localhost:8080":          "http://localhost:8080",
		"https://example.com/jman/api?x": "https://example.com/jman",
	}
	for in, want := range tests {
		if got, err := NormalizeURL(in); err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x.example.com", "https://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) should fail", bad)
		}
	}
}

func TestVersionChecks(t *testing.T) {
	if CheckMinVersion("6.11.0", "6.12.0") == nil {
		t.Error("6.11.0 is below 6.12.0")
	}
	if err := CheckMinVersion("v6.12.0", "6.12.0"); err != nil {
		t.Errorf("equal versions: %v", err)
	}
	if err := CheckMinVersion("dev", "6.12.0"); err != nil {
		t.Errorf("dev builds aren't blocked: %v", err)
	}
	if err := CheckMinVersion("6.11.0", ""); err != nil {
		t.Errorf("no minimum: %v", err)
	}
	if !NewerAvailable("6.11.0", "6.11.1") || NewerAvailable("6.11.1", "6.11.1") || NewerAvailable("dev", "7.0.0") {
		t.Error("NewerAvailable")
	}
}
