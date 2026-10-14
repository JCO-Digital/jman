package vuln

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// ValidateVulnBackport checks a backport rule before it is saved. A returned
// error means the rule is malformed and must not be saved. Warnings flag
// rules that can't be verified against vulnerability data, or that look like
// they'd have no effect; such rules may still be saved.
func ValidateVulnBackport(rule models.VulnBackport) ([]string, error) {
	// Validate first: it rejects slugs and versions that would escape the
	// cache directory when used as cache file names below.
	if err := rule.Validate(); err != nil {
		return nil, err
	}

	if rule.Type == models.VulnBackportTypeCore {
		resp, err := cache.GetCachedCoreVulnerabilities(rule.MinVersion)
		if err != nil || resp == nil || resp.Data == nil {
			return []string{fmt.Sprintf("No vulnerability data for WordPress %s; the rule could not be verified.", rule.MinVersion)}, nil
		}
		for _, v := range resp.Data.Vulnerability {
			if v.Uuid == rule.VulnUUID {
				return nil, nil
			}
		}
		return []string{fmt.Sprintf("WordPress %s is not reported as affected by vulnerability %s, so the rule may have no effect.", rule.MinVersion, rule.VulnUUID)}, nil
	}

	resp, err := cache.GetCachedVulnerabilities(rule.Slug)
	if err != nil || resp == nil || resp.Data == nil {
		return []string{fmt.Sprintf("No vulnerability data for plugin %q; the rule could not be verified.", rule.Slug)}, nil
	}
	for _, v := range resp.Data.Vulnerability {
		if v.Uuid != rule.VulnUUID {
			continue
		}
		// Covers both a branch outside the affected range and a minimum at
		// or above the official fix.
		if !IsVersionAffected(rule.MinVersion, v.Operator) {
			return []string{fmt.Sprintf("Version %s is outside the vulnerability's affected range, so the rule has no effect.", rule.MinVersion)}, nil
		}
		return nil, nil
	}
	return []string{fmt.Sprintf("Vulnerability %s is not known for plugin %q.", rule.VulnUUID, rule.Slug)}, nil
}

// ListVulnBackports returns all backport rules with the number of sites whose
// installed plugin (active only) or core version each rule currently matches.
func ListVulnBackports() ([]models.VulnBackportListing, error) {
	rules, err := db.ListVulnBackports()
	if err != nil {
		return nil, err
	}

	plugins, cores, err := loadInstalledVersions()
	if err != nil {
		return nil, err
	}

	listings := make([]models.VulnBackportListing, 0, len(rules))
	for _, r := range rules {
		listings = append(listings, models.VulnBackportListing{
			VulnBackport: r,
			MatchedSites: countBackportMatches(r, plugins, cores),
		})
	}
	return listings, nil
}

// CountVulnBackportMatches returns the number of sites whose installed
// version the rule currently matches.
func CountVulnBackportMatches(rule models.VulnBackport) (int, error) {
	plugins, cores, err := loadInstalledVersions()
	if err != nil {
		return 0, err
	}
	return countBackportMatches(rule, plugins, cores), nil
}

func loadInstalledVersions() ([]models.WPPlugin, []models.SiteCore, error) {
	plugins, err := db.GetAllSitePlugins()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load site plugins: %w", err)
	}
	cores, err := db.GetAllSiteCore()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load site core versions: %w", err)
	}
	return plugins, cores, nil
}

func countBackportMatches(rule models.VulnBackport, plugins []models.WPPlugin, cores []models.SiteCore) int {
	n := 0
	if rule.Type == models.VulnBackportTypeCore {
		for _, c := range cores {
			if rule.Matches(c.Version) {
				n++
			}
		}
		return n
	}
	for _, p := range plugins {
		if p.Status == "active" && p.Name == rule.Slug && rule.Matches(p.Version) {
			n++
		}
	}
	return n
}
