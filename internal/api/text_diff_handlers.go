package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/textdiff"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/verb"
)

// Limits on text diff settings (see docs/update-text-diff.md).
const (
	maxTextDiffPages      = 20
	maxTextDiffMuteLine   = 2000
	maxTextDiffMuteReason = 500
	maxTextDiffMuteDays   = 365
)

// ListSiteDiffPagesHandler returns a site's extra text diff pages. The
// frontpage is always compared and isn't listed.
func ListSiteDiffPagesHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	pages, err := db.ListSiteDiffPages(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListSiteDiffPagesHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, pages)
}

// CreateSiteDiffPageHandler adds an extra page, by path, to a site's text
// diffs.
func CreateSiteDiffPageHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	path, ok := updatejobs.ValidTextDiffPath(body.Path)
	if !ok {
		WriteError(w, http.StatusBadRequest, "Invalid path: it must start with / and stay on the site, e.g. /en/")
		return
	}
	if path == "/" {
		WriteError(w, http.StatusBadRequest, "The frontpage is always compared")
		return
	}
	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	existing, err := db.ListSiteDiffPages(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CreateSiteDiffPageHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if len(existing) >= maxTextDiffPages {
		WriteError(w, http.StatusBadRequest, "A site can have at most 20 extra pages")
		return
	}

	page := models.SiteDiffPage{SiteID: siteID, Path: path, CreatedBy: getUsername(r)}
	if err := db.InsertSiteDiffPage(&page); err != nil {
		if errors.Is(err, db.ErrSiteDiffPageExists) {
			WriteError(w, http.StatusConflict, err.Error())
			return
		}
		verb.LogPrintf(verb.Normal, "CreateSiteDiffPageHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to add the page")
		return
	}
	WriteJSON(w, http.StatusCreated, page)
}

// DeleteSiteDiffPageHandler removes an extra page from a site's text diffs.
func DeleteSiteDiffPageHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := strconv.ParseInt(r.PathValue("pageId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid page ID")
		return
	}
	found, err := db.DeleteSiteDiffPage(siteID, id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteSiteDiffPageHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !found {
		WriteError(w, http.StatusNotFound, "Page not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListTextDiffMutesHandler returns a site's text diff mutes, expired ones
// included; a mute is active while expires_at is in the future.
func ListTextDiffMutesHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	mutes, err := db.ListTextDiffMutes(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListTextDiffMutesHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, mutes)
}

// CreateTextDiffMuteHandler mutes a changed line on one of a site's pages.
// The line is sent as shown in the diff and stored normalized, so it still
// matches when only digits or dates in it change. days defaults to
// textDiffMuteDays.
func CreateTextDiffMuteHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		Path      string `json:"path"`
		Direction string `json:"direction"`
		Text      string `json:"text"`
		Reason    string `json:"reason"`
		Days      int    `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	path, ok := updatejobs.ValidTextDiffPath(body.Path)
	if !ok {
		WriteError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	if body.Direction != models.TextDiffAdded && body.Direction != models.TextDiffRemoved {
		WriteError(w, http.StatusBadRequest, `direction must be "added" or "removed"`)
		return
	}
	line := textdiff.Normalize(body.Text)
	if line == "" || len(line) > maxTextDiffMuteLine {
		WriteError(w, http.StatusBadRequest, "text must be a line from the diff")
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if len(body.Reason) > maxTextDiffMuteReason {
		WriteError(w, http.StatusBadRequest, "Reason is too long")
		return
	}
	days := body.Days
	if days == 0 {
		days = config.Cfg.TextDiffMuteDays
	}
	if days < 1 || days > maxTextDiffMuteDays {
		WriteError(w, http.StatusBadRequest, "days must be between 1 and 365")
		return
	}
	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	m := models.TextDiffMute{
		SiteID:      siteID,
		Path:        path,
		Direction:   body.Direction,
		Line:        line,
		Reason:      body.Reason,
		ExpiresAt:   time.Now().AddDate(0, 0, days),
		AuditFields: models.AuditFields{CreatedBy: getUsername(r)},
	}
	if err := db.InsertTextDiffMute(&m); err != nil {
		verb.LogPrintf(verb.Normal, "CreateTextDiffMuteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to save the mute")
		return
	}
	WriteJSON(w, http.StatusCreated, m)
}

// DeleteTextDiffMuteHandler ends a mute now. It is kept for the record.
func DeleteTextDiffMuteHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid mute ID")
		return
	}
	m, err := db.GetTextDiffMute(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteTextDiffMuteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if m == nil {
		WriteError(w, http.StatusNotFound, "Mute not found")
		return
	}
	if err := db.ExpireTextDiffMute(id, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteTextDiffMuteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
