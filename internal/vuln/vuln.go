package vuln

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/hashicorp/go-version"
)

// ScanOptions defines the parameters for a vulnerability scan.
type ScanOptions struct {
	Mode          string  // "list" or "sites"
	Slack         bool    // whether to send reports to Slack
	CVSSThreshold float64 // threshold for reporting (0 to disable)
	SiteSearch    string  // filter by site name (case-insensitive)
}

// ScanVulnerabilities runs vulnerability scanning and reporting based on the provided options.
//
// Supported modes:
//   - "list": produce vulnerability-centric reports (vulnerability -> affected sites)
//   - "sites": produce site-centric summaries (site -> vulnerable plugins/count/CVSS)
//
// If SiteSearch is provided with mode "list", only vulnerabilities affecting the matching site
// are shown in full list format, without applying thresholds.
func ScanVulnerabilities(opts ScanOptions) error {
	if opts.Mode == "sites" {
		return scanSites(opts)
	}
	return scanReports(opts)
}

// scanSites builds a site-indexed vulnerability view, applies configured thresholds,
// prints matching site summaries, and optionally sends them to Slack.
//
// A site is reported when either:
//   - its highest plugin CVSS meets or exceeds configured threshold, or
//   - its total vulnerability count meets or exceeds config.Cfg.VulnThreshold.
//
// Site names listed in config.Cfg.IgnoreSites are skipped.
func scanSites(opts ScanOptions) error {
	sitesMap, err := buildSiteList()
	if err != nil {
		return err
	}

	// Sort site IDs for consistent output.
	siteIDs := make([]int, 0, len(sitesMap))
	for siteID := range sitesMap {
		siteIDs = append(siteIDs, siteID)
	}
	sort.Ints(siteIDs)

	siteCount := 0
	for _, siteID := range siteIDs {
		plugins := sitesMap[siteID]
		siteName, err := getSiteName(siteID)
		if err != nil {
			// If a site cannot be resolved from cache, skip it and continue.
			continue
		}

		// Honor explicit site ignore list from config.
		if slices.Contains(config.Cfg.IgnoreSites, siteName) {
			continue
		}

		var maxCvss float64 = 0
		var totalVulns int = 0

		// Aggregate per-site severity and vulnerability count from all vulnerable plugins.
		for _, plugin := range plugins {
			totalVulns += len(plugin.Vulnerability)
			if plugin.Cvss != nil && *plugin.Cvss > maxCvss {
				maxCvss = *plugin.Cvss
			}
		}

		cvssThreshold := opts.CVSSThreshold
		if cvssThreshold <= 0 {
			cvssThreshold = config.Cfg.CVSSThreshold
		}
		if maxCvss < cvssThreshold && float64(totalVulns) < config.Cfg.VulnThreshold {
			continue
		}

		siteCount++
		message := formatSiteReport(fmt.Sprintf("%s (%d Vulnerabilities)", siteName, totalVulns), plugins, false)
		fmt.Println(message)

		// If Slack is enabled, also send this site summary to Slack.
		if opts.Slack {
			slack.SendMessage(message, false)
		}
	}

	verb.Printf(verb.Verbose, "%d sites match criteria\n", siteCount)
	return nil
}

// scanReports generates vulnerability-centric reports and handles filtering and output.
//
// If SiteSearch is provided, only reports where at least one site name matches are shown,
// the Affected Sites list is trimmed to matching sites only, and thresholds are not applied.
// For Slack sending, "force" is enabled only when report CVSS is at/above the configured threshold.
func scanReports(opts ScanOptions) error {
	reports, err := ProcessVulnerabilities()
	if err != nil {
		return err
	}

	for _, report := range reports {
		cvss := getCvss(report)

		if opts.SiteSearch != "" {
			// Filter sites to those matching the search term.
			var matched []models.PluginSite
			for _, site := range report.Sites {
				siteName, err := getSiteName(site.SiteID)
				if err != nil {
					continue
				}
				if strings.Contains(strings.ToLower(siteName), strings.ToLower(opts.SiteSearch)) {
					matched = append(matched, site)
				}
			}
			if len(matched) == 0 {
				continue
			}
			report.Sites = matched
		} else {
			// Apply CVSS threshold filter only when not doing a site search.
			if opts.CVSSThreshold > 0 && cvss < opts.CVSSThreshold {
				continue
			}
		}

		message, err := formatReport(report)
		if err != nil {
			// Skip malformed/unformattable entries without stopping the whole scan.
			continue
		}

		fmt.Println(message)

		if opts.Slack {
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

			currentPlugin, ok := currentSite[report.Slug]
			if !ok {
				currentPlugin = &models.VulnPlugin{
					PluginName:    report.PluginName,
					Version:       site.Version,
					Cvss:          &cvss,
					Vulnerability: []models.Vulnerability{},
				}
				currentSite[report.Slug] = currentPlugin
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
// Vulnerabilities whose UUID appears in the ignore list are silently skipped.
func GetVulnerabilityReportsForPlugin(pluginName string, sites []models.PluginSite) []models.VulnReport {
	reports := []models.VulnReport{}

	vulnResponse, err := cache.GetCachedVulnerabilities(pluginName)
	if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
		// Missing or invalid vulnerability cache for this plugin; skip it.
		return reports
	}

	ignoredMap, err := db.GetIgnoredVulnMap()
	if err != nil {
		verb.Printf(verb.Verbose, "Warning: could not load vuln ignore list: %v\n", err)
		ignoredMap = map[string]bool{}
	}

	for _, vulnerability := range vulnResponse.Data.Vulnerability {
		if ignoredMap[vulnerability.Uuid] {
			verb.Printf(verb.Verbose, "Skipping ignored vulnerability: %s\n", vulnerability.Uuid)
			continue
		}

		report := models.VulnReport{
			Plugin:        pluginName,
			Slug:          pluginName,
			PluginName:    pluginName,
			Vulnerability: vulnerability,
			Sites:         []models.PluginSite{},
		}
		if vulnResponse.Data.Name != nil {
			report.PluginName = *vulnResponse.Data.Name
		}

		// Add every site whose installed plugin version falls inside the affected range.
		for _, site := range sites {
			if IsVersionAffected(site.Version, vulnerability.Operator) {
				report.Sites = append(report.Sites, site)
			}
		}

		// Keep only vulnerabilities that affect at least one known site.
		if len(report.Sites) > 0 {
			reports = append(reports, report)
		}
	}

	return reports
}

// ProcessVulnerabilities loads cached plugin inventory and vulnerability data, then
// determines which sites are affected by which vulnerabilities based on version ranges.
func ProcessVulnerabilities() ([]models.VulnReport, error) {
	reports := []models.VulnReport{}

	pluginData, err := cache.GetCachedPluginData()
	if err != nil {
		return nil, err
	}

	for _, plugin := range pluginData {
		verb.Printf(verb.Verbose, "Processing plugin: %s\n", plugin.Name)
		reports = append(reports, GetVulnerabilityReportsForPlugin(plugin.Name, plugin.Sites)...)
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
	fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Plugin:"), verb.Bold(utils.CleanHTML(report.PluginName)))

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

	fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Vulnerability:"), verb.Yellow(utils.CleanHTML(infoName)))
	fmt.Fprintf(&sb, "%s %s\n", verb.Gray("UUID:"), verb.Cyan(report.Vulnerability.Uuid))
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
	for _, site := range report.Sites {
		siteName, _ := getSiteName(site.SiteID)
		fmt.Fprintf(&sb, "  %s %s\n", verb.Green("→"), fmt.Sprintf("%s %s", siteName, verb.Gray("("+site.Version+")")))
	}

	return sb.String(), nil
}

// formatSiteReport renders a site-centric summary showing each vulnerable plugin,
// number of matched vulnerabilities, and the plugin's highest CVSS at that site.
// If detailed is true, it also lists individual vulnerability names.
func formatSiteReport(siteTitle string, plugins map[string]*models.VulnPlugin, detailed bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", verb.Bold(siteTitle))

	// Extract keys and sort them for consistent output.
	keys := make([]string, 0, len(plugins))
	for pluginName := range plugins {
		keys = append(keys, pluginName)
	}
	sort.Strings(keys)

	for _, pluginSlug := range keys {
		info := plugins[pluginSlug]
		displayName := info.PluginName
		if displayName == "" {
			displayName = pluginSlug
		}
		fmt.Fprintf(&sb, "  %s %s\n", verb.Yellow(utils.CleanHTML(displayName)), verb.Gray("("+info.Version+")"))

		if detailed {
			for _, v := range info.Vulnerability {
				vName := v.Name
				for _, source := range v.Source {
					if !strings.HasPrefix(source.Name, "CVE") {
						vName = source.Name
						break
					}
				}
				fmt.Fprintf(&sb, "    %s %s\n", verb.Gray("-"), utils.CleanHTML(vName))
			}
		} else {
			fmt.Fprintf(&sb, "    %s %d\n", verb.Gray("Vulnerabilities:"), len(info.Vulnerability))
		}

		if info.Cvss != nil {
			fmt.Fprintf(&sb, "    %s %s\n", verb.Gray("Highest CVSS:"), colorCvss(*info.Cvss))
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

// colorCvss returns the CVSS score formatted and colored by severity:
// red for high (≥7.0), yellow for medium (≥4.0), green for low (<4.0).
func colorCvss(cvss float64) string {
	s := fmt.Sprintf("%.1f", cvss)
	switch {
	case cvss >= 7.0:
		return verb.Red(s)
	case cvss >= 4.0:
		return verb.Yellow(s)
	default:
		return verb.Green(s)
	}
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
