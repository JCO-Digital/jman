package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// maxSiteTrafficDays bounds how far back an ?period=hourly|daily request
// can query, to avoid an unbounded scan for a heavily-used, long-lived site.
const maxSiteTrafficDays = 90

// maxSiteTrafficMonthlyDays bounds how far back a ?period=monthly request
// can query before being grouped into calendar months — enough to
// comfortably cover up to a year of history without an unbounded scan.
const maxSiteTrafficMonthlyDays = 366

// SiteTrafficHandler returns a site's aggregated visitor traffic, collected
// locally by jman-agent from its access logs and pushed via POST
// /api/agent/report. Supports ?period=hourly|daily|monthly (default hourly)
// and ?days=N (default 7, capped at maxSiteTrafficDays for hourly/daily or
// maxSiteTrafficMonthlyDays for monthly).
func SiteTrafficHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	period := r.URL.Query().Get("period")
	if period != "daily" && period != "monthly" {
		period = "hourly"
	}

	maxDays := maxSiteTrafficDays
	if period == "monthly" {
		maxDays = maxSiteTrafficMonthlyDays
	}

	days := 7
	if raw := r.URL.Query().Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			days = parsed
		}
	}
	if days > maxDays {
		days = maxDays
	}

	var traffic []models.SiteTrafficPeriod
	if period == "monthly" {
		traffic, err = db.GetSiteTrafficMonthly(siteID, days)
	} else {
		traffic, err = db.GetSiteTraffic(siteID, period, days)
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load site traffic: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, traffic)
}
