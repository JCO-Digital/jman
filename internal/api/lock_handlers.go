package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// maxLockCommentLength bounds an update lock's comment.
const maxLockCommentLength = 1000

// ListUpdateLocksHandler returns every update lock.
//
//	GET /api/update-locks
func ListUpdateLocksHandler(w http.ResponseWriter, r *http.Request) {
	locks, err := db.ListUpdateLocks()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListUpdateLocksHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, locks)
}

// CreateUpdateLockHandler locks a site, or one plugin on it, to fix-release
// updates. Locking something already locked replaces the lock's comment and
// author.
//
//	POST /api/sites/{id}/update-locks
//	{"plugin": "woocommerce", "comment": "Checkout customisations, test first"}
//
// An empty or missing plugin locks the whole site.
func CreateUpdateLockHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Plugin  string `json:"plugin"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Plugin != "" && !pluginSlugRegex.MatchString(body.Plugin) {
		WriteError(w, http.StatusBadRequest, "Invalid plugin slug: must start with a letter or digit and contain only [a-z0-9_-/]")
		return
	}
	body.Comment = strings.TrimSpace(body.Comment)
	if len(body.Comment) > maxLockCommentLength {
		WriteError(w, http.StatusBadRequest, "Comment is too long")
		return
	}

	// Only sites jman can update need locks.
	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	lock := models.UpdateLock{
		SiteID:    siteID,
		Plugin:    body.Plugin,
		Comment:   body.Comment,
		CreatedBy: getUsername(r),
	}
	if err := db.SaveUpdateLock(&lock); err != nil {
		verb.LogPrintf(verb.Normal, "CreateUpdateLockHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to save update lock")
		return
	}
	WriteJSON(w, http.StatusCreated, lock)
}

// DeleteUpdateLockHandler removes an update lock. Removing a site lock
// leaves the site's plugin locks in place.
//
//	DELETE /api/update-locks/{id}
func DeleteUpdateLockHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid lock ID")
		return
	}
	found, err := db.DeleteUpdateLock(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteUpdateLockHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !found {
		WriteError(w, http.StatusNotFound, "Lock not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
