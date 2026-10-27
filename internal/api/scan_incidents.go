package api

import (
	"fmt"
	"strings"
	"sync"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/pagerduty"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// scanTriageMu serializes changes to scan findings and security incidents,
// so an agent report and a person's triage don't interleave.
var scanTriageMu sync.Mutex

// scanIncidentMinRank is the lowest severity that puts a finding in a
// security incident. Low findings are for information and only listed.
var scanIncidentMinRank = config.ScanSeverityRank("medium")

// scanStateRules are the rules that describe a file's state rather than a
// change to it, which a path acknowledgement can cover; for new-admin, the
// path names the account ("users/<login>").
var scanStateRules = map[string]bool{"stray-file": true, "foreign-owner": true, "unreadable": true, ruleNewAdmin: true}

// scanTriageBy is who status changes jman-api makes on its own are
// recorded as.
const scanTriageBy = "jman-api"

// TriagePendingScanFindings assigns every site's open findings that have
// no incident yet, without alerting. jman-api runs it on startup, so
// findings stored before security incidents existed get one.
func TriagePendingScanFindings() {
	sites, err := db.ListSitesWithPendingScanFindings()
	if err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
		return
	}
	for _, siteID := range sites {
		triageScanFindings(siteID, false)
	}
}

// triageScanFindings handles a site's pending findings: it marks the ones
// the allowlist or a path acknowledgement covers, and puts the rest that
// are at least medium in a security incident. A finding goes back to the
// closed incident it was in, or that has a finding with the same hash;
// otherwise into the site's active security incident, or a new one. With
// alert set, new findings alert by severity (see config.ScanSlackAlertRank).
func triageScanFindings(siteID string, alert bool) {
	scanTriageMu.Lock()
	defer scanTriageMu.Unlock()

	pending, err := db.ListPendingScanFindings(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
		return
	}
	added := map[int64][]models.StoredScanFinding{}
	reopened := map[int64]bool{}
	var order []int64
	for _, f := range pending {
		if entry, err := db.GetScanAllowlistEntryByHash(f.SHA256); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		} else if entry != nil {
			if _, err := db.SetScanFindingStatus([]int64{f.ID}, models.ScanFindingAllowlisted, scanTriageBy); err != nil {
				verb.LogPrintf(verb.Normal, "%v", err)
			}
			continue
		}
		if scanStateRules[f.Rule] {
			if ack, err := db.GetScanPathAck(siteID, f.Rule, f.Path); err != nil {
				verb.LogPrintf(verb.Normal, "%v", err)
				continue
			} else if ack != nil {
				if _, err := db.SetScanFindingStatus([]int64{f.ID}, models.ScanFindingAcknowledged, scanTriageBy); err != nil {
					verb.LogPrintf(verb.Normal, "%v", err)
				}
				continue
			}
		}
		if config.ScanSeverityRank(f.Severity) < scanIncidentMinRank {
			continue
		}

		incidentID, wasReopened, err := securityIncidentFor(siteID, f)
		if err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		}
		if err := db.SetScanFindingIncident(f.ID, incidentID); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		}
		if _, seen := added[incidentID]; !seen {
			order = append(order, incidentID)
		}
		added[incidentID] = append(added[incidentID], f)
		reopened[incidentID] = reopened[incidentID] || wasReopened
	}

	for _, id := range order {
		if err := db.RefreshSecurityIncident(id); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
		if alert {
			alertSecurityIncident(id, added[id], reopened[id])
		}
	}
	// A finding reported again after it was cleared isn't cleared any
	// more, which the summary of its incident should show.
	if active, err := db.GetActiveSecurityIncident(siteID); err != nil {
		verb.LogPrintf(verb.Normal, "%v", err)
	} else if active != nil && added[active.ID] == nil {
		if err := db.RefreshSecurityIncident(active.ID); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
	}
}

// securityIncidentFor returns the security incident a finding goes in, and
// whether it was reopened for it.
func securityIncidentFor(siteID string, f models.StoredScanFinding) (int64, bool, error) {
	// The finding itself, or another file with its content, was in an
	// incident that was closed: the same thing is back.
	closedID := int64(0)
	if f.IncidentID != nil {
		closedID = *f.IncidentID
	} else {
		id, err := db.FindClosedSecurityIncidentByHash(siteID, f.SHA256)
		if err != nil {
			return 0, false, err
		}
		closedID = id
	}
	if closedID != 0 {
		if err := db.ReopenIncident(closedID); err != nil {
			return 0, false, err
		}
		return closedID, true, nil
	}

	active, err := db.GetActiveSecurityIncident(siteID)
	if err != nil {
		return 0, false, err
	}
	if active != nil {
		return active.ID, false, nil
	}

	domain := siteID
	if site, err := db.GetManagedSite(siteID); err == nil && site != nil {
		domain = site.Domain
	}
	inc, err := db.CreateSecurityIncident(siteID, domain, f)
	if err != nil {
		return 0, false, err
	}
	return inc.ID, false, nil
}

// alertSecurityIncident alerts about findings added to a security incident,
// in Slack and PagerDuty by their severity.
func alertSecurityIncident(id int64, findings []models.StoredScanFinding, reopened bool) {
	slackRank, pdRank := config.ScanSlackAlertRank(), config.ScanPagerDutyAlertRank()
	worst := ""
	var listed []models.StoredScanFinding
	for _, f := range findings {
		rank := config.ScanSeverityRank(f.Severity)
		if rank > config.ScanSeverityRank(worst) {
			worst = f.Severity
		}
		if slackRank > 0 && rank >= slackRank {
			listed = append(listed, f)
		}
	}
	if len(listed) == 0 && (pdRank == 0 || config.ScanSeverityRank(worst) < pdRank) {
		return
	}
	inc, err := db.GetIncidentByID(id)
	if err != nil || inc == nil {
		verb.LogPrintf(verb.Normal, "alertSecurityIncident: incident %d: %v", id, err)
		return
	}

	if len(listed) > 0 {
		if err := slack.SendMessageToChannel(securitySlackMessage(inc, listed, reopened), securitySlackChannel(), true); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to send Slack security alert for %s: %v", inc.Domain, err)
		}
	}
	if pdRank > 0 && config.ScanSeverityRank(worst) >= pdRank && pagerduty.Enabled() && !inc.PDTriggered {
		summary := fmt.Sprintf("Security incident on %s: %s", inc.Domain, inc.ErrorMessage)
		if err := pagerduty.TriggerSecurityEvent(inc.ID, inc.Domain, summary); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to trigger PagerDuty for security incident %d: %v", inc.ID, err)
		} else if err := db.SetIncidentPDTriggered(inc.ID, true); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
	}
}

// maxSlackScanFindings caps how many findings one Slack alert lists.
const maxSlackScanFindings = 10

func securitySlackMessage(inc *models.Incident, findings []models.StoredScanFinding, reopened bool) string {
	var b strings.Builder
	what := "New findings"
	if reopened {
		what = "Reopened incident, findings back"
	}
	fmt.Fprintf(&b, "🚨 *Security* — %s on *%s*", what, inc.Domain)
	if base := strings.TrimSuffix(config.Cfg.WebURL, "/"); base != "" {
		fmt.Fprintf(&b, " (<%s/monitoring?tab=security&incident=%d|incident #%d>)", base, inc.ID, inc.ID)
	}
	for i, f := range findings {
		if i == maxSlackScanFindings {
			fmt.Fprintf(&b, "\n… and %d more", len(findings)-i)
			break
		}
		fmt.Fprintf(&b, "\n• [%s] %s `%s`", f.Severity, f.Rule, f.Path)
	}
	return b.String()
}

// securitySlackChannel is where security alerts go: slackSecurityChannel,
// or the monitor's channel.
func securitySlackChannel() string {
	for _, ch := range []string{config.Cfg.SlackSecurityChannel, config.Cfg.SlackMonitorChannel} {
		if ch != "" {
			return ch
		}
	}
	return config.Cfg.SlackChannel
}

// closeEmptySecurityIncidents closes the given security incidents that have
// no open findings left, as by.
func closeEmptySecurityIncidents(ids map[int64]bool, by string) {
	for id := range ids {
		if err := db.RefreshSecurityIncident(id); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
		n, err := db.CountOpenScanFindings(id)
		if err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		}
		if n > 0 {
			continue
		}
		inc, err := db.GetIncidentByID(id)
		if err != nil || inc == nil || (inc.Status != models.IncidentStatusOpen && inc.Status != models.IncidentStatusAcknowledged) {
			continue
		}
		if _, err := db.CloseIncident(id, by); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
			continue
		}
		resolveSecurityPagerDuty(inc)
	}
}

// resolveSecurityPagerDuty resolves a security incident's PagerDuty alert,
// if it raised one.
func resolveSecurityPagerDuty(inc *models.Incident) {
	if inc.PDTriggered && pagerduty.Enabled() {
		if err := pagerduty.ResolveSecurityEvent(inc.ID); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to resolve PagerDuty for security incident %d: %v", inc.ID, err)
		}
	}
}
