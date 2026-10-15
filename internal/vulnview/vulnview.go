// Package vulnview filters and formats vulnerability reports for display:
// the `jman vuln` output and jman-api's Slack reports. It only works on
// reports it is given, so the CLI can use it on reports fetched from
// jman-api without the database.
package vulnview

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// SiteNamer returns a site's display name from its ID, or "" if unknown.
type SiteNamer func(siteID string) string

// FilterReports returns the plugin reports to show, with suppressed
// (ignored) vulnerabilities and sites left out.
//
// With a siteSearch, only vulnerabilities affecting a site whose name
// contains it are kept, their site lists are trimmed to the matching sites,
// and cvssThreshold isn't applied. Otherwise reports whose highest
// remaining CVSS is below cvssThreshold (if > 0) are dropped.
func FilterReports(reports []models.VulnReport, siteSearch string, cvssThreshold float64, siteName SiteNamer) []models.VulnReport {
	out := []models.VulnReport{}
	search := strings.ToLower(siteSearch)
	for _, report := range reports {
		if report.Suppressed {
			continue
		}

		var activeVulns []models.Vulnerability
		if search != "" {
			filterSites := func(sites []models.PluginSite) []models.PluginSite {
				var matched []models.PluginSite
				for _, site := range sites {
					if site.Suppressed {
						continue
					}
					if name := siteName(site.SiteID); name != "" && strings.Contains(strings.ToLower(name), search) {
						matched = append(matched, site)
					}
				}
				return matched
			}
			for _, v := range report.Vulnerabilities {
				if matched := filterSites(v.Sites); len(matched) > 0 {
					v.Sites = matched
					v.PatchedSites = filterSites(v.PatchedSites)
					activeVulns = append(activeVulns, v)
				}
			}
		} else {
			for _, v := range report.Vulnerabilities {
				if v.Suppressed {
					continue
				}
				if active := unsuppressed(v.Sites); len(active) > 0 {
					v.Sites = active
					activeVulns = append(activeVulns, v)
				}
			}
		}
		if len(activeVulns) == 0 {
			continue
		}
		report.Vulnerabilities = activeVulns

		if search == "" && cvssThreshold > 0 && ReportCvss(report) < cvssThreshold {
			continue
		}
		out = append(out, report)
	}
	return out
}

// FilterCoreReports returns the core reports to show, with suppressed
// vulnerabilities and sites left out, and reports whose highest CVSS is
// below cvssThreshold (if > 0) dropped.
func FilterCoreReports(reports []models.CoreVulnReport, cvssThreshold float64) []models.CoreVulnReport {
	out := []models.CoreVulnReport{}
	for _, report := range reports {
		var activeVulns []models.Vulnerability
		for _, v := range report.Vulnerabilities {
			if v.Suppressed {
				continue
			}
			if active := unsuppressed(v.Sites); len(active) > 0 {
				v.Sites = active
				activeVulns = append(activeVulns, v)
			}
		}
		if len(activeVulns) == 0 {
			continue
		}
		report.Vulnerabilities = activeVulns
		if cvssThreshold > 0 && CoreReportCvss(report) < cvssThreshold {
			continue
		}
		out = append(out, report)
	}
	return out
}

func unsuppressed(sites []models.PluginSite) []models.PluginSite {
	var out []models.PluginSite
	for _, s := range sites {
		if !s.Suppressed {
			out = append(out, s)
		}
	}
	return out
}

// SiteVulns is one site's vulnerable plugins, keyed by plugin slug.
type SiteVulns struct {
	SiteID     string
	Plugins    map[string]*models.VulnPlugin
	TotalVulns int
	MaxCvss    float64
}

// SitesOverThreshold turns plugin reports into per-site summaries and
// returns, sorted by site ID, the sites whose highest plugin CVSS is at
// least cvssThreshold or whose number of vulnerabilities is at least
// vulnThreshold. Suppressed reports, vulnerabilities and sites are left out.
func SitesOverThreshold(reports []models.VulnReport, cvssThreshold, vulnThreshold float64) []SiteVulns {
	bySite := map[string]map[string]*models.VulnPlugin{}
	for _, report := range reports {
		if report.Suppressed {
			continue
		}
		for _, v := range report.Vulnerabilities {
			if v.Suppressed {
				continue
			}
			cvss := VulnCvss(v)
			for _, site := range v.Sites {
				if site.Suppressed {
					continue
				}
				plugins, ok := bySite[site.SiteID]
				if !ok {
					plugins = map[string]*models.VulnPlugin{}
					bySite[site.SiteID] = plugins
				}
				plugin, ok := plugins[report.Slug]
				if !ok {
					score := cvss
					plugin = &models.VulnPlugin{PluginName: report.PluginName, Version: site.Version, Cvss: &score, Vulnerability: []models.Vulnerability{}}
					plugins[report.Slug] = plugin
				}
				// Keep the highest CVSS seen for this plugin on this site.
				if plugin.Cvss == nil || cvss > *plugin.Cvss {
					score := cvss
					plugin.Cvss = &score
				}
				plugin.Vulnerability = append(plugin.Vulnerability, v)
			}
		}
	}

	ids := make([]string, 0, len(bySite))
	for id := range bySite {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := []SiteVulns{}
	for _, id := range ids {
		sv := SiteVulns{SiteID: id, Plugins: bySite[id]}
		for _, p := range sv.Plugins {
			sv.TotalVulns += len(p.Vulnerability)
			if p.Cvss != nil && *p.Cvss > sv.MaxCvss {
				sv.MaxCvss = *p.Cvss
			}
		}
		if sv.MaxCvss < cvssThreshold && float64(sv.TotalVulns) < vulnThreshold {
			continue
		}
		out = append(out, sv)
	}
	return out
}

// FormatReport renders a plugin vulnerability report as plain text,
// preferring the enriched source metadata (name, description, date, link)
// over the base fields. If showPatched is true, sites excluded by a
// backport rule are listed too.
func FormatReport(report models.VulnReport, showPatched bool, siteName SiteNamer) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", verb.Gray("Plugin:"), verb.Bold(utils.CleanHTML(report.PluginName)))
	for i, v := range report.Vulnerabilities {
		if i > 0 {
			fmt.Fprintln(&sb, "---")
		}
		writeVulnDetails(&sb, v)

		fmt.Fprintf(&sb, "\n%s\n", verb.Bold("Affected Sites:"))
		for _, site := range v.Sites {
			fmt.Fprintf(&sb, "  %s %s %s\n", verb.Green("→"), siteName(site.SiteID), verb.Gray("("+site.Version+")"))
		}
		if showPatched && len(v.PatchedSites) > 0 {
			fmt.Fprintf(&sb, "\n%s\n", verb.Bold("Patched (backport):"))
			for _, site := range v.PatchedSites {
				fmt.Fprintf(&sb, "  %s %s %s\n", verb.Gray("✓"), siteName(site.SiteID), verb.Gray("("+site.Version+")"))
			}
		}
	}
	return sb.String()
}

// FormatCoreReport renders a WordPress core vulnerability report as plain
// text, like FormatReport.
func FormatCoreReport(report models.CoreVulnReport, siteName SiteNamer) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", verb.Gray("WordPress Core:"), verb.Bold(report.Version))
	for i, v := range report.Vulnerabilities {
		if i > 0 {
			fmt.Fprintln(&sb, "---")
		}
		writeVulnDetails(&sb, v)

		fmt.Fprintf(&sb, "\n%s\n", verb.Bold("Affected Sites:"))
		for _, site := range v.Sites {
			fmt.Fprintf(&sb, "  %s %s\n", verb.Green("→"), siteName(site.SiteID))
		}
	}
	return sb.String()
}

func writeVulnDetails(sb *strings.Builder, v models.Vulnerability) {
	cvss := VulnCvss(v)
	infoName, infoDesc, infoDate, infoLink := "", "", "", ""
	if v.Description != nil {
		infoDesc = *v.Description
	}
	// Pull the first useful values from the source metadata.
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

	fmt.Fprintf(sb, "%s %s\n", verb.Gray("Vulnerability:"), verb.Yellow(utils.CleanHTML(infoName)))
	fmt.Fprintf(sb, "%s %s\n", verb.Gray("UUID:"), verb.Cyan(v.Uuid))
	if infoDate != "" {
		fmt.Fprintf(sb, "%s %s\n", verb.Gray("Date:"), infoDate)
	}
	if cvss > 0 {
		fmt.Fprintf(sb, "%s %s\n", verb.Gray("CVSS Score:"), ColorCvss(cvss))
	}
	if infoDesc != "" {
		fmt.Fprintf(sb, "%s %s\n", verb.Gray("Description:"), utils.CleanHTML(infoDesc))
	}
	if infoLink != "" {
		fmt.Fprintf(sb, "%s %s\n", verb.Gray("Link:"), verb.Cyan(infoLink))
	}
}

// FormatSiteReport renders a site summary: each vulnerable plugin, its
// number of vulnerabilities (or, if detailed, their names) and its highest
// CVSS on the site.
func FormatSiteReport(siteTitle string, plugins map[string]*models.VulnPlugin, detailed bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", verb.Bold(siteTitle))

	keys := make([]string, 0, len(plugins))
	for slug := range plugins {
		keys = append(keys, slug)
	}
	sort.Strings(keys)

	for _, slug := range keys {
		info := plugins[slug]
		displayName := info.PluginName
		if displayName == "" {
			displayName = slug
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
			fmt.Fprintf(&sb, "    %s %s\n", verb.Gray("Highest CVSS:"), ColorCvss(*info.Cvss))
		}
	}
	return sb.String()
}

// ReportCvss returns the highest CVSS score in a plugin report.
func ReportCvss(report models.VulnReport) float64 {
	return maxCvss(report.Vulnerabilities)
}

// CoreReportCvss returns the highest CVSS score in a core report.
func CoreReportCvss(report models.CoreVulnReport) float64 {
	return maxCvss(report.Vulnerabilities)
}

func maxCvss(vulns []models.Vulnerability) float64 {
	var m float64
	for _, v := range vulns {
		if score := VulnCvss(v); score > m {
			m = score
		}
	}
	return m
}

// VulnCvss returns a vulnerability's CVSS score, or 0 if it has none.
func VulnCvss(v models.Vulnerability) float64 {
	if v.Impact != nil && v.Impact.Cvss != nil {
		var score float64
		fmt.Sscanf(v.Impact.Cvss.Score, "%f", &score)
		return score
	}
	return 0
}

// ColorCvss formats a CVSS score colored by severity: red for high (≥7.0),
// yellow for medium (≥4.0), green for low.
func ColorCvss(cvss float64) string {
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
