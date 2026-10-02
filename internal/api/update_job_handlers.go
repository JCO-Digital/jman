package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/verb"
)

// recentJobWindow is how long a finished job keeps being returned by the
// job list, so clients polling for it see its result.
const recentJobWindow = 10 * time.Minute

// maxPluginsPerJob bounds one `wp plugin update` call.
const maxPluginsPerJob = 50

// CreatePluginUpdateJobsHandler queues background plugin updates: one job
// per site, each updating all of its listed plugins in one WP-CLI call.
//
//	POST /api/plugin-update-jobs
//	{"jobs": [{"site_id": "<uuid>", "plugins": ["akismet", "jetpack"]}]}
func CreatePluginUpdateJobsHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Jobs []struct {
			SiteID  FlexID   `json:"site_id"`
			Plugins []string `json:"plugins"`
		} `json:"jobs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.Jobs) == 0 {
		WriteError(w, http.StatusBadRequest, "At least one job is required")
		return
	}

	// Validate the whole request before queueing anything.
	jobs := make([]models.UpdateJob, 0, len(body.Jobs))
	seenSites := map[string]bool{}
	for _, req := range body.Jobs {
		siteID, err := resolveSiteUUID(string(req.SiteID))
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if seenSites[siteID] {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Site %s is listed more than once", siteID))
			return
		}
		seenSites[siteID] = true

		plugins, err := uniquePluginSlugs(req.Plugins)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		jobs = append(jobs, models.UpdateJob{
			Kind:      models.UpdateJobKindPlugins,
			SiteID:    siteID,
			Plugins:   withInstalledVersions(siteID, plugins),
			CreatedBy: getUsername(r),
		})
	}

	sites, err := cache.GetFastSiteList()
	if err != nil {
		verb.LogPrintf(verb.Normal, "CreatePluginUpdateJobsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to load site list")
		return
	}
	reachable := make(map[string]bool, len(sites))
	for _, s := range sites {
		reachable[s.ID] = true
	}
	for _, job := range jobs {
		if !reachable[job.SiteID] {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("site %s not found or not reachable via WP-CLI", job.SiteID))
			return
		}
	}

	for i := range jobs {
		if err := updatejobs.Enqueue(&jobs[i]); err != nil {
			verb.LogPrintf(verb.Normal, "CreatePluginUpdateJobsHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Failed to queue plugin updates")
			return
		}
	}
	WriteJSON(w, http.StatusCreated, jobs)
}

// ListUpdateJobsHandler returns every queued or running update job (plugin
// and core), plus jobs that finished in the last few minutes.
func ListUpdateJobsHandler(w http.ResponseWriter, r *http.Request) {
	jobs, err := db.ListUpdateJobs(time.Now().Add(-recentJobWindow))
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListUpdateJobsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, jobs)
}

// GetUpdateJobHandler returns one update job.
func GetUpdateJobHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid job ID")
		return
	}
	job, err := db.GetUpdateJob(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "GetUpdateJobHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if job == nil {
		WriteError(w, http.StatusNotFound, "Job not found")
		return
	}
	WriteJSON(w, http.StatusOK, job)
}

func uniquePluginSlugs(plugins []string) ([]string, error) {
	if len(plugins) == 0 {
		return nil, fmt.Errorf("Each job needs at least one plugin")
	}
	seen := map[string]bool{}
	unique := make([]string, 0, len(plugins))
	for _, p := range plugins {
		if !pluginSlugRegex.MatchString(p) {
			return nil, fmt.Errorf("Invalid plugin slug %q: must start with a letter or digit and contain only [a-z0-9_-/]", p)
		}
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}
	if len(unique) > maxPluginsPerJob {
		return nil, fmt.Errorf("At most %d plugins per site", maxPluginsPerJob)
	}
	return unique, nil
}

// withInstalledVersions pairs each plugin with its cached installed version,
// so the job can tell afterwards whether the version changed.
func withInstalledVersions(siteID string, plugins []string) []models.PluginUpdateRequest {
	installed := map[string]string{}
	if cached, err := db.GetSitePlugins(siteID); err == nil {
		for _, p := range cached {
			installed[p.Name] = p.Version
		}
	}
	reqs := make([]models.PluginUpdateRequest, len(plugins))
	for i, p := range plugins {
		reqs[i] = models.PluginUpdateRequest{Name: p, OldVersion: installed[p]}
	}
	return reqs
}
