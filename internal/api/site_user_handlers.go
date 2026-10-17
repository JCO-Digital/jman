package api

import (
	"net/http"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// siteUsersResponse is every site's stored WordPress users.
type siteUsersResponse struct {
	Users     []models.SiteUser        `json:"users"`
	Summaries []models.SiteUserSummary `json:"summaries"`
}

// SiteUsersHandler returns the stored WordPress users of every site (not
// jman's own users, see ListUsersHandler), ordered by site and user ID,
// with each site's summary.
//
//	GET /api/site-users
func SiteUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := db.GetAllSiteUsers()
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUsersHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	summaries, err := db.GetAllSiteUserSummaries()
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUsersHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, siteUsersResponse{Users: users, Summaries: summaries})
}

// siteUserListResponse is one site's stored WordPress users. UpdatedAt is
// empty if they were never read.
type siteUserListResponse struct {
	models.SiteUsers
	UpdatedAt string `json:"updated_at"`
}

// SiteUserListHandler returns a site's stored WordPress users, ordered by
// ID.
//
//	GET /api/sites/{id}/users
func SiteUserListHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	users, _, err := db.GetSiteUsers(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUserListHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	summary, _, err := db.GetSiteUserSummary(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUserListHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if users.Users == nil {
		users.Users = []models.SiteUser{}
	}
	if users.ExcludedCounts == nil {
		users.ExcludedCounts = map[string]int{}
	}
	WriteJSON(w, http.StatusOK, siteUserListResponse{SiteUsers: users, UpdatedAt: summary.UpdatedAt})
}
