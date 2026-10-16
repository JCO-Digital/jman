package monitor

import (
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// systemUser is the created_by/updated_by name for checks the monitor
// manages itself.
const systemUser = "system"

// Target is a monitor check ready to run.
type Target struct {
	CheckID string
	Type    string // models.MonitorCheckTypeHTTP or MonitorCheckTypeHeartbeat
	Domain  string // the checked host; a heartbeat check's label
	Probe   HTTPProbe
	// Heartbeat configures a heartbeat check; zero for HTTP checks.
	Heartbeat HeartbeatSettings
	Label     string // name used in alerts: the check's name, else host plus path
}

// monitoredSite is a site that has (or should have) a default check.
type monitoredSite struct {
	domain string
	// active sites have their checks run; inactive ones keep them but skip.
	active bool
}

// LoadTargets reconciles default checks with the current site list and
// returns every check the monitor should run now.
func LoadTargets() ([]Target, error) {
	sites, spinupListed, err := loadMonitoredSites()
	if err != nil {
		return nil, err
	}
	checks, err := db.ListMonitorChecks()
	if err != nil {
		return nil, err
	}

	checks = reconcileDefaultChecks(checks, sites, spinupListed)

	secrets, err := db.GetMonitorCheckSecretValues()
	if err != nil {
		return nil, err
	}

	targets := make([]Target, 0, len(checks))
	for _, c := range checks {
		if t, ok := targetFor(c, sites, secrets[c.ID]); ok {
			targets = append(targets, t)
		}
	}
	return targets, nil
}

// loadMonitoredSites returns every site that gets a default check, keyed
// by site UUID: the SpinupWP sites in the cached list (all of them run, as
// SpinupWP decides which exist) and the other managed sites (run while
// active). spinupListed is false when the cached SpinupWP list is empty,
// which more likely means a failed refresh than no sites at all.
func loadMonitoredSites() (map[string]monitoredSite, bool, error) {
	spinupSites, err := cache.GetCachedSites()
	if err != nil {
		return nil, false, err
	}
	managed, err := db.ListManagedSites()
	if err != nil {
		return nil, false, err
	}

	sites := make(map[string]monitoredSite, len(spinupSites)+len(managed))
	for _, s := range spinupSites {
		sites[utils.SpinupWPSiteUUID(s.ID)] = monitoredSite{domain: s.Domain, active: true}
	}
	for _, ms := range managed {
		// SyncSpinupWPIntoInventory only upserts, so inventory can still hold
		// SpinupWP sites since deleted there: the cached list decides.
		if ms.Provider == "spinupwp" {
			continue
		}
		sites[ms.ID] = monitoredSite{domain: ms.Domain, active: ms.Status == "active"}
	}

	return sites, len(spinupSites) > 0, nil
}

// reconcileDefaultChecks gives every site without one a default check and
// marks checks whose site is gone as orphaned, closing out any outage (or
// clears the mark when the site is back). It never changes what users set on a check, and never
// deletes one. It returns checks with these changes applied.
//
// While spinupListed is false, checks on SpinupWP sites aren't orphaned:
// an empty list would otherwise orphan all of them at once.
func reconcileDefaultChecks(checks []models.MonitorCheck, sites map[string]monitoredSite, spinupListed bool) []models.MonitorCheck {
	hasDefault := make(map[string]bool, len(checks))
	var spinupSiteIDs map[string]bool
	if !spinupListed {
		spinupSiteIDs = inventorySpinupSiteIDs()
	}

	now := time.Now().UTC()
	for i := range checks {
		c := &checks[i]
		if c.SiteID == "" {
			continue
		}
		if c.IsDefault {
			hasDefault[c.SiteID] = true
		}

		_, exists := sites[c.SiteID]
		switch {
		case exists && c.OrphanedAt != nil:
			if err := db.SetMonitorCheckOrphaned(c.ID, nil); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to restore orphaned check %s: %v\n", c.ID, err)
				continue
			}
			c.OrphanedAt = nil
		case !exists && c.OrphanedAt == nil && !spinupSiteIDs[c.SiteID]:
			if err := db.SetMonitorCheckOrphaned(c.ID, &now); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to orphan check %s: %v\n", c.ID, err)
				continue
			}
			orphanedAt := now
			c.OrphanedAt = &orphanedAt
			verb.LogPrintf(verb.Verbose, "Check %s orphaned: site %s is gone\n", c.ID, c.SiteID)
			if err := CloseOutCheck(c.ID, c.Name, StopOrphaned, systemUser, ""); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to close out orphaned check %s: %v\n", c.ID, err)
			}
		}
	}

	for siteID, site := range sites {
		if hasDefault[siteID] {
			continue
		}
		c := models.MonitorCheck{SiteID: siteID, IsDefault: true, Enabled: true}
		if err := db.CreateMonitorCheck(&c, systemUser); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to create default check for %s: %v\n", site.domain, err)
			continue
		}
		verb.LogPrintf(verb.Verbose, "Created default check for %s\n", site.domain)
		checks = append(checks, c)
	}

	return checks
}

// inventorySpinupSiteIDs returns the UUIDs of the SpinupWP sites in
// inventory.db.
func inventorySpinupSiteIDs() map[string]bool {
	ids := map[string]bool{}
	managed, err := db.ListManagedSites("spinupwp")
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to list SpinupWP sites: %v\n", err)
		return ids
	}
	for _, ms := range managed {
		ids[ms.ID] = true
	}
	return ids
}

// targetFor returns the target for a check, or false if the check
// shouldn't run now: it's paused, orphaned, its site is inactive, or it
// has no host to check. headers are the check's secret header values.
func targetFor(c models.MonitorCheck, sites map[string]monitoredSite, headers map[string]string) (Target, bool) {
	if !c.Enabled || c.OrphanedAt != nil {
		return Target{}, false
	}
	switch c.Type {
	case models.MonitorCheckTypeHTTP:
	case models.MonitorCheckTypeHeartbeat:
		if c.SiteID != "" {
			if site, ok := sites[c.SiteID]; !ok || !site.active {
				return Target{}, false
			}
		}
		return heartbeatTarget(c), true
	default:
		return Target{}, false
	}

	host := c.Host
	if c.SiteID != "" {
		site, ok := sites[c.SiteID]
		if !ok || !site.active {
			return Target{}, false
		}
		if c.IsDefault {
			host = site.domain
		}
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return Target{}, false
	}

	return Target{
		CheckID: c.ID,
		Type:    models.MonitorCheckTypeHTTP,
		Domain:  host,
		Probe:   NewHTTPProbe(c, host, headers),
		Label:   CheckLabel(c, host),
	}, true
}

// CheckLabel names a check in messages: its name, else the host it checks
// plus its path when that isn't "/".
func CheckLabel(c models.MonitorCheck, host string) string {
	if c.Name != "" {
		return c.Name
	}
	if c.Type == models.MonitorCheckTypeHeartbeat {
		return "check-in " + c.ID
	}
	if c.Path != "" && c.Path != "/" {
		return host + c.Path
	}
	return host
}
