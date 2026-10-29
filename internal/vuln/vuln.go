package vuln

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/hashicorp/go-version"
)

// IsVersionAffected checks if a specific version is within the range defined by a vulnerability operator.
func IsVersionAffected(ver string, op models.Operator) bool {
	// Default bounds:
	// - min: "0" (effectively no lower bound for semantic versions)
	// - max: ""  (treated as unbounded upper range)
	minVer := "0"
	minOp := "ge"
	if op.MinVersion != nil {
		minVer = *op.MinVersion
		if op.MinOperator != nil {
			minOp = *op.MinOperator
		}
	}
	maxVer := ""
	maxOp := "le"
	if op.MaxVersion != nil {
		maxVer = *op.MaxVersion
		if op.MaxOperator != nil {
			maxOp = *op.MaxOperator
		}
	}

	if maxVer != "" {
		if maxTest, _ := versionCompare(ver, maxVer, maxOp); !maxTest {
			return false
		}
	}
	if minTest, _ := versionCompare(ver, minVer, minOp); !minTest {
		return false
	}

	return true
}

// GetVulnerabilityReportsForPlugin finds all vulnerabilities affecting the provided sites for a given plugin.
// Vulnerabilities whose UUID appears in the ignore list are silently skipped. Sites whose version is
// covered by a backport rule are listed in PatchedSites instead of Sites.
func GetVulnerabilityReportsForPlugin(pluginName string, sites []models.PluginSite, matcher *db.VulnIgnoreMatcher) *models.VulnReport {
	vulnResponse, err := cache.GetCachedVulnerabilities(pluginName)
	if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
		// Missing or invalid vulnerability cache for this plugin; skip it.
		return nil
	}

	// Load site metadata for server ID lookups
	cliSites, err := cache.GetFastSiteList()
	siteMeta := make(map[string]models.CliSite)
	if err == nil {
		for _, s := range cliSites {
			siteMeta[s.ID] = s
		}
	} else {
		verb.LogPrintf(verb.Verbose, "Warning: failed to fetch site list, server-level ignores may not be fully resolved: %v\n", err)
	}

	pluginSuppressed := matcher != nil && matcher.IsPluginIgnored(pluginName)
	report := &models.VulnReport{
		Plugin:          pluginName,
		Slug:            pluginName,
		PluginName:      pluginName,
		Vulnerabilities: []models.Vulnerability{},
		Suppressed:      pluginSuppressed,
	}
	if vulnResponse.Data.Name != nil {
		report.PluginName = *vulnResponse.Data.Name
	}

	for _, vulnerability := range vulnResponse.Data.Vulnerability {
		// Specific vulnerability ignores continue to be completely ignored.
		if matcher != nil && matcher.IsVulnerabilityUUIDIgnored(vulnerability.Uuid) {
			continue
		}

		v := vulnerability
		v.Sites = []models.PluginSite{}

		// Add every site whose installed plugin version falls inside the affected range.
		allSitesSuppressed := true
		for _, site := range sites {
			if matcher != nil {
				serverID := ""
				if s, ok := siteMeta[site.SiteID]; ok {
					serverID = s.ServerID
				}
				site.Suppressed = matcher.IsSiteIgnored(site.SiteID, serverID)
			}

			if !IsVersionAffected(site.Version, v.Operator) {
				continue
			}
			// Versions fixed by a backport release aren't affected.
			if matcher != nil && matcher.PatchedBy(v.Uuid, site.Version) != nil {
				v.PatchedSites = append(v.PatchedSites, site)
				continue
			}
			v.Sites = append(v.Sites, site)
			if !site.Suppressed {
				allSitesSuppressed = false
			}
		}

		// Keep only vulnerabilities that affect at least one known site;
		// one whose affected sites are all patched by backports is dropped.
		if len(v.Sites) > 0 {
			// A vulnerability is suppressed if the plugin is suppressed OR all affected sites are suppressed.
			v.Suppressed = pluginSuppressed || allSitesSuppressed
			report.Vulnerabilities = append(report.Vulnerabilities, v)
		}
	}

	if len(report.Vulnerabilities) == 0 {
		return nil
	}

	return report
}

// IsPluginVulnerableOnSite reports whether the given version of a plugin
// on a site is affected by a known vulnerability that isn't suppressed by
// an ignore entry for the plugin, the vulnerability, the site or its server.
func IsPluginVulnerableOnSite(siteID, plugin, version string, matcher *db.VulnIgnoreMatcher) bool {
	report := GetVulnerabilityReportsForPlugin(plugin, []models.PluginSite{{SiteID: siteID, Version: version}}, matcher)
	if report == nil {
		return false
	}
	for _, v := range report.Vulnerabilities {
		if !v.Suppressed {
			return true
		}
	}
	return false
}

// PluginFixedInOnSite returns the lowest version that fixes every
// unsuppressed known vulnerability of the given plugin version on a site,
// as recorded by the vulnerability database. It returns "" if the version
// isn't vulnerable or if some vulnerability has no recorded fix. The
// recorded version is often the next minor or major release even when a
// fix release on the installed branch also fixes it.
func PluginFixedInOnSite(siteID, plugin, version string, matcher *db.VulnIgnoreMatcher) string {
	report := GetVulnerabilityReportsForPlugin(plugin, []models.PluginSite{{SiteID: siteID, Version: version}}, matcher)
	if report == nil {
		return ""
	}
	var vulns []models.Vulnerability
	for _, v := range report.Vulnerabilities {
		if !v.Suppressed {
			vulns = append(vulns, v)
		}
	}
	return FixedIn(vulns)
}

// FixedIn returns the lowest version that fixes every given vulnerability,
// as recorded by the vulnerability database. It returns "" if the list is
// empty or if some vulnerability has no recorded fix.
func FixedIn(vulns []models.Vulnerability) string {
	fixedIn := ""
	for _, v := range vulns {
		op := v.Operator
		if op.MaxVersion == nil || op.MaxOperator == nil || *op.MaxOperator != "lt" {
			return ""
		}
		if fixedIn == "" {
			fixedIn = *op.MaxVersion
		} else if newer, err := versionCompare(*op.MaxVersion, fixedIn, "gt"); err == nil && newer {
			fixedIn = *op.MaxVersion
		}
	}
	return fixedIn
}

// ProcessVulnerabilities loads cached plugin inventory and vulnerability data, then
// determines which sites are affected by which vulnerabilities based on version ranges.
func ProcessVulnerabilities(matcher *db.VulnIgnoreMatcher) ([]models.VulnReport, error) {
	reports := []models.VulnReport{}

	pluginData, err := cache.GetCachedPluginData()
	if err != nil {
		return nil, err
	}

	for _, plugin := range pluginData {
		verb.Printf(verb.Verbose, "Processing plugin: %s\n", plugin.Name)
		report := GetVulnerabilityReportsForPlugin(plugin.Name, plugin.Sites, matcher)
		if report != nil {
			reports = append(reports, *report)
		}
	}

	return reports, nil
}

// versionCompare compares v1 and v2 using semantic version parsing.
//
// If semantic parsing fails for either input, it returns an error.
func versionCompare(v1, v2, op string) (bool, error) {
	parsed1, err1 := version.NewVersion(v1)
	parsed2, err2 := version.NewVersion(v2)

	if err1 != nil || err2 != nil {
		return false, fmt.Errorf("failed to parse version: v1=%s v2=%s op=%s", v1, v2, op)
	}

	switch op {
	case "ge":
		return parsed1.GreaterThanOrEqual(parsed2), nil
	case "le":
		return parsed1.LessThanOrEqual(parsed2), nil
	case "gt":
		return parsed1.GreaterThan(parsed2), nil
	case "lt":
		return parsed1.LessThan(parsed2), nil
	default:
		return false, fmt.Errorf("unknown operator: %s", op)
	}
}
