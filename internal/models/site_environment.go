package models

import "strings"

// SiteEnvironmentType classifies a site's deployment environment.
type SiteEnvironmentType string

const (
	SiteEnvironmentProduction  SiteEnvironmentType = "production"
	SiteEnvironmentStaging     SiteEnvironmentType = "staging"
	SiteEnvironmentDevelopment SiteEnvironmentType = "development"
)

// InferEnvironmentFromDomain guesses a site's environment from its primary domain.
// It returns an empty SiteEnvironmentType if no pattern matches.
func InferEnvironmentFromDomain(domain string) SiteEnvironmentType {
	d := strings.ToLower(domain)

	switch {
	case strings.Contains(d, ".staging."):
		return SiteEnvironmentStaging
	case strings.Contains(d, ".dev."), strings.Contains(d, ".develop."), strings.Contains(d, ".development."):
		return SiteEnvironmentDevelopment
	default:
		return ""
	}
}
