package api

import (
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/verb"
)

// MonitorHistoryHandler returns aggregated history from monitor_history.
// Query param: hours (default 48).
func MonitorHistoryHandler(w http.ResponseWriter, r *http.Request) {
	hoursStr := r.URL.Query().Get("hours")
	hours := 48
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}

	history, err := db.GetMonitorHistory(hours)
	if err != nil {
		verb.LogPrintf(verb.Normal, "MonitorHistoryHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, history)
}

// MonitorStatusHandler returns the current status of a specific site or all sites.
// Query param: domain (optional).
func MonitorStatusHandler(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")

	if domain != "" {
		status, err := db.GetMonitorStatus(domain)
		if err != nil {
			verb.LogPrintf(verb.Normal, "MonitorStatusHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if status == nil {
			// If site exists in cache but hasn't been checked yet, return a pending status
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"domain":         domain,
				"is_down":        false,
				"failure_count":  0,
				"last_checked":   nil,
				"status_message": "Pending first check",
			})
			return
		}
		WriteJSON(w, http.StatusOK, status)
		return
	}

	statuses, err := db.GetAllMonitorStatuses()
	if err != nil {
		verb.LogPrintf(verb.Normal, "MonitorStatusHandler: failed to fetch all statuses: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, statuses)
}
