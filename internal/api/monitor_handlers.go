package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
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
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch monitor history: %v", err))
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
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
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
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch monitor statuses: %v", err))
		return
	}
	WriteJSON(w, http.StatusOK, statuses)
}

// IgnoredSitesHandler handles listing and adding ignored sites.
func IgnoredSitesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sites, err := db.GetIgnoredSites()
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch ignored sites: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, sites)

	case http.MethodPost:
		var req struct {
			Domain string `json:"domain"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if req.Domain == "" {
			WriteError(w, http.StatusBadRequest, "Domain is required")
			return
		}

		if err := db.IgnoreSite(req.Domain, req.Reason); err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to ignore site: %v", err))
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]string{"status": "site ignored"})

	default:
		w.Header().Set("Allow", "GET, POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// UnignoreSiteHandler handles removing a site from the ignore list.
func UnignoreSiteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	domain := r.PathValue("domain")
	if domain == "" {
		WriteError(w, http.StatusBadRequest, "Domain is required")
		return
	}

	if err := db.UnignoreSite(domain); err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to unignore site: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "site unignored"})
}
