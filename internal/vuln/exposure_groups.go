package vuln

import (
	"sort"
	"strconv"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// groupDetails is the feed data an exposure group shows next to its sites.
type groupDetails struct {
	Name         string
	Closed       bool
	ClosedReason string
	ClosedDate   string
	LastUpdated  *time.Time
	Vulns        map[string]models.Vulnerability
}

// BuildExposureGroups groups exposures into one group per plugin, or per
// core version, with site names, vulnerability details from the cached feed
// and each site's overdue state at now. Groups with overdue sites come
// first, then by highest CVSS and name; within a group, overdue sites come
// first, then by site name.
func BuildExposureGroups(exposures []models.VulnExposure, now time.Time) []models.VulnExposureGroup {
	siteNames := map[string]string{}
	if sites, err := cache.GetFastSiteList(); err == nil {
		for _, s := range sites {
			siteNames[s.ID] = s.Name
		}
	}
	return groupExposures(exposures, siteNames, loadGroupDetails, now)
}

// loadGroupDetails reads a group's feed data from the vulnerability cache.
// Missing data leaves the group with its slug (or core version) as name and
// vulnerabilities listed by UUID only.
func loadGroupDetails(kind, slug, coreVersion string) groupDetails {
	d := groupDetails{Name: slug, Vulns: map[string]models.Vulnerability{}}
	if kind == models.VulnExposureCore {
		d.Name = "WordPress " + coreVersion
		if resp, err := cache.GetCachedCoreVulnerabilities(coreVersion); err == nil && resp != nil && resp.Data != nil {
			for _, v := range resp.Data.Vulnerability {
				d.Vulns[v.Uuid] = v
			}
		}
		return d
	}
	resp, err := cache.GetCachedVulnerabilities(slug)
	if err != nil || resp == nil || resp.Data == nil {
		return d
	}
	data := resp.Data
	if data.Name != nil && *data.Name != "" {
		d.Name = utils.CleanHTML(*data.Name)
	}
	d.Closed = bool(data.Closed)
	if data.ClosedReason != nil {
		d.ClosedReason = *data.ClosedReason
	}
	if data.ClosedDate != nil {
		d.ClosedDate = *data.ClosedDate
	}
	if data.Latest != nil {
		if ts, err := strconv.ParseInt(*data.Latest, 10, 64); err == nil && ts > 0 {
			t := time.Unix(ts, 0).UTC()
			d.LastUpdated = &t
		}
	}
	for _, v := range data.Vulnerability {
		d.Vulns[v.Uuid] = v
	}
	return d
}

func groupExposures(exposures []models.VulnExposure, siteNames map[string]string,
	details func(kind, slug, coreVersion string) groupDetails, now time.Time) []models.VulnExposureGroup {
	type groupKey struct{ Kind, Slug, Version string }
	groups := map[groupKey]*models.VulnExposureGroup{}
	var order []groupKey

	for _, e := range exposures {
		k := groupKey{Kind: e.Kind, Slug: e.Slug}
		if e.Kind == models.VulnExposureCore {
			k.Version = e.Version
		}
		g, ok := groups[k]
		if !ok {
			g = &models.VulnExposureGroup{Kind: k.Kind, Slug: k.Slug, Version: k.Version,
				Vulnerabilities: []models.VulnExposureVuln{}, Sites: []models.VulnExposureSite{}}
			groups[k] = g
			order = append(order, k)
		}

		site := models.VulnExposureSite{VulnExposure: e, SiteName: siteNames[e.SiteID]}
		if site.SiteName == "" {
			site.SiteName = e.SiteID
		}
		site.DueAt, site.Overdue = ExposureDue(e, now)
		g.Sites = append(g.Sites, site)

		if e.MaxCvss != nil && (g.MaxCvss == nil || *e.MaxCvss > *g.MaxCvss) {
			score := *e.MaxCvss
			g.MaxCvss = &score
		}
		if e.FixState == models.VulnFixAvailable {
			g.FixAvailableCount++
		} else {
			g.NoFixCount++
		}
		if site.Overdue {
			g.OverdueCount++
		}
	}

	result := make([]models.VulnExposureGroup, 0, len(order))
	for _, k := range order {
		g := groups[k]
		d := details(k.Kind, k.Slug, k.Version)
		g.Name, g.Closed, g.ClosedReason, g.ClosedDate, g.LastUpdated = d.Name, d.Closed, d.ClosedReason, d.ClosedDate, d.LastUpdated

		seen := map[string]bool{}
		for _, s := range g.Sites {
			for _, uuid := range s.VulnUUIDs {
				if seen[uuid] {
					continue
				}
				seen[uuid] = true
				g.Vulnerabilities = append(g.Vulnerabilities, exposureVuln(uuid, d.Vulns))
			}
		}
		sort.SliceStable(g.Vulnerabilities, func(i, j int) bool {
			return cvssOrZero(g.Vulnerabilities[i].Cvss) > cvssOrZero(g.Vulnerabilities[j].Cvss)
		})
		sort.SliceStable(g.Sites, func(i, j int) bool {
			if g.Sites[i].Overdue != g.Sites[j].Overdue {
				return g.Sites[i].Overdue
			}
			return g.Sites[i].SiteName < g.Sites[j].SiteName
		})
		result = append(result, *g)
	}

	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if (a.OverdueCount > 0) != (b.OverdueCount > 0) {
			return a.OverdueCount > 0
		}
		if cvssOrZero(a.MaxCvss) != cvssOrZero(b.MaxCvss) {
			return cvssOrZero(a.MaxCvss) > cvssOrZero(b.MaxCvss)
		}
		return a.Name < b.Name
	})
	return result
}

func exposureVuln(uuid string, vulns map[string]models.Vulnerability) models.VulnExposureVuln {
	out := models.VulnExposureVuln{UUID: uuid, Name: uuid}
	v, ok := vulns[uuid]
	if !ok {
		return out
	}
	out.Name, out.Link = vulnDisplayName(v)
	out.Name = utils.CleanHTML(out.Name)
	if score, ok := cvssScore(v); ok {
		out.Cvss = &score
	}
	out.FixedIn = FixedIn([]models.Vulnerability{v})
	out.Unfixed = v.Operator.Unfixed == "1"
	return out
}

func cvssOrZero(score *float64) float64 {
	if score == nil {
		return 0
	}
	return *score
}

// SummarizeExposures counts open exposures at now: groups, exposures,
// overdue ones and no-fix ones.
func SummarizeExposures(exposures []models.VulnExposure, now time.Time) models.VulnExposureSummary {
	var s models.VulnExposureSummary
	groups := map[string]bool{}
	for _, e := range exposures {
		if e.ResolvedAt != nil {
			continue
		}
		key := e.Kind + "\x00" + e.Slug
		if e.Kind == models.VulnExposureCore {
			key += "\x00" + e.Version
		}
		groups[key] = true
		s.Exposures++
		if _, overdue := ExposureDue(e, now); overdue {
			s.Overdue++
		}
		if e.FixState == models.VulnNoFix {
			s.NoFix++
		}
	}
	s.Groups = len(groups)
	return s
}
