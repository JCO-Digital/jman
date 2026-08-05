package vuln

import (
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/utils"
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
	siteMeta := make(map[int]models.CliSite)
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

		v := vulnerability
		v.Sites = make([]models.PluginSite, 0, len(sites))

		allSitesSuppressed := true
		for _, site := range sites {
			if matcher != nil {
				serverID := 0
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

// ScanCoreVulnerabilities generates vulnerability-centric reports for WordPress core, applies the
// configured CVSS threshold, prints them, and optionally sends them to Slack.
func ScanCoreVulnerabilities(opts ScanOptions) error {
	matcher, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to load ignore entries: %v\n", err)
	}

	reports, err := ProcessCoreVulnerabilities(matcher)
	if err != nil {
		return err
	}

	for _, report := range reports {
		var activeVulns []models.Vulnerability
		for _, v := range report.Vulnerabilities {
			if v.Suppressed {
				continue
			}

			var activeSites []models.PluginSite
			for _, site := range v.Sites {
				if !site.Suppressed {
					activeSites = append(activeSites, site)
				}
			}
			if len(activeSites) > 0 {
				v.Sites = activeSites
				activeVulns = append(activeVulns, v)
			}
		}

		if len(activeVulns) == 0 {
			continue
		}
		report.Vulnerabilities = activeVulns

		maxCvss := getCoreCvss(report)
		if opts.CVSSThreshold > 0 && maxCvss < opts.CVSSThreshold {
			continue
		}

		message, err := formatCoreReport(report)
		if err != nil {
			continue
		}

		fmt.Println(message)

		if opts.Slack {
			force := maxCvss >= config.Cfg.CVSSThreshold
			slack.SendMessage(message, force)
		}
	}

	return nil
}

// formatCoreReport renders a single core-vulnerability report as plain text, mirroring formatReport.
func formatCoreReport(report models.CoreVulnReport) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", verb.Gray("WordPress Core:"), verb.Bold(report.Version))

	for i, v := range report.Vulnerabilities {
		if i > 0 {
			fmt.Fprintln(&sb, "---")
		}

		cvss := getVulnCvss(v)
		infoName := ""
		infoDesc := ""
		if v.Description != nil {
			infoDesc = *v.Description
		}
		infoDate := ""
		infoLink := ""

		for _, source := range v.Source {
			if infoName == "" && !strings.HasPrefix(source.Name, "CVE") {
				infoName = source.Name
			}
			if infoDesc == "" && source.Description != nil {
				infoDesc = *source.Description
			}
			if infoDate == "" && source.Date != nil {
				infoDate = *source.Date
			}
			if infoLink == "" && source.Link != "" {
				infoLink = source.Link
			}
		}

		if infoName == "" {
			infoName = v.Name
		}

		fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Vulnerability:"), verb.Yellow(utils.CleanHTML(infoName)))
		fmt.Fprintf(&sb, "%s %s\n", verb.Gray("UUID:"), verb.Cyan(v.Uuid))
		if infoDate != "" {
			fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Date:"), infoDate)
		}
		if cvss > 0 {
			fmt.Fprintf(&sb, "%s %s\n", verb.Gray("CVSS Score:"), colorCvss(cvss))
		}
		if infoDesc != "" {
			fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Description:"), utils.CleanHTML(infoDesc))
		}
		if infoLink != "" {
			fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Link:"), verb.Cyan(infoLink))
		}

		fmt.Fprintf(&sb, "\n%s\n", verb.Bold("Affected Sites:"))
		for _, site := range v.Sites {
			siteName, _ := getSiteName(site.SiteID)
			fmt.Fprintf(&sb, "  %s %s\n", verb.Green("→"), siteName)
		}
	}

	return sb.String(), nil
}

// getCoreCvss extracts the maximum CVSS numeric score across all vulnerabilities in a core report.
func getCoreCvss(report models.CoreVulnReport) float64 {
	var maxCvss float64
	for _, v := range report.Vulnerabilities {
		score := getVulnCvss(v)
		if score > maxCvss {
			maxCvss = score
		}
	}
	return maxCvss
}
