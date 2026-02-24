package vuln

import (
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verbosity"
	"github.com/hashicorp/go-version"
)

// ScanVulnerabilities runs the vulnerability scanner.
// target can be "sites", "cvss", or "slack".
func ScanVulnerabilities(target string, args []string) error {
	if target == "sites" {
		return scanSites(args)
	}
	return scanReports(target, args)
}

func scanSites(args []string) error {
	sitesMap, err := buildSiteList()
	if err != nil {
		return err
	}

	siteCount := 0
	for siteID, plugins := range sitesMap {
		var maxCvss float64 = 0
		var totalVulns int = 0

		for _, plugin := range plugins {
			totalVulns += len(plugin.Vulnerability)
			if plugin.Cvss != nil && *plugin.Cvss > maxCvss {
				maxCvss = *plugin.Cvss
			}
		}

		if maxCvss > config.Cfg.CVSSThreshold || float64(totalVulns) > config.Cfg.VulnThreshold {
			siteName, err := getSiteName(siteID)
			if err != nil {
				continue
			}

			// Check ignore list
			ignored := slices.Contains(config.Cfg.IgnoreSites, siteName)
			if ignored {
				continue
			}

			siteCount++
			message := formatSiteReport(fmt.Sprintf("%s (%d Vulnerabilities)", siteName, totalVulns), plugins)
			fmt.Println(message)

			// If "slack" is in args, send to slack
			sendToSlack := slices.Contains(args, "slack")

			if sendToSlack {
				slack.SendMessage(message, false)
			}
		}
	}

	fmt.Printf("%d sites match criteria\n", siteCount)
	return nil
}

func scanReports(target string, args []string) error {
	reports, err := ProcessVulnerabilities()
	if err != nil {
		return err
	}

	for _, report := range reports {
		cvss := getCvss(report)

		if target == "cvss" {
			cvssThreshold := config.Cfg.CVSSThreshold
			if len(args) > 1 {
				fmt.Sscanf(args[1], "%f", &cvssThreshold)
			}
			if cvss < cvssThreshold {
				continue
			}
		}

		message, err := formatReport(report)
		if err != nil {
			continue
		}

		fmt.Println(message)

		if target == "slack" {
			force := cvss >= config.Cfg.CVSSThreshold
			slack.SendMessage(message, force)
		}
	}

	return nil
}

func buildSiteList() (map[int]map[string]*models.VulnPlugin, error) {
	sitesMap := make(map[int]map[string]*models.VulnPlugin)

	reports, err := ProcessVulnerabilities()
	if err != nil {
		return nil, err
	}

	for _, report := range reports {
		cvss := getCvss(report)

		for _, site := range report.Sites {
			currentSite, ok := sitesMap[site.SiteID]
			if !ok {
				currentSite = make(map[string]*models.VulnPlugin)
				sitesMap[site.SiteID] = currentSite
			}

			currentPlugin, ok := currentSite[report.Plugin]
			if !ok {
				currentPlugin = &models.VulnPlugin{
					Version:       site.Version,
					Cvss:          &cvss,
					Vulnerability: []models.Vulnerability{},
				}
				currentSite[report.Plugin] = currentPlugin
			}

			if currentPlugin.Cvss == nil || cvss > *currentPlugin.Cvss {
				currentPlugin.Cvss = &cvss
			}

			currentPlugin.Vulnerability = append(currentPlugin.Vulnerability, report.Vulnerability)
		}
	}

	return sitesMap, nil
}

// ProcessVulnerabilities processes cached plugins and identifies vulnerable sites.
func ProcessVulnerabilities() ([]models.VulnReport, error) {
	var reports []models.VulnReport

	pluginData, err := cache.GetCachedPluginData()
	if err != nil {
		return nil, err
	}

	for _, plugin := range pluginData {
		verbosity.Printf(verbosity.Verbose, "Processing plugin: %s\n", plugin.Name)
		vulnResponse, err := cache.GetCachedVulnerabilities(plugin.Name, false)
		if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
			continue
		}

		for _, vulnerability := range vulnResponse.Data.Vulnerability {
			report := models.VulnReport{
				Plugin:        *vulnResponse.Data.Name,
				Vulnerability: vulnerability,
				Sites:         []models.PluginSite{},
			}

			minVer := "0"
			if vulnerability.Operator.MinVersion != nil {
				minVer = *vulnerability.Operator.MinVersion
			}
			maxVer := ""
			if vulnerability.Operator.MaxVersion != nil {
				maxVer = *vulnerability.Operator.MaxVersion
			}

			for _, site := range plugin.Sites {
				if versionIsNotBigger(site.Version, maxVer) && versionIsNotBigger(minVer, site.Version) {
					report.Sites = append(report.Sites, site)
				}
			}

			if len(report.Sites) > 0 {
				reports = append(reports, report)
			}
		}
	}

	return reports, nil
}

func formatReport(report models.VulnReport) (string, error) {
	cvss := getCvss(report)
	infoName := ""
	infoDesc := ""
	if report.Vulnerability.Description != nil {
		infoDesc = *report.Vulnerability.Description
	}
	infoDate := ""
	infoLink := ""

	var sb strings.Builder
	fmt.Fprintf(&sb, "Plugin: %s\n", cleanHTML(report.Plugin))

	for _, source := range report.Vulnerability.Source {
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
		infoName = report.Vulnerability.Name
	}

	fmt.Fprintf(&sb, "Vulnerability: %s\n", cleanHTML(infoName))
	if infoDate != "" {
		fmt.Fprintf(&sb, "Date: %s\n", infoDate)
	}
	if cvss > 0 {
		fmt.Fprintf(&sb, "CVS Score: %.1f\n", cvss)
	}
	if infoDesc != "" {
		fmt.Fprintf(&sb, "Description: %s\n", cleanHTML(infoDesc))
	}

	sb.WriteString("\nAffected Sites:\n")
	for _, site := range report.Sites {
		siteName, _ := getSiteName(site.SiteID)
		fmt.Fprintf(&sb, "  - %s (%s)\n", siteName, site.Version)
	}

	return sb.String(), nil
}

func formatSiteReport(siteTitle string, plugins map[string]*models.VulnPlugin) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", siteTitle)

	for pluginName, info := range plugins {
		fmt.Fprintf(&sb, "  %s - %s\n", cleanHTML(pluginName), info.Version)
		fmt.Fprintf(&sb, "    Vulnerabilities: %d\n", len(info.Vulnerability))
		if info.Cvss != nil {
			fmt.Fprintf(&sb, "    Highest CVSS: %.1f\n", *info.Cvss)
		}
	}

	return sb.String()
}

func getSiteName(siteID int) (string, error) {
	sites, err := cache.GetSiteList()
	if err != nil {
		return "", err
	}

	for _, site := range sites {
		if site.ID == siteID {
			return site.Name, nil
		}
	}
	return "", fmt.Errorf("site not found")
}

func getCvss(report models.VulnReport) float64 {
	if report.Vulnerability.Impact != nil && report.Vulnerability.Impact.Cvss != nil {
		var score float64
		fmt.Sscanf(report.Vulnerability.Impact.Cvss.Score, "%f", &score)
		return score
	}
	return 0
}

var htmlTagRegexp = regexp.MustCompile(`<[^>]*>`)

// cleanHTML strips HTML tags and decodes HTML entities from a string.
func cleanHTML(s string) string {
	s = htmlTagRegexp.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// versionIsNotBigger compares v1 and v2 using hashicorp/go-version comparison.
// It returns true if v1 is <= v2, or if v2 is empty.
func versionIsNotBigger(v1, v2 string) bool {
	if v2 == "" {
		return true
	}

	parsed1, err1 := version.NewVersion(v1)
	parsed2, err2 := version.NewVersion(v2)

	if err1 != nil || err2 != nil {
		// Fallback to simple string comparison if parsing fails
		return v1 <= v2
	}

	return parsed1.LessThanOrEqual(parsed2)
}
