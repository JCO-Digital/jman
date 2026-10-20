package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
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

// minUserPasswordLength is the shortest password a created user may get.
const minUserPasswordLength = 12

var (
	// wpLoginRegex is what WordPress's sanitize_user keeps (letters,
	// digits, space, _ . - @), not starting with "-" so wp-cli can't take
	// it for an option.
	wpLoginRegex = regexp.MustCompile(`^[A-Za-z0-9_.@][A-Za-z0-9 _.@-]{0,59}$`)
	wpRoleRegex  = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]*$`)
)

// SiteUserCreateHandler queues creating a WordPress user on a site. The
// password is held in jman-api's memory only until the job runs; it's never
// stored or returned. The role defaults to administrator.
//
//	POST /api/sites/{id}/users
//	{"login": "jco", "email": "jco@example.com", "role": "administrator", "password": "…"}
func SiteUserCreateHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		Login    string `json:"login"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Role == "" {
		body.Role = "administrator"
	}
	if msg := checkNewUser(body.Login, body.Email, body.Role, body.Password); msg != "" {
		WriteError(w, http.StatusBadRequest, msg)
		return
	}
	// The job fails the same way, but this answers at once.
	users, _, err := db.GetSiteUsers(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUserCreateHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	for _, u := range users.Users {
		if strings.EqualFold(u.Login, body.Login) || strings.EqualFold(u.Email, body.Email) {
			WriteError(w, http.StatusConflict, fmt.Sprintf("The site already has the user %s <%s>", u.Login, u.Email))
			return
		}
	}
	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	job := models.NewUpdateJob(models.UpdateJobKindUserCreate, siteID, models.UserCreatePayload{
		Login: body.Login,
		Email: body.Email,
		Role:  body.Role,
	})
	job.CreatedBy = getUsername(r)
	if err := updatejobs.EnqueueUserCreate(&job, body.Password); err != nil {
		verb.LogPrintf(verb.Normal, "SiteUserCreateHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to queue the user")
		return
	}
	WriteJSON(w, http.StatusAccepted, job)
}

// checkNewUser returns why a user can't be created as given, or "".
func checkNewUser(login, email, role, password string) string {
	if !wpLoginRegex.MatchString(login) || strings.TrimSpace(login) != login {
		return "The login may have up to 60 letters, digits, spaces and _ . - @, and must not start with - or a space or end with a space"
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || strings.HasPrefix(email, "-") {
		return "Invalid email address"
	}
	if !wpRoleRegex.MatchString(role) {
		return "Invalid role"
	}
	if len(password) < minUserPasswordLength {
		return fmt.Sprintf("The password must have at least %d characters", minUserPasswordLength)
	}
	if strings.ContainsFunc(password, unicode.IsControl) {
		return "The password must not contain control characters"
	}
	return ""
}

// SiteUserDeleteHandler queues deleting a WordPress user of a site, as
// stored in jman's user list. reassign is the login or email of the user
// who gets the deleted user's content; without it, a user with posts isn't
// deleted. Multisite sites are refused.
//
//	DELETE /api/sites/{id}/users/{userId}?reassign=admin
func SiteUserDeleteHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil || userID <= 0 {
		WriteError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}
	reassign := strings.TrimSpace(r.URL.Query().Get("reassign"))

	user, found, err := db.GetSiteUser(siteID, userID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUserDeleteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !found {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("User %d isn't in the site's user list", userID))
		return
	}
	flags, err := db.GetAllSiteWpFlags()
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUserDeleteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if flags[siteID].IsMultisite {
		WriteError(w, http.StatusConflict, "Deleting users on multisite sites isn't supported")
		return
	}
	// The job checks the site's own count; this answers at once.
	if user.PostCount != nil && *user.PostCount > 0 && reassign == "" {
		WriteError(w, http.StatusConflict, fmt.Sprintf("%s has %d posts, which deleting the user would delete; give a user to reassign them to", user.Login, *user.PostCount))
		return
	}
	if strings.EqualFold(reassign, user.Login) || strings.EqualFold(reassign, user.Email) {
		WriteError(w, http.StatusBadRequest, "The content can't be reassigned to the user being deleted")
		return
	}
	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	job := models.NewUpdateJob(models.UpdateJobKindUserDelete, siteID, models.UserDeletePayload{
		UserID:   user.ID,
		Login:    user.Login,
		Email:    user.Email,
		Reassign: reassign,
	})
	job.CreatedBy = getUsername(r)
	if err := updatejobs.Enqueue(&job); err != nil {
		verb.LogPrintf(verb.Normal, "SiteUserDeleteHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to queue the user delete")
		return
	}
	WriteJSON(w, http.StatusAccepted, job)
}
