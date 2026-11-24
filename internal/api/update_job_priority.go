package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/verb"
)

// errUrgentNeedsAdmin is returned for an urgent priority asked for by a
// user below the admin level.
var errUrgentNeedsAdmin = errors.New("Only admins can make a job urgent")

// defaultJobPriority is the priority of jobs a request queues on sites
// sites when it asks for none: high for one site, as someone is usually
// waiting for it, normal for more (docs/update-queue.md).
func defaultJobPriority(sites int) models.UpdateJobPriority {
	if sites == 1 {
		return models.UpdateJobPriorityHigh
	}
	return models.UpdateJobPriorityNormal
}

// parseJobPriority parses a priority a request asks for. Urgent needs the
// admin level (errUrgentNeedsAdmin).
func parseJobPriority(r *http.Request, name string) (models.UpdateJobPriority, error) {
	p, err := models.ParseUpdateJobPriority(name)
	if err != nil {
		return 0, err
	}
	if p == models.UpdateJobPriorityUrgent {
		claims := GetAuthClaims(r.Context())
		if claims == nil || levelToInt(claims.Level) < levelToInt(config.LevelAdmin) {
			return 0, errUrgentNeedsAdmin
		}
	}
	return p, nil
}

// writePriorityError answers a request whose priority was refused.
func writePriorityError(w http.ResponseWriter, err error) {
	if errors.Is(err, errUrgentNeedsAdmin) {
		WriteError(w, http.StatusForbidden, err.Error())
		return
	}
	WriteError(w, http.StatusBadRequest, err.Error())
}

// vulnerablePluginsBySite returns site ID → plugin slugs with an open
// vulnerability exposure, or nil if exposures can't be loaded.
func vulnerablePluginsBySite() map[string]map[string]bool {
	exposures, err := db.ListOpenVulnExposures()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to load vulnerability exposures for job priorities: %v", err)
		return nil
	}
	bySite := map[string]map[string]bool{}
	for _, e := range exposures {
		if e.Kind != models.VulnExposurePlugin {
			continue
		}
		if bySite[e.SiteID] == nil {
			bySite[e.SiteID] = map[string]bool{}
		}
		bySite[e.SiteID][e.Slug] = true
	}
	return bySite
}

// withVulnerableBoost raises a plugin update job's priority to high when
// one of its plugins is vulnerable on the site: a security fix doesn't wait
// behind routine updates.
func withVulnerableBoost(p models.UpdateJobPriority, plugins []string, vulnerable map[string]bool) models.UpdateJobPriority {
	if p >= models.UpdateJobPriorityHigh {
		return p
	}
	for _, name := range plugins {
		if vulnerable[name] {
			return models.UpdateJobPriorityHigh
		}
	}
	return p
}

// SetUpdateJobPriorityHandler changes the priority of a queued job, e.g. to
// run it next. Urgent needs the admin level.
//
//	PATCH /api/update-jobs/{id}
//	{"priority": "high"}
func SetUpdateJobPriorityHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid job ID")
		return
	}
	var body struct {
		Priority string `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	p, err := parseJobPriority(r, body.Priority)
	if err != nil {
		writePriorityError(w, err)
		return
	}
	if err := updatejobs.SetPriority(id, p); errors.Is(err, updatejobs.ErrJobNotQueued) {
		WriteError(w, http.StatusConflict, "The job is no longer queued")
		return
	} else if err != nil {
		verb.LogPrintf(verb.Normal, "SetUpdateJobPriorityHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	verb.LogPrintf(verb.Normal, "%s set the priority of update job %d to %s", getUsername(r), id, p)

	job, err := db.GetUpdateJob(id)
	if err != nil || job == nil {
		verb.LogPrintf(verb.Normal, "SetUpdateJobPriorityHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	jobs := []models.UpdateJob{*job}
	updatejobs.WithPositions(jobs)
	WriteJSON(w, http.StatusOK, jobs[0])
}
