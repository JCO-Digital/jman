package models

import "testing"

func TestEffectiveCoreUpdatePolicy(t *testing.T) {
	for _, tc := range []struct {
		override CoreUpdatePolicy
		env      SiteEnvironmentType
		tier     int
		want     CoreUpdatePolicy
	}{
		{"", SiteEnvironmentStaging, 0, CorePolicyMajor},
		{"", SiteEnvironmentDevelopment, 0, CorePolicyMajor},
		{"", SiteEnvironmentProduction, TierBasic, CorePolicyMajor},
		{"", SiteEnvironmentProduction, 15, CorePolicyMajor},
		{"", SiteEnvironmentProduction, TierStandard, CorePolicyVulnMinor},
		{"", SiteEnvironmentProduction, TierEnterprise, CorePolicyVulnMinor},
		{"", SiteEnvironmentProduction, 0, CorePolicyVulnMinor},
		{"", SiteEnvironmentArchived, 0, CorePolicyVulnMinor},
		{"", "", 0, CorePolicyVulnMinor},
		{CorePolicyVulnMinor, SiteEnvironmentStaging, 0, CorePolicyVulnMinor},
		{CorePolicyMajor, SiteEnvironmentProduction, TierPremium, CorePolicyMajor},
		{CorePolicyMinor, SiteEnvironmentProduction, TierBasic, CorePolicyMinor},
		{"bogus", SiteEnvironmentStaging, 0, CorePolicyMajor},
	} {
		if got := EffectiveCoreUpdatePolicy(tc.override, tc.env, tc.tier); got != tc.want {
			t.Errorf("EffectiveCoreUpdatePolicy(%q, %q, %d) = %q, want %q", tc.override, tc.env, tc.tier, got, tc.want)
		}
	}
}
