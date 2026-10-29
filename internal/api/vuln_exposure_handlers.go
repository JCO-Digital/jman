package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
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
	mutes, err := db.ListOpenVulnMutes()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to list vulnerability mutes: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to list vulnerability mutes")
		return
	}
	WriteJSON(w, http.StatusOK, vuln.SummarizeExposures(exposures, mutes, time.Now()))
}

// vulnMuteRequest is the body of POST /api/vulns/mutes and of a mute's
// confirm.
type vulnMuteRequest struct {
	Kind   string `json:"kind"`
	Slug   string `json:"slug"`
	SiteID string `json:"site_id"`
	Reason string `json:"reason"`
}

// ListVulnMutesHandler handles GET /api/vulns/mutes: the open mutes (active
// or needing review), or with ?status=lifted the mutes lifted in the last
// ?days= (default 30, max 365).
func ListVulnMutesHandler(w http.ResponseWriter, r *http.Request) {
	var mutes []models.VulnMute
	var err error
	switch r.URL.Query().Get("status") {
	case "", "open":
		mutes, err = db.ListOpenVulnMutes()
	case "lifted":
		days := defaultResolvedExposureDays
		if v := r.URL.Query().Get("days"); v != "" {
			days, err = strconv.Atoi(v)
			if err != nil || days < 1 || days > 365 {
				WriteError(w, http.StatusBadRequest, "days must be between 1 and 365")
				return
			}
		}
		mutes, err = db.ListVulnMutesLiftedSince(time.Now().AddDate(0, 0, -days))
	default:
		WriteError(w, http.StatusBadRequest, "status must be open or lifted")
		return
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to list vulnerability mutes: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to list vulnerability mutes")
		return
	}
	vuln.DescribeMutes(mutes)
	WriteJSON(w, http.StatusOK, mutes)
}

// MuteVulnExposureHandler handles POST /api/vulns/mutes: it mutes the open
// exposure of a plugin (or core) on a site, with a required reason. The
// mute records the exposure as it is now and needs review after
// models.VulnMuteReviewDays.
func MuteVulnExposureHandler(w http.ResponseWriter, r *http.Request) {
	var req vulnMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Kind == "" || req.Slug == "" || req.SiteID == "" || req.Reason == "" {
		WriteError(w, http.StatusBadRequest, "kind, slug, site_id and reason are required")
		return
	}
	exposure, err := db.GetOpenVulnExposure(req.Kind, req.Slug, req.SiteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "MuteVulnExposureHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if exposure == nil {
		WriteError(w, http.StatusNotFound, "No open vulnerability exposure for that plugin and site")
		return
	}

	by := requestUsername(r)
	mute := models.VulnMute{Kind: req.Kind, Slug: req.Slug, SiteID: req.SiteID, Reason: req.Reason,
		AuditFields: models.AuditFields{CreatedBy: by}}
	vuln.SnapshotMute(&mute, *exposure, time.Now())
	if err := db.InsertVulnMute(&mute); err != nil {
		if errors.Is(err, db.ErrVulnMuteExists) {
			WriteError(w, http.StatusConflict, "That plugin is already muted on the site")
			return
		}
		verb.LogPrintf(verb.Normal, "MuteVulnExposureHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	verb.LogPrintf(verb.Normal, "%s muted %s %s on %s (%s): %s", by, mute.Kind, mute.Slug, mute.SiteID, exposure.FixState, mute.Reason)
	WriteJSON(w, http.StatusCreated, mute)
}

// ConfirmVulnMuteHandler handles POST /api/vulns/mutes/{id}/confirm: it
// keeps an open mute for another review period, with a (new) reason,
// recording the exposure as it is now.
func ConfirmVulnMuteHandler(w http.ResponseWriter, r *http.Request) {
	mute, ok := openVulnMute(w, r)
	if !ok {
		return
	}
	var req vulnMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Reason = strings.TrimSpace(req.Reason); req.Reason == "" {
		WriteError(w, http.StatusBadRequest, "reason is required")
		return
	}
	exposure, err := db.GetOpenVulnExposure(mute.Kind, mute.Slug, mute.SiteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ConfirmVulnMuteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if exposure == nil {
		WriteError(w, http.StatusConflict, "The site is no longer affected; the mute will be lifted")
		return
	}

	mute.Reason, mute.UpdatedBy = req.Reason, requestUsername(r)
	vuln.SnapshotMute(mute, *exposure, time.Now())
	if err := db.UpdateVulnMute(mute); err != nil {
		verb.LogPrintf(verb.Normal, "ConfirmVulnMuteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	verb.LogPrintf(verb.Normal, "%s confirmed vulnerability mute %d: %s", mute.UpdatedBy, mute.ID, mute.Reason)
	WriteJSON(w, http.StatusOK, mute)
}

// UnmuteVulnHandler handles DELETE /api/vulns/mutes/{id}: it lifts an open
// mute. The mute is kept for the record.
func UnmuteVulnHandler(w http.ResponseWriter, r *http.Request) {
	mute, ok := openVulnMute(w, r)
	if !ok {
		return
	}
	now := time.Now()
	by := requestUsername(r)
	mute.Status, mute.LiftedAt, mute.LiftedReason, mute.UpdatedBy = models.VulnMuteLifted, &now, "Unmuted by "+by, by
	if err := db.UpdateVulnMute(mute); err != nil {
		verb.LogPrintf(verb.Normal, "UnmuteVulnHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	verb.LogPrintf(verb.Normal, "%s lifted vulnerability mute %d", by, mute.ID)
	w.WriteHeader(http.StatusNoContent)
}

// openVulnMute loads the open mute named by the {id} path value, writing an
// error response and returning false if there is none.
func openVulnMute(w http.ResponseWriter, r *http.Request) (*models.VulnMute, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid mute ID")
		return nil, false
	}
	mute, err := db.GetVulnMute(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "openVulnMute: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return nil, false
	}
	if mute == nil || !mute.Open() {
		WriteError(w, http.StatusNotFound, "Mute not found or already lifted")
		return nil, false
	}
	return mute, true
}
