package models

import "testing"

func TestInferEnvironmentFromDomain(t *testing.T) {
	tests := []struct {
		domain string
		want   SiteEnvironmentType
	}{
		{"www.staging.example.com", SiteEnvironmentStaging},
		{"app.dev.example.com", SiteEnvironmentDevelopment},
		{"app.develop.example.com", SiteEnvironmentDevelopment},
		{"app.development.example.com", SiteEnvironmentDevelopment},
		{"WWW.STAGING.EXAMPLE.COM", SiteEnvironmentStaging},
		{"www.example.com", ""},
		{"development.example.com", ""},
	}

	for _, tt := range tests {
		if got := InferEnvironmentFromDomain(tt.domain); got != tt.want {
			t.Errorf("InferEnvironmentFromDomain(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}
