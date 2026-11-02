package vuln

import (
	"fmt"
	"sort"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// coreVulnIDsMigratedSettingKey is the system setting marking that the
// stored core vulnerability UUIDs have been moved to the per-vulnerability
// UUIDs of models.NormalizeCoreVulnerabilities.
const coreVulnIDsMigratedSettingKey = "core_vuln_ids_migrated"

// coreVersionVulns is what the core feed says about one core version.
type coreVersionVulns struct {
	// FeedUUID is the UUID the feed shares between the version's
	// vulnerabilities.
	FeedUUID string
	// IDs are the version's per-vulnerability UUIDs.
	IDs  []string
	Cvss map[string]float64
}

// coreIDMigration is the stored data to change, see planCoreVulnIDMigration.
type coreIDMigration struct {
	Mutes      []models.VulnMute
	Alerts     map[string]float64 // UUID -> CVSS
	AlertSites []db.VulnAlertSite
}

// migrateCoreVulnIDs moves the core vulnerability UUIDs stored in mutes and
// alert records from the feed's shared per-version UUID to the
// per-vulnerability ones, once. Without it, the first sync with the new
// UUIDs would lift every core mute and alert every known core
// vulnerability again.
//
// It needs the feed data of every core version with an exposure, and
// returns an error, without marking itself done, when some can't be read:
// core exposures must then be left alone until it can run.
func migrateCoreVulnIDs(now time.Time) error {
	done, err := db.GetSetting(db.SystemSettingsUserID, coreVulnIDsMigratedSettingKey)
	if err != nil || done != nil {
		return err
	}

	open, err := db.ListOpenVulnExposures()
	if err != nil {
		return err
	}
	recent, err := db.ListVulnExposuresResolvedSince(now.Add(-exposureReopenWindow))
	if err != nil {
		return err
	}
	exposures := append(open, recent...)

	versions := map[string]coreVersionVulns{}
	for _, e := range exposures {
		if e.Kind != models.VulnExposureCore {
			continue
		}
		if _, ok := versions[e.Version]; ok {
			continue
		}
		resp, err := cache.GetCachedCoreVulnerabilities(e.Version)
		if err != nil || resp == nil || resp.Error != 0 || resp.Data == nil {
			return fmt.Errorf("no vulnerability data for WordPress %s: %v", e.Version, err)
		}
		vv := coreVersionVulns{Cvss: map[string]float64{}}
		for _, v := range resp.Data.Vulnerability {
			if v.FeedUuid != "" {
				vv.FeedUUID = v.FeedUuid
			}
			vv.IDs = append(vv.IDs, v.Uuid)
			if score, ok := cvssScore(v); ok {
				vv.Cvss[v.Uuid] = score
			}
		}
		versions[e.Version] = vv
	}

	mutes, err := db.ListOpenVulnMutes()
	if err != nil {
		return err
	}
	alerted, err := db.GetAlertedVulnUUIDs()
	if err != nil {
		return err
	}
	alertSites, err := db.ListVulnAlertSites()
	if err != nil {
		return err
	}

	plan := planCoreVulnIDMigration(exposures, versions, mutes, alerted, alertSites)
	for i := range plan.Mutes {
		if err := db.UpdateVulnMute(&plan.Mutes[i]); err != nil {
			return err
		}
	}
	for uuid, cvss := range plan.Alerts {
		if err := db.RecordVulnAlert(uuid, models.CoreExposureSlug, cvss); err != nil {
			return err
		}
	}
	if err := db.RecordVulnAlertSites(plan.AlertSites); err != nil {
		return err
	}
	if _, err := db.SaveSetting(db.SystemSettingsUserID, coreVulnIDsMigratedSettingKey, true); err != nil {
		return err
	}
	verb.LogPrintf(verb.Normal, "Moved core vulnerability IDs to per-vulnerability IDs: %d mutes, %d alerts, %d per-site alerts\n",
		len(plan.Mutes), len(plan.Alerts), len(plan.AlertSites))
	return nil
}

// planCoreVulnIDMigration works out the changes migrateCoreVulnIDs makes. A
// stored core UUID is the feed's UUID of a version when the feed says so or
// when a core exposure on that version lists it; it stands for all of the
// version's vulnerabilities, and is replaced by (or, in the alert tables,
// joined by) their UUIDs. The old alert records stay: they match nothing
// once the UUIDs have changed.
func planCoreVulnIDMigration(exposures []models.VulnExposure, versions map[string]coreVersionVulns,
	mutes []models.VulnMute, alerted map[string]bool, alertSites map[db.VulnAlertSite]bool) coreIDMigration {
	current := map[string]bool{}
	for _, vv := range versions {
		for _, id := range vv.IDs {
			current[id] = true
		}
	}

	// legacy maps a feed UUID to its version.
	legacy := map[string]string{}
	for ver, vv := range versions {
		if vv.FeedUUID != "" {
			legacy[vv.FeedUUID] = ver
		}
	}
	for _, e := range exposures {
		if e.Kind != models.VulnExposureCore {
			continue
		}
		if _, ok := versions[e.Version]; !ok {
			continue
		}
		for _, u := range e.VulnUUIDs {
			if !current[u] {
				legacy[u] = e.Version
			}
		}
	}

	plan := coreIDMigration{Alerts: map[string]float64{}}

	for _, m := range mutes {
		if m.Kind != models.VulnExposureCore {
			continue
		}
		ids := map[string]bool{}
		changed := false
		for _, u := range m.VulnUUIDsAtMute {
			ver, ok := legacy[u]
			if !ok {
				ids[u] = true
				continue
			}
			changed = true
			for _, id := range versions[ver].IDs {
				ids[id] = true
			}
		}
		if !changed {
			continue
		}
		m.VulnUUIDsAtMute = make([]string, 0, len(ids))
		for id := range ids {
			m.VulnUUIDsAtMute = append(m.VulnUUIDsAtMute, id)
		}
		sort.Strings(m.VulnUUIDsAtMute)
		plan.Mutes = append(plan.Mutes, m)
	}

	for u := range alerted {
		ver, ok := legacy[u]
		if !ok {
			continue
		}
		vv := versions[ver]
		for _, id := range vv.IDs {
			if !alerted[id] {
				plan.Alerts[id] = vv.Cvss[id]
			}
		}
	}

	added := map[db.VulnAlertSite]bool{}
	for r := range alertSites {
		ver, ok := legacy[r.UUID]
		if r.Kind != models.VulnExposureCore || !ok {
			continue
		}
		for _, id := range versions[ver].IDs {
			rec := db.VulnAlertSite{Kind: r.Kind, Slug: r.Slug, SiteID: r.SiteID, UUID: id}
			if !alertSites[rec] && !added[rec] {
				added[rec] = true
				plan.AlertSites = append(plan.AlertSites, rec)
			}
		}
	}
	sort.Slice(plan.AlertSites, func(i, j int) bool {
		a, b := plan.AlertSites[i], plan.AlertSites[j]
		if a.SiteID != b.SiteID {
			return a.SiteID < b.SiteID
		}
		return a.UUID < b.UUID
	})
	return plan
}
