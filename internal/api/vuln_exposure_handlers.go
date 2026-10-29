package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/vuln"
)

// defaultResolvedExposureDays is how far back GET /api/vulns/exposures
// lists resolved exposures when no days parameter is given.
const defaultResolvedExposureDays = 30

// ListVulnExposuresHandler returns vulnerability exposures grouped by plugin
// (or core version). Query params: status (open, the default, or
// resolved), days (for resolved: how far back, default 30, max 365).
func ListVulnExposuresHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	var exposures []models.VulnExposure
	var err error

	switch status := r.URL.Query().Get("status"); status {
	case "", "open":
		exposures, err = db.ListOpenVulnExposures()
	case "resolved":
		days := defaultResolvedExposureDays
		if v := r.URL.Query().Get("days"); v != "" {
			days, err = strconv.Atoi(v)
			if err != nil || days < 1 || days > 365 {
				WriteError(w, http.StatusBadRequest, "days must be between 1 and 365")
				return
			}
		}
		exposures, err = db.ListVulnExposuresResolvedSince(now.AddDate(0, 0, -days))
	default:
		WriteError(w, http.StatusBadRequest, "status must be open or resolved")
		return
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to list vulnerability exposures: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to list vulnerability exposures")
		return
	}

	WriteJSON(w, http.StatusOK, vuln.BuildExposureGroups(exposures, now))
}

// VulnExposureSummaryHandler returns counts of the open exposures, for
// badges.
func VulnExposureSummaryHandler(w http.ResponseWriter, r *http.Request) {
	exposures, err := db.ListOpenVulnExposures()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to list vulnerability exposures: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to list vulnerability exposures")
		return
	}
	WriteJSON(w, http.StatusOK, vuln.SummarizeExposures(exposures, time.Now()))
}
