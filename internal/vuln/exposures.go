package vuln

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/hashicorp/go-version"
)

// exposureReopenWindow is how long after an exposure is resolved it is
// reopened, rather than replaced by a new one, if it is seen again. A site
// missing from one refresh (an SSH failure, say) then keeps its first-seen
// time and overdue clock.
const exposureReopenWindow = 24 * time.Hour

type exposureKey struct {
	Kind, Slug, SiteID string
}

func keyOf(e models.VulnExposure) exposureKey {
	return exposureKey{e.Kind, e.Slug, e.SiteID}
}

// observedExposure is what the latest data says about one exposure.
type observedExposure struct {
	Version            string
	VulnUUIDs          []string
	MaxCvss            *float64
	FixState           string
	FixedIn            string
	PossibleFixVersion string
}

// SyncVulnExposures brings vuln_exposures in line with the current plugin
// and core vulnerability data: it opens an exposure for each site newly
// running an affected version, updates the open ones, and resolves those
// that no longer apply. Ignore entries, suppression and backport rules
// apply as in the vulnerability reports.
//
// An exposure is only resolved when its kind's data was processed and its
// vulnerability data is readable, so a failed fetch doesn't resolve (and
// later reopen) everything. Mutes are then lifted or put up for review as
// the exposures now stand (see planMuteLifecycle).
func SyncVulnExposures() error {
	matcher, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to load ignore entries: %v\n", err)
	}

	observed := map[exposureKey]observedExposure{}
	evaluated := map[string]bool{}

	if reports, err := ProcessVulnerabilities(matcher); err != nil {
		verb.LogPrintf(verb.Normal, "Warning: plugin vulnerabilities not processed, plugin exposures left as they are: %v\n", err)
	} else {
		siteUpdates := map[string]string{}
		if plugins, err := cache.GetFastCachedPlugins(); err == nil {
			for _, p := range plugins {
				siteUpdates[p.SiteID+"\x00"+p.Name] = p.Update
			}
		}
		latest := map[string]string{}
		if infos, err := db.GetAllPluginInfo(); err == nil {
			for _, info := range infos {
				latest[info.Slug] = info.Version
			}
		}
		for k, o := range pluginObservations(reports, siteUpdates, latest) {
			observed[k] = o
		}
		evaluated[models.VulnExposurePlugin] = true
	}

	if reports, err := ProcessCoreVulnerabilities(matcher); err != nil {
		verb.LogPrintf(verb.Normal, "Warning: core vulnerabilities not processed, core exposures left as they are: %v\n", err)
	} else {
		cores := map[string]models.SiteCore{}
		if all, err := db.GetAllSiteCore(); err == nil {
			for _, c := range all {
				cores[c.SiteID] = c
			}
		}
		for k, o := range coreObservations(reports, cores) {
			observed[k] = o
		}
		evaluated[models.VulnExposureCore] = true
	}

	now := time.Now()
	open, err := db.ListOpenVulnExposures()
	if err != nil {
		return err
	}
	recent, err := db.ListVulnExposuresResolvedSince(now.Add(-exposureReopenWindow))
	if err != nil {
		return err
	}

	plan := planExposureSync(open, recent, observed, evaluated, exposureDataReadable, now)
	for i := range plan.Inserts {
		if err := db.InsertVulnExposure(&plan.Inserts[i]); err != nil {
			return err
		}
	}
	for _, e := range plan.Updates {
		if err := db.UpdateVulnExposure(e); err != nil {
			return err
		}
	}
	if err := db.ResolveVulnExposures(plan.Resolves, now); err != nil {
		return err
	}
	verb.LogPrintf(verb.Verbose, "Vulnerability exposures: %d new, %d updated, %d resolved\n",
		len(plan.Inserts), len(plan.Updates), len(plan.Resolves))
	return syncVulnMutes(now)
}

// exposureDataReadable reports whether the vulnerability data an exposure
// was based on can still be read, so that its absence from the observed
// exposures means the site is no longer affected.
func exposureDataReadable(e models.VulnExposure) bool {
	if e.Kind == models.VulnExposureCore {
		resp, err := cache.GetCachedCoreVulnerabilities(e.Version)
		return err == nil && resp != nil && resp.Error == 0 && resp.Data != nil
	}
	resp, err := cache.GetCachedVulnerabilities(e.Slug)
	return err == nil && resp != nil && resp.Error == 0 && resp.Data != nil
}

type exposurePlan struct {
	Inserts  []models.VulnExposure
	Updates  []models.VulnExposure
	Resolves []int64
}

// planExposureSync works out the changes that bring the open exposures in
// line with observed. An observed exposure with no open row reopens one
// resolved within exposureReopenWindow (recent), or else is inserted. An
// open exposure that wasn't observed is resolved if its kind was evaluated
// and readable says its data could be read.
func planExposureSync(open, recent []models.VulnExposure, observed map[exposureKey]observedExposure,
	evaluated map[string]bool, readable func(models.VulnExposure) bool, now time.Time) exposurePlan {
	var plan exposurePlan

	openByKey := map[exposureKey]models.VulnExposure{}
	for _, e := range open {
		openByKey[keyOf(e)] = e
	}
	// recent is ordered by id, so the newest resolved row per key wins.
	recentByKey := map[exposureKey]models.VulnExposure{}
	for _, e := range recent {
		recentByKey[keyOf(e)] = e
	}

	keys := make([]exposureKey, 0, len(observed))
	for k := range observed {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Slug != b.Slug {
			return a.Slug < b.Slug
		}
		return a.SiteID < b.SiteID
	})

	for _, k := range keys {
		o := observed[k]
		if e, ok := openByKey[k]; ok {
			applyObservation(&e, o, now)
			plan.Updates = append(plan.Updates, e)
			continue
		}
		if e, ok := recentByKey[k]; ok {
			e.ResolvedAt = nil
			applyObservation(&e, o, now)
			plan.Updates = append(plan.Updates, e)
			continue
		}
		e := models.VulnExposure{Kind: k.Kind, Slug: k.Slug, SiteID: k.SiteID, FirstSeenAt: now}
		applyObservation(&e, o, now)
		plan.Inserts = append(plan.Inserts, e)
	}

	for _, e := range open {
		if _, ok := observed[keyOf(e)]; ok || !evaluated[e.Kind] || !readable(e) {
			continue
		}
		plan.Resolves = append(plan.Resolves, e.ID)
	}
	return plan
}

// applyObservation copies the observed state onto an exposure. The overdue
// clock (FixAvailableSince) starts when the exposure becomes fix-available
// and is cleared when it no longer is.
func applyObservation(e *models.VulnExposure, o observedExposure, now time.Time) {
	if e.FixState != o.FixState || (o.FixState == models.VulnFixAvailable && e.FixAvailableSince == nil) {
		e.FixAvailableSince = nil
		if o.FixState == models.VulnFixAvailable {
			t := now
			e.FixAvailableSince = &t
		}
	}
	e.Version = o.Version
	e.VulnUUIDs = o.VulnUUIDs
	e.MaxCvss = o.MaxCvss
	e.FixState = o.FixState
	e.FixedIn = o.FixedIn
	e.PossibleFixVersion = o.PossibleFixVersion
	e.LastSeenAt = now
}

// pluginObservations turns plugin vulnerability reports into one observed
// exposure per plugin and site, leaving out suppressed reports,
// vulnerabilities and sites. siteUpdates maps site ID + "\x00" + slug to
// the update wp-cli reports for that site, latest maps slugs to the latest
// wordpress.org version; both feed PossibleFixVersion.
func pluginObservations(reports []models.VulnReport, siteUpdates, latest map[string]string) map[exposureKey]observedExposure {
	vulns := map[exposureKey][]models.Vulnerability{}
	versions := map[exposureKey]string{}
	for _, report := range reports {
		if report.Suppressed {
			continue
		}
		for _, v := range report.Vulnerabilities {
			if v.Suppressed {
				continue
			}
			for _, site := range v.Sites {
				if site.Suppressed {
					continue
				}
				k := exposureKey{models.VulnExposurePlugin, report.Slug, site.SiteID}
				vulns[k] = append(vulns[k], v)
				versions[k] = site.Version
			}
		}
	}

	observed := make(map[exposureKey]observedExposure, len(vulns))
	for k, vs := range vulns {
		o := observedExposure{Version: versions[k], VulnUUIDs: vulnUUIDs(vs), MaxCvss: maxCvss(vs)}
		fixedIn := FixedIn(vs)
		if fixedIn != "" && !anyUnfixed(vs) {
			o.FixState, o.FixedIn = models.VulnFixAvailable, fixedIn
		} else {
			o.FixState = models.VulnNoFix
			o.PossibleFixVersion = possibleFix(o.Version, vs, siteUpdates[k.SiteID+"\x00"+k.Slug], latest[k.Slug])
		}
		observed[k] = o
	}
	return observed
}

// coreObservations turns core vulnerability reports into one observed
// exposure per site. A core exposure is fix-available when the site has a
// core update, preferring the minor one.
func coreObservations(reports []models.CoreVulnReport, cores map[string]models.SiteCore) map[exposureKey]observedExposure {
	vulns := map[exposureKey][]models.Vulnerability{}
	versions := map[exposureKey]string{}
	for _, report := range reports {
		for _, v := range report.Vulnerabilities {
			if v.Suppressed {
				continue
			}
			for _, site := range v.Sites {
				if site.Suppressed {
					continue
				}
				k := exposureKey{models.VulnExposureCore, models.CoreExposureSlug, site.SiteID}
				vulns[k] = append(vulns[k], v)
				versions[k] = report.Version
			}
		}
	}

	observed := make(map[exposureKey]observedExposure, len(vulns))
	for k, vs := range vulns {
		o := observedExposure{Version: versions[k], VulnUUIDs: vulnUUIDs(vs), MaxCvss: maxCvss(vs), FixState: models.VulnNoFix}
		core := cores[k.SiteID]
		if fix := core.MinorUpdate; fix != "" {
			o.FixState, o.FixedIn = models.VulnFixAvailable, fix
		} else if fix := core.MajorUpdate; fix != "" {
			o.FixState, o.FixedIn = models.VulnFixAvailable, fix
		}
		observed[k] = o
	}
	return observed
}

// vulnUUIDs returns the vulnerabilities' distinct UUIDs, sorted. The core
// feed reuses a UUID for several vulnerabilities, hence the dedup.
func vulnUUIDs(vs []models.Vulnerability) []string {
	seen := make(map[string]bool, len(vs))
	uuids := make([]string, 0, len(vs))
	for _, v := range vs {
		if !seen[v.Uuid] {
			seen[v.Uuid] = true
			uuids = append(uuids, v.Uuid)
		}
	}
	sort.Strings(uuids)
	return uuids
}

func maxCvss(vs []models.Vulnerability) *float64 {
	var highest *float64
	for _, v := range vs {
		if score, ok := cvssScore(v); ok && (highest == nil || score > *highest) {
			s := score
			highest = &s
		}
	}
	return highest
}

// cvssScore returns a vulnerability's CVSS score, and false if it has none.
func cvssScore(v models.Vulnerability) (float64, bool) {
	if v.Impact == nil || v.Impact.Cvss == nil || v.Impact.Cvss.Score == "" {
		return 0, false
	}
	var score float64
	if _, err := fmt.Sscanf(v.Impact.Cvss.Score, "%f", &score); err != nil {
		return 0, false
	}
	return score, true
}

func anyUnfixed(vs []models.Vulnerability) bool {
	for _, v := range vs {
		if v.Operator.Unfixed == "1" {
			return true
		}
	}
	return false
}

// possibleFix returns the newest of the candidate versions that is newer
// than installed and outside every vulnerability's affected range, or "".
func possibleFix(installed string, vs []models.Vulnerability, candidates ...string) string {
	cur, err := version.NewVersion(installed)
	if err != nil {
		return ""
	}
	var best *version.Version
	bestRaw := ""
	for _, c := range candidates {
		cv, err := version.NewVersion(c)
		if err != nil || !cv.GreaterThan(cur) || (best != nil && !cv.GreaterThan(best)) {
			continue
		}
		affected := false
		for _, v := range vs {
			if IsVersionAffected(c, v.Operator) {
				affected = true
				break
			}
		}
		if !affected {
			best, bestRaw = cv, c
		}
	}
	return bestRaw
}

// ExposureDueDays is how many days a fix-available exposure with the given
// highest CVSS may stay open before it is overdue. Unscored exposures use
// the medium tier.
func ExposureDueDays(maxCvss *float64) int {
	days := func(v, def int) int {
		if v > 0 {
			return v
		}
		return def
	}
	switch {
	case maxCvss == nil:
		return days(config.Cfg.VulnDueMediumDays, 14)
	case *maxCvss >= 9:
		return days(config.Cfg.VulnDueCriticalDays, 2)
	case *maxCvss >= 7:
		return days(config.Cfg.VulnDueHighDays, 7)
	case *maxCvss >= 4:
		return days(config.Cfg.VulnDueMediumDays, 14)
	default:
		return days(config.Cfg.VulnDueLowDays, 30)
	}
}

// ExposureDue returns when an open fix-available exposure becomes overdue,
// and whether it already is at now. It returns nil for no-fix and resolved
// exposures, which are never overdue.
func ExposureDue(e models.VulnExposure, now time.Time) (*time.Time, bool) {
	if e.ResolvedAt != nil || e.FixState != models.VulnFixAvailable || e.FixAvailableSince == nil {
		return nil, false
	}
	due := e.FixAvailableSince.AddDate(0, 0, ExposureDueDays(e.MaxCvss))
	return &due, now.After(due)
}

// vulnDisplayName returns the most readable name for a vulnerability (the
// first non-CVE source name, which says what kind of vulnerability it is)
// and the first source link. Core vulnerabilities are named by their
// version only, so one with nothing but CVE sources is named by its CVE.
func vulnDisplayName(v models.Vulnerability) (name, link string) {
	name, named := v.Name, false
	for _, source := range v.Source {
		if !named && !strings.HasPrefix(source.Name, "CVE") {
			name, named = source.Name, true
		}
		if link == "" && source.Link != "" {
			link = source.Link
		}
	}
	if !named && len(v.Source) > 0 && strings.Trim(v.Name, "0123456789.") == "" {
		name = v.Source[0].Name
	}
	return name, link
}
