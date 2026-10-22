package monitor

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// NormalizeRule checks a rule's match type and value, lower-casing the
// value: server UUIDs and host globs both compare case-insensitively.
func NormalizeRule(r *models.MonitorCheckRule) error {
	r.Value = strings.ToLower(strings.TrimSpace(r.Value))
	r.Note = strings.TrimSpace(r.Note)
	if r.Value == "" {
		return errors.New("value is required")
	}
	switch r.MatchType {
	case models.MonitorRuleMatchServer:
	case models.MonitorRuleMatchHost:
		if strings.Contains(r.Value, "/") {
			return errors.New("a host pattern can't contain /")
		}
		if _, err := path.Match(r.Value, ""); err != nil {
			return fmt.Errorf("invalid host pattern %q", r.Value)
		}
	default:
		return errors.New("match_type must be server or host")
	}
	if r.Action == "" {
		r.Action = models.MonitorRuleActionStartPaused
	}
	if r.Action != models.MonitorRuleActionStartPaused {
		return errors.New("action must be start_paused")
	}
	return nil
}

// ruleMatches reports whether a rule matches a site: its server, or its
// domain against a glob such as *.staging.*.
func ruleMatches(r models.MonitorCheckRule, site monitoredSite) bool {
	switch r.MatchType {
	case models.MonitorRuleMatchServer:
		return site.serverID != "" && strings.EqualFold(r.Value, site.serverID)
	case models.MonitorRuleMatchHost:
		ok, _ := path.Match(r.Value, strings.ToLower(site.domain))
		return ok
	}
	return false
}

// ruleFor returns the first rule matching a site, or nil.
func ruleFor(rules []models.MonitorCheckRule, site monitoredSite) *models.MonitorCheckRule {
	for i := range rules {
		if ruleMatches(rules[i], site) {
			return &rules[i]
		}
	}
	return nil
}

// RuleMatch is a site's default check that a rule matches.
type RuleMatch struct {
	CheckID string `json:"check_id"`
	SiteID  string `json:"site_id"`
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
}

// RuleMatches returns the existing default checks a rule matches,
// leaving out orphaned ones.
func RuleMatches(r models.MonitorCheckRule) ([]RuleMatch, error) {
	sites, _, err := loadMonitoredSites()
	if err != nil {
		return nil, err
	}
	checks, err := db.ListMonitorChecks()
	if err != nil {
		return nil, err
	}

	matches := []RuleMatch{}
	for _, c := range checks {
		if !c.IsDefault || c.OrphanedAt != nil {
			continue
		}
		site, ok := sites[c.SiteID]
		if !ok || !ruleMatches(r, site) {
			continue
		}
		matches = append(matches, RuleMatch{CheckID: c.ID, SiteID: c.SiteID, Domain: site.domain, Enabled: c.Enabled})
	}
	return matches, nil
}
