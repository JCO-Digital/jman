package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

var validSiteEnvironments = map[string]bool{
	string(models.SiteEnvironmentProduction):  true,
	string(models.SiteEnvironmentStaging):     true,
	string(models.SiteEnvironmentDevelopment): true,
	string(models.SiteEnvironmentArchived):    true,
}

// SetSiteEnvironmentHandler sets or clears the environment classification for a site.
// An empty environment value clears the classification (unclassified).
func SetSiteEnvironmentHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	var body struct {
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if body.Environment == "" {
		if err := db.ClearSiteEnvironment(siteID); err != nil {
			verb.LogPrintf(verb.Normal, "SetSiteEnvironmentHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if !validSiteEnvironments[body.Environment] {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid environment: %q", body.Environment))
		return
	}

	if err := db.SetSiteEnvironment(siteID, body.Environment, getUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "SetSiteEnvironmentHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusOK)
}
