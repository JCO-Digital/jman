package vuln

import (
	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// GetVulnerabilityReportForCoreVersion finds all vulnerabilities affecting a specific installed
// WordPress core version, attributed to the sites running that version.
//
// Unlike plugins, the wpvulnerability.net core endpoint already scopes vulnerabilities to the
// requested version, so every returned vulnerability applies to every site in sites.
func GetVulnerabilityReportForCoreVersion(coreVersion string, sites []models.PluginSite, matcher *db.VulnIgnoreMatcher) *models.CoreVulnReport {
	return coreReport(coreVersion, sites, matcher, loadSiteServers(matcher))
}

// coreReport is GetVulnerabilityReportForCoreVersion with the sites'
// server IDs already loaded (see loadSiteServers).
func coreReport(coreVersion string, sites []models.PluginSite, matcher *db.VulnIgnoreMatcher, siteServers map[string]string) *models.CoreVulnReport {
	vulnResponse, err := cache.ReadCachedCoreVulnerabilities(coreVersion)
	if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
		return nil
	}

	report := &models.CoreVulnReport{
		Version:         coreVersion,
		Vulnerabilities: []models.Vulnerability{},
	}

	for _, vulnerability := range vulnResponse.Data.Vulnerability {
		if coreVulnIgnored(matcher, vulnerability) {
			continue
		}
		// Every site here runs coreVersion, so a backport rule covering it
		// means none of them are affected.
		if corePatchedBy(matcher, vulnerability, coreVersion) != nil {
			continue
		}

		v := vulnerability
		v.PatchedSites = nil
		v.Sites = make([]models.PluginSite, 0, len(sites))

		allSitesSuppressed := true
		for _, site := range sites {
			if matcher != nil {
				site.Suppressed = matcher.IsSiteIgnored(site.SiteID, siteServers[site.SiteID])
			}

			v.Sites = append(v.Sites, site)
			if !site.Suppressed {
				allSitesSuppressed = false
			}
		}

		v.Suppressed = allSitesSuppressed
		report.Vulnerabilities = append(report.Vulnerabilities, v)
	}

	if len(report.Vulnerabilities) == 0 {
		return nil
	}

	return report
}

// coreVulnIgnored reports whether an ignore entry covers a core
// vulnerability. Entries made before core vulnerabilities had their own
// UUIDs name the feed's UUID, which covers all of a version's
// vulnerabilities.
func coreVulnIgnored(matcher *db.VulnIgnoreMatcher, v models.Vulnerability) bool {
	if matcher == nil {
		return false
	}
	return matcher.IsVulnerabilityUUIDIgnored(v.Uuid) ||
		(v.FeedUuid != "" && matcher.IsVulnerabilityUUIDIgnored(v.FeedUuid))
}

// corePatchedBy returns the backport rule under which coreVersion is not
// affected by a core vulnerability, matching rules made with the feed's
// UUID as in coreVulnIgnored.
func corePatchedBy(matcher *db.VulnIgnoreMatcher, v models.Vulnerability, coreVersion string) *models.VulnBackport {
	if matcher == nil {
		return nil
	}
	if r := matcher.PatchedBy(v.Uuid, coreVersion); r != nil {
		return r
	}
	if v.FeedUuid != "" {
		return matcher.PatchedBy(v.FeedUuid, coreVersion)
	}
	return nil
}

// ProcessCoreVulnerabilities matches the stored WordPress core versions
// against the cached vulnerability data and returns, per version, the
// vulnerabilities affecting the sites running it. Like
// ProcessVulnerabilities, it reads only stored data.
func ProcessCoreVulnerabilities(matcher *db.VulnIgnoreMatcher) ([]models.CoreVulnReport, error) {
	return processCoreVulnerabilities(matcher, "")
}

// processCoreVulnerabilities is ProcessCoreVulnerabilities for every site,
// or only for siteID.
func processCoreVulnerabilities(matcher *db.VulnIgnoreMatcher, siteID string) ([]models.CoreVulnReport, error) {
	reports := []models.CoreVulnReport{}

	versionData, err := cache.GetFastCachedCoreVersionData()
	if err != nil {
		return nil, err
	}
	siteServers := loadSiteServers(matcher)

	for _, vd := range versionData {
		sites := vd.Sites
		if siteID != "" {
			if sites = onlySiteInstalls(sites, siteID); len(sites) == 0 {
				continue
			}
		}
		verb.Printf(verb.Verbose, "Processing core version: %s\n", vd.Version)
		if report := coreReport(vd.Version, sites, matcher, siteServers); report != nil {
			reports = append(reports, *report)
		}
	}

	return reports, nil
}
