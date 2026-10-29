package vuln

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/vulnview"
)

// liftedMuteWindow is how far back a mute lifted for a new vulnerability
// marks its site in the new-vulnerability alert. The alert runs right after
// the sync that lifts it, so this only needs to cover one tick.
const liftedMuteWindow = time.Hour

// newVulnAlert is one vulnerability that hasn't been alerted before, and
// the sites it is on.
type newVulnAlert struct {
	Vuln       models.Vulnerability
	Kind       string
	PluginSlug string
	PluginName string
	Cvss       float64
	Sites      []alertSite
	Records    []db.VulnAlertSite
}

type alertSite struct {
	Name    string
	Version string
	// MuteLifted marks a site whose mute this vulnerability lifted.
	MuteLifted bool
}

// newSitesAlert is a plugin (or core version) whose already alerted
// vulnerabilities newly apply to some sites: it was installed or
// downgraded there.
type newSitesAlert struct {
	Kind       string
	PluginSlug string
	PluginName string
	Sites      []newSite
	Records    []db.VulnAlertSite
}

type newSite struct {
	Name     string
	Version  string
	Vulns    int
	Cvss     float64
	FixState string
	FixedIn  string
}

type vulnAlertPlan struct {
	New      []newVulnAlert
	NewSites []newSitesAlert
	// Silent are recorded without an alert: below cvssThreshold.
	Silent []db.VulnAlertSite
}

// AlertNewVulnerabilities sends the one-off Slack alerts for the open
// vulnerability exposures (see SyncVulnExposures, which runs first):
//
//   - a vulnerability never alerted before, at or above cvssThreshold,
//     listing its sites; one that lifted a mute is alerted whatever its
//     score, and the site is marked;
//   - an alerted vulnerability on a site it wasn't on before, at or above
//     cvssThreshold, one message per plugin (or core version) listing the
//     newly affected sites.
//
// Sent alerts are tracked per vulnerability in vuln_alerts and per site in
// vuln_alert_sites. A site's records are forgotten once it is no longer
// affected (past the reopen window), so a reinstall or downgrade is
// alerted again. On the very first run every current exposure is recorded
// without sending, so enabling this doesn't flood the channel.
func AlertNewVulnerabilities() error {
	exposures, err := db.ListOpenVulnExposures()
	if err != nil {
		return err
	}

	seeded, err := db.GetSetting(db.SystemSettingsUserID, db.VulnAlertSitesSeededSettingKey)
	if err != nil {
		return err
	}
	if seeded == nil {
		return seedVulnAlerts(exposures)
	}

	alertedUUIDs, err := db.GetAlertedVulnUUIDs()
	if err != nil {
		return err
	}
	alertedSites, err := db.ListVulnAlertSites()
	if err != nil {
		return err
	}
	lifted := map[exposureKey]bool{}
	if mutes, err := db.ListVulnMutesLiftedSince(time.Now().Add(-liftedMuteWindow)); err == nil {
		for _, m := range mutes {
			if m.LiftedReason == MuteLiftNewVuln {
				lifted[exposureKey{m.Kind, m.Slug, m.SiteID}] = true
			}
		}
	} else {
		verb.LogPrintf(verb.Normal, "Warning: failed to load lifted mutes: %v\n", err)
	}

	plan := planVulnAlerts(exposures, alertedUUIDs, alertedSites, lifted, siteNames(),
		loadGroupDetails, config.Cfg.CVSSThreshold)

	for _, a := range plan.New {
		message := formatNewVulnAlert(a)
		fmt.Println(message)
		// force: the alert tables are the dedup here, not the message hash.
		if err := slack.SendMessage(message, true); err != nil {
			verb.LogPrintf(verb.Normal, "Warning: failed to send new-vulnerability alert for %s: %v\n", a.Vuln.Uuid, err)
			continue // retried on the next scan
		}
		if err := db.RecordVulnAlert(a.Vuln.Uuid, a.PluginSlug, a.Cvss); err != nil {
			verb.LogPrintf(verb.Normal, "Warning: failed to record vuln alert %s: %v\n", a.Vuln.Uuid, err)
		}
		if err := db.RecordVulnAlertSites(a.Records); err != nil {
			verb.LogPrintf(verb.Normal, "Warning: %v\n", err)
		}
	}
	for _, a := range plan.NewSites {
		message := formatNewSitesAlert(a)
		fmt.Println(message)
		if err := slack.SendMessage(message, true); err != nil {
			verb.LogPrintf(verb.Normal, "Warning: failed to send newly-affected alert for %s: %v\n", a.PluginSlug, err)
			continue
		}
		if err := db.RecordVulnAlertSites(a.Records); err != nil {
			verb.LogPrintf(verb.Normal, "Warning: %v\n", err)
		}
	}
	if err := db.RecordVulnAlertSites(plan.Silent); err != nil {
		return err
	}
	return forgetResolvedAlertSites(exposures, alertedSites)
}

// seedVulnAlerts records every current exposure's vulnerabilities as
// alerted, per vulnerability and per site, without sending anything.
func seedVulnAlerts(exposures []models.VulnExposure) error {
	var records []db.VulnAlertSite
	for _, e := range exposures {
		for _, u := range e.VulnUUIDs {
			records = append(records, db.VulnAlertSite{Kind: e.Kind, Slug: e.Slug, SiteID: e.SiteID, UUID: u})
			cvss := 0.0
			if e.MaxCvss != nil {
				cvss = *e.MaxCvss
			}
			if err := db.RecordVulnAlert(u, e.Slug, cvss); err != nil {
				return fmt.Errorf("failed to seed vuln alert %s: %w", u, err)
			}
		}
	}
	if err := db.RecordVulnAlertSites(records); err != nil {
		return err
	}
	if _, err := db.SaveSetting(db.SystemSettingsUserID, db.VulnAlertSitesSeededSettingKey, true); err != nil {
		return err
	}
	if _, err := db.SaveSetting(db.SystemSettingsUserID, db.VulnAlertsSeededSettingKey, true); err != nil {
		return err
	}
	verb.LogPrintf(verb.Normal, "Seeded %d existing per-site vulnerability alerts; alerts start from the next scan\n", len(records))
	return nil
}

// forgetResolvedAlertSites drops the per-site alert records of plugins (or
// core) a site is no longer affected by, keeping those resolved within the
// reopen window in case they come back.
func forgetResolvedAlertSites(open []models.VulnExposure, recorded map[db.VulnAlertSite]bool) error {
	keep := map[exposureKey]bool{}
	for _, e := range open {
		keep[keyOf(e)] = true
	}
	recent, err := db.ListVulnExposuresResolvedSince(time.Now().Add(-exposureReopenWindow))
	if err != nil {
		return err
	}
	for _, e := range recent {
		keep[keyOf(e)] = true
	}
	forgotten := map[exposureKey]bool{}
	for r := range recorded {
		k := exposureKey{r.Kind, r.Slug, r.SiteID}
		if keep[k] || forgotten[k] {
			continue
		}
		if err := db.DeleteVulnAlertSites(r.Kind, r.Slug, r.SiteID); err != nil {
			return err
		}
		forgotten[k] = true
	}
	return nil
}

// planVulnAlerts sorts every vulnerability on every open exposure that
// hasn't been alerted for its site into a new-vulnerability alert, a
// newly-affected-sites alert or a silent record; see
// AlertNewVulnerabilities. muteLifted marks the plugins (or core) and sites
// whose mute a new vulnerability just lifted.
func planVulnAlerts(exposures []models.VulnExposure, alertedUUIDs map[string]bool,
	alertedSites map[db.VulnAlertSite]bool, muteLifted map[exposureKey]bool, siteNames map[string]string,
	details func(kind, slug, coreVersion string) groupDetails, cvssThreshold float64) vulnAlertPlan {
	var plan vulnAlertPlan

	sorted := append([]models.VulnExposure(nil), exposures...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Kind != b.Kind {
			return a.Kind > b.Kind // plugins before core
		}
		if a.Slug != b.Slug {
			return a.Slug < b.Slug
		}
		// Core alerts are per version; plugin alerts list their sites by name.
		if a.Kind == models.VulnExposureCore && a.Version != b.Version {
			return a.Version < b.Version
		}
		return siteName(siteNames, a.SiteID) < siteName(siteNames, b.SiteID)
	})

	type groupKey struct{ Kind, Slug, Version string }
	keyFor := func(e models.VulnExposure) groupKey {
		k := groupKey{Kind: e.Kind, Slug: e.Slug}
		if e.Kind == models.VulnExposureCore {
			k.Version = e.Version
		}
		return k
	}
	detailCache := map[groupKey]groupDetails{}
	detailsOf := func(e models.VulnExposure) groupDetails {
		k := keyFor(e)
		d, ok := detailCache[k]
		if !ok {
			d = details(k.Kind, k.Slug, k.Version)
			detailCache[k] = d
		}
		return d
	}

	newByVuln := map[string]*newVulnAlert{}
	var newOrder []string
	newSitesByGroup := map[groupKey]*newSitesAlert{}
	var newSitesOrder []groupKey

	for _, e := range sorted {
		d := detailsOf(e)
		name := siteName(siteNames, e.SiteID)
		var known []db.VulnAlertSite
		var knownCvss *float64
		for _, u := range e.VulnUUIDs {
			rec := db.VulnAlertSite{Kind: e.Kind, Slug: e.Slug, SiteID: e.SiteID, UUID: u}
			if alertedSites[rec] {
				continue
			}
			if alertedUUIDs[u] {
				known = append(known, rec)
				if score, ok := cvssScore(d.Vulns[u]); ok && (knownCvss == nil || score > *knownCvss) {
					knownCvss = &score
				}
				continue
			}
			id := e.Kind + "\x00" + e.Slug + "\x00" + u
			a, ok := newByVuln[id]
			if !ok {
				v, found := d.Vulns[u]
				if !found {
					v = models.Vulnerability{Uuid: u, Name: u}
				}
				score, _ := cvssScore(v)
				a = &newVulnAlert{Vuln: v, Kind: e.Kind, PluginSlug: e.Slug, PluginName: d.Name, Cvss: score}
				newByVuln[id] = a
				newOrder = append(newOrder, id)
			}
			a.Sites = append(a.Sites, alertSite{Name: name, Version: e.Version, MuteLifted: muteLifted[keyOf(e)]})
			a.Records = append(a.Records, rec)
		}

		if len(known) == 0 {
			continue
		}
		if knownCvss == nil || *knownCvss < cvssThreshold {
			plan.Silent = append(plan.Silent, known...)
			continue
		}
		k := keyFor(e)
		g, ok := newSitesByGroup[k]
		if !ok {
			g = &newSitesAlert{Kind: e.Kind, PluginSlug: e.Slug, PluginName: d.Name}
			newSitesByGroup[k] = g
			newSitesOrder = append(newSitesOrder, k)
		}
		g.Sites = append(g.Sites, newSite{Name: name, Version: e.Version, Vulns: len(known), Cvss: *knownCvss,
			FixState: e.FixState, FixedIn: e.FixedIn})
		g.Records = append(g.Records, known...)
	}

	for _, id := range newOrder {
		a := newByVuln[id]
		liftedMute := false
		for _, s := range a.Sites {
			liftedMute = liftedMute || s.MuteLifted
		}
		if a.Cvss >= cvssThreshold || liftedMute {
			plan.New = append(plan.New, *a)
		} else {
			plan.Silent = append(plan.Silent, a.Records...)
		}
	}
	for _, k := range newSitesOrder {
		plan.NewSites = append(plan.NewSites, *newSitesByGroup[k])
	}
	return plan
}

func siteName(names map[string]string, siteID string) string {
	if n := names[siteID]; n != "" {
		return n
	}
	return "Site ID: " + siteID
}

// alertSubject is the "Plugin: Name (slug)" or "Core: WordPress x.y" line.
func alertSubject(kind, slug, name string) string {
	if kind == models.VulnExposureCore {
		return fmt.Sprintf("%s %s", verb.Gray("Core:"), utils.CleanHTML(name))
	}
	if name == "" {
		name = slug
	}
	return fmt.Sprintf("%s %s %s", verb.Gray("Plugin:"), utils.CleanHTML(name), verb.Gray("("+slug+")"))
}

// formatNewVulnAlert renders a vulnerability-centric alert: the vulnerability,
// its plugin (or core version) and CVSS, and the sites it is installed on.
func formatNewVulnAlert(a newVulnAlert) string {
	vName, link := vulnDisplayName(a.Vuln)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", verb.Bold("New vulnerability:"), verb.Yellow(utils.CleanHTML(vName)))
	fmt.Fprintf(&sb, "  %s\n", alertSubject(a.Kind, a.PluginSlug, a.PluginName))
	fmt.Fprintf(&sb, "  %s %s\n", verb.Gray("CVSS:"), vulnview.ColorCvss(a.Cvss))
	if link != "" {
		fmt.Fprintf(&sb, "  %s %s\n", verb.Gray("Link:"), verb.Cyan(link))
	}
	fmt.Fprintf(&sb, "  %s\n", verb.Bold(fmt.Sprintf("Installed on %s:", plural(len(a.Sites), "site"))))
	for _, s := range a.Sites {
		fmt.Fprintf(&sb, "    %s %s %s", verb.Gray("-"), s.Name, verb.Gray("("+s.Version+")"))
		if s.MuteLifted {
			fmt.Fprintf(&sb, " %s", verb.Yellow("mute lifted"))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatNewSitesAlert renders the sites a plugin's (or core version's)
// known vulnerabilities newly apply to.
func formatNewSitesAlert(a newSitesAlert) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", verb.Bold("Newly affected:"),
		verb.Yellow(fmt.Sprintf("known vulnerabilities on %s", plural(len(a.Sites), "more site"))))
	fmt.Fprintf(&sb, "  %s\n", alertSubject(a.Kind, a.PluginSlug, a.PluginName))
	for _, s := range a.Sites {
		fix := "no fix"
		if s.FixState == models.VulnFixAvailable {
			fix = "update to " + s.FixedIn
		}
		fmt.Fprintf(&sb, "    %s %s %s: %s, CVSS %s, %s\n", verb.Gray("-"), s.Name, verb.Gray("("+s.Version+")"),
			plural(s.Vulns, "vulnerability"), vulnview.ColorCvss(s.Cvss), fix)
	}
	return sb.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	if strings.HasSuffix(word, "y") {
		return fmt.Sprintf("%d %sies", n, strings.TrimSuffix(word, "y"))
	}
	return fmt.Sprintf("%d %ss", n, word)
}
