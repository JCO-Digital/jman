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
	vulnResponse, err := cache.GetCachedCoreVulnerabilities(coreVersion)
	if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
		return nil
	}

	cliSites, err := cache.GetFastSiteList()
	siteMeta := make(map[string]models.CliSite)
	if err == nil {
		for _, s := range cliSites {
			siteMeta[s.ID] = s
		}
	} else {
		verb.LogPrintf(verb.Verbose, "Warning: failed to fetch site list, server-level ignores may not be fully resolved: %v\n", err)
	}

	report := &models.CoreVulnReport{
		Version:         coreVersion,
		Vulnerabilities: []models.Vulnerability{},
	}

	for _, vulnerability := range vulnResponse.Data.Vulnerability {
		if matcher != nil && matcher.IsVulnerabilityUUIDIgnored(vulnerability.Uuid) {
			continue
		}
		// Every site here runs coreVersion, so a backport rule covering it
		// means none of them are affected.
		if matcher != nil && matcher.PatchedBy(vulnerability.Uuid, coreVersion) != nil {
			continue
		}

		v := vulnerability
		v.Sites = make([]models.PluginSite, 0, len(sites))

		allSitesSuppressed := true
		for _, site := range sites {
			if matcher != nil {
				serverID := ""
				if s, ok := siteMeta[site.SiteID]; ok {
					serverID = s.ServerID
				}
				site.Suppressed = matcher.IsSiteIgnored(site.SiteID, serverID)
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

// ProcessCoreVulnerabilities loads the cached WordPress core version inventory and vulnerability
// data, then determines which sites are affected by which core vulnerabilities.
func ProcessCoreVulnerabilities(matcher *db.VulnIgnoreMatcher) ([]models.CoreVulnReport, error) {
	reports := []models.CoreVulnReport{}

	versionData, err := cache.GetCachedCoreVersionData()
	if err != nil {
		return nil, err
	}

	for _, vd := range versionData {
		verb.Printf(verb.Verbose, "Processing core version: %s\n", vd.Version)
		report := GetVulnerabilityReportForCoreVersion(vd.Version, vd.Sites, matcher)
		if report != nil {
			reports = append(reports, *report)
		}
	}

	return reports, nil
}
