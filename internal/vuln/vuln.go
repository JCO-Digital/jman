package vuln

import (
	"fmt"
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
	Mode string // "list" or "sites"
	// Slack sends matching reports via internal/slack.SendMessage. Its
	// per-message dedup (backed by the api.db slack_messages table) is only
	// available when the calling process has api.db open — jman-api does,
	// but the standalone `jman vuln --slack` CLI command only opens
	// inventory.db, so each manual run re-sends every matching report with
	// no cross-run dedup. That's an accepted tradeoff for an explicit,
	// operator-triggered flag, not a bug: slack.SendMessage degrades
	// gracefully (send-without-dedup) rather than failing when no database
	// connection is available.
	Slack         bool
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
	matcher, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to load ignore entries: %v\n", err)
	}

	if opts.Mode == "sites" {
		return scanSites(opts, matcher)
	}
	return scanReports(opts, matcher)
}

// scanSites builds a site-indexed vulnerability view, applies configured thresholds,
// prints matching site summaries, and optionally sends them to Slack.
//
// A site is reported when either:
//   - its highest plugin CVSS meets or exceeds configured threshold, or
//   - its total vulnerability count meets or exceeds config.Cfg.VulnThreshold.
func scanSites(opts ScanOptions, matcher *db.VulnIgnoreMatcher) error {
	sitesMap, err := buildSiteList(matcher)
	if err != nil {
		return err
	}

	// Fetch cached sites to get ServerIDs and check ignores
	cachedSites, err := cache.GetCachedSites()
	siteMeta := make(map[int]models.Site)
	if err == nil {
		for _, s := range cachedSites {
			siteMeta[s.ID] = s
		}
	} else {
		verb.LogPrintf(verb.Normal, "Warning: failed to fetch site cache, site names and server-level ignores may be missing: %v\n", err)
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
		s, ok := siteMeta[siteID]

		// Check if site is ignored for vulnerabilities
		var ignored bool
		if matcher != nil {
			serverID := 0
			if ok {
				serverID = s.ServerID
			}
			ignored = matcher.IsSiteIgnored(siteID, serverID)
		}

		if ignored {
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
		displayName := fmt.Sprintf("Site ID: %d", siteID)
		if ok {
			displayName = s.Domain
		}
		message := formatSiteReport(fmt.Sprintf("%s (%d Vulnerabilities)", displayName, totalVulns), plugins, false)
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
func scanReports(opts ScanOptions, matcher *db.VulnIgnoreMatcher) error {
	reports, err := ProcessVulnerabilities(matcher)
	if err != nil {
		return err
	}

	for _, report := range reports {
		if report.Suppressed {
			continue
		}

		if opts.SiteSearch != "" {
			// Filter vulnerabilities and their sites to those matching the search term.
			var activeVulns []models.Vulnerability
			for _, v := range report.Vulnerabilities {
				var matchedSites []models.PluginSite
				for _, site := range v.Sites {
					if site.Suppressed {
						continue
					}
					siteName, err := getSiteName(site.SiteID)
					if err != nil {
						continue
					}
					if strings.Contains(strings.ToLower(siteName), strings.ToLower(opts.SiteSearch)) {
						matchedSites = append(matchedSites, site)
					}
				}
				if len(matchedSites) > 0 {
					v.Sites = matchedSites
					activeVulns = append(activeVulns, v)
				}
			}
			if len(activeVulns) == 0 {
				continue
			}
			report.Vulnerabilities = activeVulns
		} else {
			// Filter out suppressed vulnerabilities and sites, and apply CVSS threshold filter.
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

			// Recompute maxCvss from filtered vulnerabilities before applying threshold.
			maxCvss := getCvss(report)
			if opts.CVSSThreshold > 0 && maxCvss < opts.CVSSThreshold {
				continue
			}
		}

		// Recompute final maxCvss for Slack force flag after all filtering is complete.
		finalMaxCvss := getCvss(report)
		message, err := formatReport(report)
		if err != nil {
			// Skip malformed/unformattable entries without stopping the whole scan.
			continue
		}

		fmt.Println(message)

		if opts.Slack {
			force := finalMaxCvss >= config.Cfg.CVSSThreshold
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
func buildSiteList(matcher *db.VulnIgnoreMatcher) (map[int]map[string]*models.VulnPlugin, error) {
	sitesMap := make(map[int]map[string]*models.VulnPlugin)

	reports, err := ProcessVulnerabilities(matcher)
	if err != nil {
		return nil, err
	}

	for _, report := range reports {
		if report.Suppressed {
			continue
		}

		for _, v := range report.Vulnerabilities {
			if v.Suppressed {
				continue
			}
			cvss := getVulnCvss(v)

			for _, site := range v.Sites {
				if site.Suppressed {
					continue
				}
				currentSite, ok := sitesMap[site.SiteID]
				if !ok {
					currentSite = make(map[string]*models.VulnPlugin)
					sitesMap[site.SiteID] = currentSite
				}

				currentPlugin, ok := currentSite[report.Slug]
				if !ok {
					score := cvss
					currentPlugin = &models.VulnPlugin{
						PluginName:    report.PluginName,
						Version:       site.Version,
						Cvss:          &score,
						Vulnerability: []models.Vulnerability{},
					}
					currentSite[report.Slug] = currentPlugin
				}

				// Keep the maximum CVSS observed for this plugin on this site.
				if currentPlugin.Cvss == nil || cvss > *currentPlugin.Cvss {
					score := cvss
					currentPlugin.Cvss = &score
				}

				currentPlugin.Vulnerability = append(currentPlugin.Vulnerability, v)
			}
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
func GetVulnerabilityReportsForPlugin(pluginName string, sites []models.PluginSite, matcher *db.VulnIgnoreMatcher) *models.VulnReport {
	vulnResponse, err := cache.GetCachedVulnerabilities(pluginName)
	if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
		// Missing or invalid vulnerability cache for this plugin; skip it.
		return nil
	}

	// Load site metadata for server ID lookups
	cliSites, err := cache.GetFastSiteList()
	siteMeta := make(map[int]models.CliSite)
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
				serverID := 0
				if s, ok := siteMeta[site.SiteID]; ok {
					serverID = s.ServerID
				}
				site.Suppressed = matcher.IsSiteIgnored(site.SiteID, serverID)
			}

			if IsVersionAffected(site.Version, v.Operator) {
				v.Sites = append(v.Sites, site)
				if !site.Suppressed {
					allSitesSuppressed = false
				}
			}
		}

		// Keep only vulnerabilities that affect at least one known site.
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

// formatReport renders a single vulnerability-centric report as plain text.
//
// It prefers enriched metadata from vulnerability sources (name/description/date/link),
// then falls back to base vulnerability fields where needed.
// User-facing text is HTML-cleaned before display.
func formatReport(report models.VulnReport) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Plugin:"), verb.Bold(utils.CleanHTML(report.PluginName)))

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

		// Pull the first useful values from source metadata.
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
			fmt.Fprintf(&sb, "  %s %s\n", verb.Green("→"), fmt.Sprintf("%s %s", siteName, verb.Gray("("+site.Version+")")))
		}
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

// getCvss extracts the maximum CVSS numeric score across all vulnerabilities in a report.
func getCvss(report models.VulnReport) float64 {
	var maxCvss float64
	for _, v := range report.Vulnerabilities {
		score := getVulnCvss(v)
		if score > maxCvss {
			maxCvss = score
		}
	}
	return maxCvss
}

// getVulnCvss extracts and parses a CVSS numeric score from a vulnerability.
// Returns 0 when no score is available or parsing is not possible.
func getVulnCvss(v models.Vulnerability) float64 {
	if v.Impact != nil && v.Impact.Cvss != nil {
		var score float64
		fmt.Sscanf(v.Impact.Cvss.Score, "%f", &score)
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
