package models

import (
	"fmt"
	"sort"
	"strings"
)

// SiteTier is a service tier a production site can be on. ID is the stable
// identifier stored for a site and the tier's rank: a higher ID is a higher
// tier, so code compares tiers by ID (site.Tier >= TierPremium). The IDs
// leave gaps, so a tier can later go between two others without
// renumbering. Name is display only and can change freely.
type SiteTier struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// The default tiers' IDs. Code that behaves differently by tier refers to
// these, never to a tier's name. Internal is jman's own sites, ranked above
// the customer tiers.
const (
	TierBasic      = 10
	TierStandard   = 20
	TierPremium    = 30
	TierEnterprise = 40
	TierInternal   = 90
)

// DefaultSiteTiers are the tiers used when the configuration names none.
var DefaultSiteTiers = []SiteTier{
	{ID: TierBasic, Name: "Basic"},
	{ID: TierStandard, Name: "Standard"},
	{ID: TierPremium, Name: "Premium"},
	{ID: TierEnterprise, Name: "Enterprise"},
	{ID: TierInternal, Name: "Internal"},
}

// ResolveSiteTiers returns the tiers in effect: the configured ones sorted
// by ID, or DefaultSiteTiers when none are configured. Configured tiers must
// have positive, unique IDs and non-empty names.
func ResolveSiteTiers(configured []SiteTier) ([]SiteTier, error) {
	if len(configured) == 0 {
		return append([]SiteTier(nil), DefaultSiteTiers...), nil
	}
	tiers := make([]SiteTier, 0, len(configured))
	seen := make(map[int]bool, len(configured))
	for _, t := range configured {
		name := strings.TrimSpace(t.Name)
		switch {
		case t.ID <= 0:
			return nil, fmt.Errorf("site tier %q: id must be a positive integer", name)
		case name == "":
			return nil, fmt.Errorf("site tier %d: name is empty", t.ID)
		case seen[t.ID]:
			return nil, fmt.Errorf("site tier id %d is used twice", t.ID)
		}
		seen[t.ID] = true
		tiers = append(tiers, SiteTier{ID: t.ID, Name: name})
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].ID < tiers[j].ID })
	return tiers, nil
}
