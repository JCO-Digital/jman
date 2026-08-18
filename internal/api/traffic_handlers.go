package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
)

// maxSiteTrafficDays bounds how far back a single request can query, to
// avoid an unbounded scan for a heavily-used, long-lived site.
const maxSiteTrafficDays = 90

// SiteTrafficHandler returns a site's aggregated visitor traffic, collected
// locally by jman-agent from its access logs and pushed via POST
// /api/agent/report. Supports ?period=hourly|daily (default hourly) and
// ?days=N (default 7, capped at maxSiteTrafficDays).
func SiteTrafficHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	period := "hourly"
	if r.URL.Query().Get("period") == "daily" {
		period = "daily"
	}

	days := 7
	if raw := r.URL.Query().Get("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			days = parsed
		}
	}
	if days > maxSiteTrafficDays {
		days = maxSiteTrafficDays
	}

	traffic, err := db.GetSiteTraffic(siteID, period, days)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load site traffic: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, traffic)
}
