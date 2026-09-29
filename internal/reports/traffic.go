package reports

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// maxTrafficReportDays bounds the report's date range to avoid an unbounded
// scan across all sites' daily traffic history.
const maxTrafficReportDays = 366

type trafficReport struct{}

func init() {
	Register(&trafficReport{})
}

func (r *trafficReport) ID() string   { return "traffic" }
func (r *trafficReport) Name() string { return "Traffic Analytics" }
func (r *trafficReport) Description() string {
	return "Total visitor traffic per site for the selected date range."
}

func (r *trafficReport) Params() []ParamDef {
	return []ParamDef{
		{Key: "range", Type: ParamDateRange, Label: "Date range", Required: false},
	}
}

func (r *trafficReport) Run(q url.Values) (*Result, error) {
	start, end, err := ParseDateRange(q, maxTrafficReportDays)
	if err != nil {
		return nil, err
	}

	dailyRows, err := db.GetSiteTrafficDailyRange(start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to load traffic data: %w", err)
	}

	// Enrich site_id (a UUID) with its domain. site_traffic_* live in api.db
	// while sites live in inventory.db, so this is a lookup rather than a join.
	// The SpinupWP cache is consulted too, for sites not yet synced into the
	// inventory.
	domainByID := make(map[string]string)
	sites := []models.Site{}
	_ = cache.ReadJSONCache("sites", &sites, -1)
	for _, s := range sites {
		domainByID[utils.SpinupWPSiteUUID(s.ID)] = s.Domain
	}
	if managedSites, err := db.ListManagedSites(); err == nil {
		for _, ms := range managedSites {
			domainByID[ms.ID] = ms.Domain
		}
	}

	// Sum each site's daily rows into a single total-for-the-range row.
	// unique_visitors is therefore the sum of each day's unique count, which
	// over-counts a visitor active across multiple days in the range (same
	// caveat as SiteTrafficPeriod's own daily/monthly rollups — true
	// range-distinct isn't tracked to avoid retaining raw IPs).
	type totals struct {
		total, human, bot, unique int
	}
	totalsBySite := map[string]*totals{}
	var siteOrder []string
	for _, row := range dailyRows {
		siteKey := row.SiteID
		t, ok := totalsBySite[siteKey]
		if !ok {
			t = &totals{}
			totalsBySite[siteKey] = t
			siteOrder = append(siteOrder, siteKey)
		}
		t.total += row.RequestsTotal
		t.human += row.RequestsHuman
		t.bot += row.RequestsBot
		t.unique += row.UniqueVisitors
	}
	sort.Strings(siteOrder)

	rows := make([]map[string]any, 0, len(siteOrder))
	for _, siteKey := range siteOrder {
		site := domainByID[siteKey]
		if site == "" {
			site = fmt.Sprintf("Site %s", siteKey)
		}
		t := totalsBySite[siteKey]
		rows = append(rows, map[string]any{
			"site":            site,
			"requests_total":  t.total,
			"requests_human":  t.human,
			"requests_bot":    t.bot,
			"unique_visitors": t.unique,
		})
	}

	return &Result{
		Columns: []Column{
			{Key: "site", Label: "Site", Type: ColumnText},
			{Key: "requests_total", Label: "Total Requests", Type: ColumnNumber},
			{Key: "requests_human", Label: "Human Requests", Type: ColumnNumber},
			{Key: "requests_bot", Label: "Bot Requests", Type: ColumnNumber},
			{Key: "unique_visitors", Label: "Unique Visitors", Type: ColumnNumber},
		},
		Rows: rows,
	}, nil
}
