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
)

// newVulnAlert is one vulnerability, and the non-ignored sites it is
// installed on, that is a candidate for a one-off new-vulnerability alert.
type newVulnAlert struct {
	Vuln       models.Vulnerability
	PluginSlug string
	PluginName string
	Cvss       float64
	Sites      []alertSite
}

type alertSite struct {
	Name    string
	Version string
}

// AlertNewVulnerabilities sends a one-off Slack alert for each vulnerability
// at or above config.Cfg.CVSSThreshold that hasn't been alerted before,
// listing the sites it's installed on. Sent alerts are tracked in the
// api.db vuln_alerts table, so each vulnerability is alerted only once.
//
// On the very first run every current vulnerability is recorded without
// sending, so enabling alerting doesn't flood the channel with the backlog
// (which the daily per-site report already covers).
func AlertNewVulnerabilities() error {
	matcher, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to load ignore entries: %v\n", err)
	}

	reports, err := ProcessVulnerabilities(matcher)
	if err != nil {
		return err
	}

	siteMeta := make(map[string]models.CliSite)
	if sites, err := cache.GetSiteList(); err == nil {
		for _, s := range sites {
			siteMeta[s.ID] = s
		}
	} else {
		verb.LogPrintf(verb.Normal, "Warning: failed to fetch site cache, site names and server-level ignores may be missing: %v\n", err)
	}

	var isSiteIgnored func(siteID, serverID string) bool
	if matcher != nil {
		isSiteIgnored = matcher.IsSiteIgnored
	}
	candidates := collectNewVulnAlerts(reports, siteMeta, isSiteIgnored, config.Cfg.CVSSThreshold)

	seeded, err := db.GetSetting(db.SystemSettingsUserID, db.VulnAlertsSeededSettingKey)
	if err != nil {
		return err
	}
	if seeded == nil {
		for _, a := range candidates {
			if err := db.RecordVulnAlert(a.Vuln.Uuid, a.PluginSlug, a.Cvss); err != nil {
				return fmt.Errorf("failed to seed vuln alert %s: %w", a.Vuln.Uuid, err)
			}
		}
		if _, err := db.SaveSetting(db.SystemSettingsUserID, db.VulnAlertsSeededSettingKey, true); err != nil {
			return err
		}
		verb.LogPrintf(verb.Normal, "Seeded %d existing vulnerabilities; new-vulnerability alerts start from the next scan\n", len(candidates))
		return nil
	}

	alerted, err := db.GetAlertedVulnUUIDs()
	if err != nil {
		return err
	}

	for _, a := range candidates {
		if alerted[a.Vuln.Uuid] {
			continue
		}
		message := formatNewVulnAlert(a)
		fmt.Println(message)
		// force: vuln_alerts is the dedup here, not the message hash.
		if err := slack.SendMessage(message, true); err != nil {
			verb.LogPrintf(verb.Normal, "Warning: failed to send new-vulnerability alert for %s: %v\n", a.Vuln.Uuid, err)
			continue // retried on the next scan
		}
		if err := db.RecordVulnAlert(a.Vuln.Uuid, a.PluginSlug, a.Cvss); err != nil {
			verb.LogPrintf(verb.Normal, "Warning: failed to record vuln alert %s: %v\n", a.Vuln.Uuid, err)
		}
	}
	return nil
}

// collectNewVulnAlerts groups vulnerability reports into per-vulnerability
// alert candidates, dropping suppressed reports/vulnerabilities/sites, sites
// ignored by isSiteIgnored (may be nil), and vulnerabilities below
// cvssThreshold or with no remaining sites. Results are ordered by plugin
// slug, then vulnerability UUID.
func collectNewVulnAlerts(reports []models.VulnReport, siteMeta map[string]models.CliSite, isSiteIgnored func(siteID, serverID string) bool, cvssThreshold float64) []newVulnAlert {
	var alerts []newVulnAlert
	for _, report := range reports {
		if report.Suppressed {
			continue
		}
		for _, v := range report.Vulnerabilities {
			if v.Suppressed || v.Uuid == "" {
				continue
			}
			cvss := getVulnCvss(v)
			if cvss < cvssThreshold {
				continue
			}

			var sites []alertSite
			for _, site := range v.Sites {
				if site.Suppressed {
					continue
				}
				meta, ok := siteMeta[site.SiteID]
				if isSiteIgnored != nil && isSiteIgnored(site.SiteID, meta.ServerID) {
					continue
				}
				name := fmt.Sprintf("Site ID: %s", site.SiteID)
				if ok {
					name = meta.Name
				}
				sites = append(sites, alertSite{Name: name, Version: site.Version})
			}
			if len(sites) == 0 {
				continue
			}
			sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })

			alerts = append(alerts, newVulnAlert{
				Vuln:       v,
				PluginSlug: report.Slug,
				PluginName: report.PluginName,
				Cvss:       cvss,
				Sites:      sites,
			})
		}
	}
	sort.SliceStable(alerts, func(i, j int) bool {
		if alerts[i].PluginSlug != alerts[j].PluginSlug {
			return alerts[i].PluginSlug < alerts[j].PluginSlug
		}
		return alerts[i].Vuln.Uuid < alerts[j].Vuln.Uuid
	})
	return alerts
}

// formatNewVulnAlert renders a vulnerability-centric alert: the vulnerability,
// its plugin and CVSS, and the sites it is installed on.
func formatNewVulnAlert(a newVulnAlert) string {
	// Prefer the first non-CVE source name, as formatReport does.
	vName, named := a.Vuln.Name, false
	link := ""
	for _, source := range a.Vuln.Source {
		if !named && !strings.HasPrefix(source.Name, "CVE") {
			vName, named = source.Name, true
		}
		if link == "" && source.Link != "" {
			link = source.Link
		}
	}
	pluginName := a.PluginName
	if pluginName == "" {
		pluginName = a.PluginSlug
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", verb.Bold("New vulnerability:"), verb.Yellow(utils.CleanHTML(vName)))
	fmt.Fprintf(&sb, "  %s %s %s\n", verb.Gray("Plugin:"), utils.CleanHTML(pluginName), verb.Gray("("+a.PluginSlug+")"))
	fmt.Fprintf(&sb, "  %s %s\n", verb.Gray("CVSS:"), colorCvss(a.Cvss))
	if link != "" {
		fmt.Fprintf(&sb, "  %s %s\n", verb.Gray("Link:"), verb.Cyan(link))
	}
	siteWord := "sites"
	if len(a.Sites) == 1 {
		siteWord = "site"
	}
	fmt.Fprintf(&sb, "  %s\n", verb.Bold(fmt.Sprintf("Installed on %d %s:", len(a.Sites), siteWord)))
	for _, s := range a.Sites {
		fmt.Fprintf(&sb, "    %s %s %s\n", verb.Gray("-"), s.Name, verb.Gray("("+s.Version+")"))
	}
	return sb.String()
}
