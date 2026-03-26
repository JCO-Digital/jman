package vuln

import (
	"fmt"

	"slices"
	"sort"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/hashicorp/go-version"
)

// ScanVulnerabilities runs vulnerability scanning and reporting for the selected target.
//
// Supported targets:
//   - "sites": produce site-centric summaries (site -> vulnerable plugins/count/CVSS)
//   - "cvss": produce vulnerability-centric reports filtered by CVSS threshold
//   - "slack": produce vulnerability-centric reports and send them to Slack
//
// The args slice is interpreted by the selected mode:
//   - sites mode: if "slack" is present, site summaries are also sent to Slack.
//   - cvss mode: args[1], when present, is parsed as an override threshold.
func ScanVulnerabilities(target string, args []string) error {
	if target == "sites" {
		return scanSites(args)
	}
	return scanReports(target, args)
}

// scanSites builds a site-indexed vulnerability view, applies configured thresholds,
// prints matching site summaries, and optionally sends them to Slack.
//
// A site is reported when either:
//   - its highest plugin CVSS exceeds config.Cfg.CVSSThreshold, or
//   - its total vulnerability count exceeds config.Cfg.VulnThreshold.
//
// Site names listed in config.Cfg.IgnoreSites are skipped.
func scanSites(args []string) error {
	sitesMap, err := buildSiteList()
	if err != nil {
		return err
	}

	siteCount := 0
	for siteID, plugins := range sitesMap {
		var maxCvss float64 = 0
		var totalVulns int = 0

		// Aggregate per-site severity and vulnerability count from all vulnerable plugins.
		for _, plugin := range plugins {
			totalVulns += len(plugin.Vulnerability)
			if plugin.Cvss != nil && *plugin.Cvss > maxCvss {
				maxCvss = *plugin.Cvss
			}
		}

		// Report only sites that breach configured thresholds.
		if maxCvss > config.Cfg.CVSSThreshold || float64(totalVulns) > config.Cfg.VulnThreshold {
			siteName, err := getSiteName(siteID)
			if err != nil {
				// If a site cannot be resolved from cache, skip it and continue.
				continue
			}

			// Honor explicit site ignore list from config.
			ignored := slices.Contains(config.Cfg.IgnoreSites, siteName)
			if ignored {
				continue
			}

			siteCount++
			message := formatSiteReport(fmt.Sprintf("%s (%d Vulnerabilities)", siteName, totalVulns), plugins)
			fmt.Println(message)

			// If "slack" is in args, also send this site summary to Slack.
			sendToSlack := slices.Contains(args, "slack")
			if sendToSlack {
				slack.SendMessage(message, false)
			}
		}
	}

	verb.Printf(verb.Verbose, "%d sites match criteria\n", siteCount)
	return nil
}

// scanReports generates vulnerability-centric reports and handles target-specific filtering/output.
//
// Behavior by target:
//   - "cvss": filters reports by configured threshold, optionally overridden by args[1].
//   - "slack": prints all reports and sends each one to Slack.
//   - other: prints all reports without additional filtering.
//
// For Slack sending, "force" is enabled only when report CVSS is at/above the configured threshold.
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
			// Skip malformed/unformattable entries without stopping the whole scan.
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

// buildSiteList transforms vulnerability-centric reports into a site-centric structure.
//
// Returned map shape:
//   - key: site ID
//   - value: map[pluginName]*models.VulnPlugin
//
// Each VulnPlugin entry contains the plugin version found on that site,
// the highest CVSS among matched vulnerabilities, and the vulnerability list.
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

			// Keep the maximum CVSS observed for this plugin on this site.
			if currentPlugin.Cvss == nil || cvss > *currentPlugin.Cvss {
				currentPlugin.Cvss = &cvss
			}

			currentPlugin.Vulnerability = append(currentPlugin.Vulnerability, report.Vulnerability)
		}
	}

	return sitesMap, nil
}

// ProcessVulnerabilities loads cached plugin inventory and vulnerability data, then
// determines which sites are affected by which vulnerabilities based on version ranges.
//
// Matching rule per site:
//   - site.Version <= vulnerability.maxVersion (or max version is unbounded), and
//   - vulnerability.minVersion <= site.Version.
//
// For each vulnerability with at least one affected site, a models.VulnReport is emitted.
func ProcessVulnerabilities() ([]models.VulnReport, error) {
	var reports []models.VulnReport

	pluginData, err := cache.GetCachedPluginData()
	if err != nil {
		return nil, err
	}

	for _, plugin := range pluginData {
		verb.Printf(verb.Verbose, "Processing plugin: %s\n", plugin.Name)
		vulnResponse, err := cache.GetCachedVulnerabilities(plugin.Name)
		if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
			// Missing or invalid vulnerability cache for this plugin; skip it.
			continue
		}

		for _, vulnerability := range vulnResponse.Data.Vulnerability {
			report := models.VulnReport{
				Plugin:        *vulnResponse.Data.Name,
				Vulnerability: vulnerability,
				Sites:         []models.PluginSite{},
			}

			// Default bounds:
			// - min: "0" (effectively no lower bound for semantic versions)
			// - max: ""  (treated as unbounded upper range)
			minVer := "0"
			minOp := "ge"
			if vulnerability.Operator.MinVersion != nil {
				minVer = *vulnerability.Operator.MinVersion
				if vulnerability.Operator.MinOperator != nil {
					minOp = *vulnerability.Operator.MinOperator
				}
			}
			maxVer := ""
			maxOp := "le"
			if vulnerability.Operator.MaxVersion != nil {
				maxVer = *vulnerability.Operator.MaxVersion
				if vulnerability.Operator.MaxOperator != nil {
					maxOp = *vulnerability.Operator.MaxOperator
				}
			}

			// Add every site whose installed plugin version falls inside [minVer, maxVer].
			for _, site := range plugin.Sites {
				if maxVer != "" {
					if maxTest, _ := versionCompare(site.Version, maxVer, maxOp); !maxTest {
						continue
					}
				}
				if minTest, _ := versionCompare(site.Version, minVer, minOp); !minTest {
					continue
				}

				report.Sites = append(report.Sites, site)
			}

			// Keep only vulnerabilities that affect at least one known site.
			if len(report.Sites) > 0 {
				reports = append(reports, report)
			}
		}
	}

	return reports, nil
}

// formatReport renders a single vulnerability-centric report as plain text.
//
// It prefers enriched metadata from vulnerability sources (name/description/date/link),
// then falls back to base vulnerability fields where needed.
// User-facing text is HTML-cleaned before display.
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
	fmt.Fprintf(&sb, "Plugin: %s\n", utils.CleanHTML(report.Plugin))

	// Pull the first useful values from source metadata.
	// Non-CVE names are preferred for readability when available.
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

	fmt.Fprintf(&sb, "Vulnerability: %s\n", utils.CleanHTML(infoName))
	if infoDate != "" {
		fmt.Fprintf(&sb, "Date: %s\n", infoDate)
	}
	if cvss > 0 {
		fmt.Fprintf(&sb, "CVSS Score: %.1f\n", cvss)
	}
	if infoDesc != "" {
		fmt.Fprintf(&sb, "Description: %s\n", utils.CleanHTML(infoDesc))
	}
	if infoLink != "" {
		fmt.Fprintf(&sb, "Link: %s\n", infoLink)
	}

	sb.WriteString("\nAffected Sites:\n")
	for _, site := range report.Sites {
		siteName, _ := getSiteName(site.SiteID)
		fmt.Fprintf(&sb, "  - %s (%s)\n", siteName, site.Version)
	}

	return sb.String(), nil
}

// formatSiteReport renders a compact site-centric summary showing each vulnerable plugin,
// number of matched vulnerabilities, and the plugin's highest CVSS at that site.
func formatSiteReport(siteTitle string, plugins map[string]*models.VulnPlugin) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", siteTitle)

	// Extract keys and sort them for consistent output.
	keys := make([]string, 0, len(plugins))
	for pluginName := range plugins {
		keys = append(keys, pluginName)
	}
	sort.Strings(keys)

	for _, pluginName := range keys {
		info := plugins[pluginName]
		fmt.Fprintf(&sb, "  %s - %s\n", utils.CleanHTML(pluginName), info.Version)
		fmt.Fprintf(&sb, "    Vulnerabilities: %d\n", len(info.Vulnerability))
		if info.Cvss != nil {
			fmt.Fprintf(&sb, "    Highest CVSS: %.1f\n", *info.Cvss)
		}
	}

	return sb.String()
}

// getSiteName resolves a site ID to site name from cached site list data.
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

// getCvss extracts and parses a CVSS numeric score from a vulnerability report.
// Returns 0 when no score is available or parsing is not possible.
func getCvss(report models.VulnReport) float64 {
	if report.Vulnerability.Impact != nil && report.Vulnerability.Impact.Cvss != nil {
		var score float64
		fmt.Sscanf(report.Vulnerability.Impact.Cvss.Score, "%f", &score)
		return score
	}
	return 0
}

// versionCompare compares v1 and v2 using semantic version parsing.
//
// If semantic parsing fails for either input, it falls back to lexicographic string compare.
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
