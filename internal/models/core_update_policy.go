package models

// CoreUpdatePolicy is what the nightly core update does for a site
// (docs/site-environments-tiers.md).
type CoreUpdatePolicy string

const (
	// CorePolicyVulnMinor updates to the latest minor version when the
	// site's WordPress version has an open, unmuted core vulnerability.
	CorePolicyVulnMinor CoreUpdatePolicy = "vuln_minor"
	// CorePolicyMinor updates to the latest minor version every night.
	CorePolicyMinor CoreUpdatePolicy = "minor"
	// CorePolicyMajor updates to the latest version, major or minor, every
	// night. An update-locked site still only gets minor updates.
	CorePolicyMajor CoreUpdatePolicy = "major"
)

// Valid reports whether p is one of the policies.
func (p CoreUpdatePolicy) Valid() bool {
	switch p {
	case CorePolicyVulnMinor, CorePolicyMinor, CorePolicyMajor:
		return true
	}
	return false
}

// DefaultCoreUpdatePolicy is a site's policy without an override: staging
// and development sites, and production sites on a tier below Standard,
// update to the latest version every night; the rest (higher tiers,
// production sites without a tier, archived sites) only on a
// vulnerability. tier is the site's own tier, 0 for none.
func DefaultCoreUpdatePolicy(env SiteEnvironmentType, tier int) CoreUpdatePolicy {
	switch env {
	case SiteEnvironmentStaging, SiteEnvironmentDevelopment:
		return CorePolicyMajor
	case SiteEnvironmentProduction:
		if tier > 0 && tier < TierStandard {
			return CorePolicyMajor
		}
	}
	return CorePolicyVulnMinor
}

// EffectiveCoreUpdatePolicy is override when it's set, else the default.
func EffectiveCoreUpdatePolicy(override CoreUpdatePolicy, env SiteEnvironmentType, tier int) CoreUpdatePolicy {
	if override.Valid() {
		return override
	}
	return DefaultCoreUpdatePolicy(env, tier)
}
