package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// Seams for tests: the live reads below run WP-CLI on the site.
var (
	livePlugins       = wpcli.GetPluginsWithUpdates
	livePluginDetails = wpcli.GetPluginDetails
	livePluginInfo    = func(site models.CliSite, slug string) (*models.PluginInfo, error) {
		return wpcli.GetPluginInfo(site, slug)
	}
	liveUsers = wpcli.GetSiteUsers
)

// liveSite resolves the site of a live read request, answering the
// request itself if it can't.
func liveSite(w http.ResponseWriter, r *http.Request) (*models.CliSite, bool) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	site, err := getCliSite(siteID)
	if err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return nil, false
	}
	return site, true
}

// readLivePlugins reads a site's plugins over WP-CLI and stores them, so
// changes made outside jman since the last read go to the update ledger.
func readLivePlugins(site models.CliSite) ([]models.WPPlugin, error) {
	plugins, err := livePlugins(site)
	if err != nil {
		return nil, err
	}
	if _, err := sitestate.ApplyPlugins(site.ID, plugins, sitestate.SourceSSH, time.Now()); err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to save plugins for site %s: %v\n", site.Name, err)
	}
	cache.UpdatePluginInfoFromPlugins(plugins)
	return plugins, nil
}

// SiteLivePluginsHandler returns all of a site's plugins, read from the
// site over WP-CLI, and stores them like the refresh does.
//
//	GET /api/sites/{id}/plugins/live
func SiteLivePluginsHandler(w http.ResponseWriter, r *http.Request) {
	site, ok := liveSite(w, r)
	if !ok {
		return
	}
	plugins, err := readLivePlugins(*site)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteLivePluginsHandler on %s: %v", site.Name, err)
		WriteError(w, http.StatusBadGateway, "Failed to read plugins from the site")
		return
	}
	if plugins == nil {
		plugins = []models.WPPlugin{}
	}
	WriteJSON(w, http.StatusOK, plugins)
}

// SitePluginDetailsHandler returns the details (title, author, update) of
// every plugin on a site, read from the site over WP-CLI.
//
//	GET /api/sites/{id}/plugins/details
func SitePluginDetailsHandler(w http.ResponseWriter, r *http.Request) {
	site, ok := liveSite(w, r)
	if !ok {
		return
	}
	details, err := livePluginDetails(*site)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SitePluginDetailsHandler on %s: %v", site.Name, err)
		WriteError(w, http.StatusBadGateway, "Failed to read plugin details from the site")
		return
	}
	if details == nil {
		details = []models.PluginDetails{}
	}
	WriteJSON(w, http.StatusOK, details)
}

// SitePluginInfoHandler returns one plugin's info as the site reports it
// over WP-CLI (`wp plugin get`), or 404 if it isn't installed.
//
//	GET /api/sites/{id}/plugins/{slug}/info
func SitePluginInfoHandler(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !utils.IsValidSlug(slug) || filepath.Base(slug) != slug {
		WriteError(w, http.StatusBadRequest, "Invalid plugin slug")
		return
	}
	site, ok := liveSite(w, r)
	if !ok {
		return
	}
	info, err := livePluginInfo(*site, slug)
	if errors.Is(err, wpcli.ErrPluginNotFound) {
		info, err = nil, nil
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "SitePluginInfoHandler on %s: %v", site.Name, err)
		WriteError(w, http.StatusBadGateway, "Failed to read the plugin from the site")
		return
	}
	if info == nil {
		WriteError(w, http.StatusNotFound, "Plugin "+slug+" is not installed on "+site.Name)
		return
	}
	WriteJSON(w, http.StatusOK, info)
}

// SiteLiveUsersHandler returns a site's users, read from the site over
// WP-CLI, and stores them like the refresh does.
//
//	GET /api/sites/{id}/users/live
func SiteLiveUsersHandler(w http.ResponseWriter, r *http.Request) {
	site, ok := liveSite(w, r)
	if !ok {
		return
	}
	users, err := liveUsers(*site)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteLiveUsersHandler on %s: %v", site.Name, err)
		WriteError(w, http.StatusBadGateway, "Failed to read users from the site")
		return
	}
	if _, err := sitestate.ApplyUsers(site.ID, users, sitestate.SourceSSH, time.Now()); err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to save users for site %s: %v\n", site.Name, err)
	}
	WriteJSON(w, http.StatusOK, users)
}

// FileModsHandler queues a job per site that sets DISALLOW_FILE_MODS in
// its wp-config.php.
//
//	POST /api/file-mods
//	{"sites": ["<uuid>", …], "disallow": true}
func FileModsHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sites    []string `json:"sites"`
		Disallow *bool    `json:"disallow"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Disallow == nil {
		WriteError(w, http.StatusBadRequest, `"disallow" is required`)
		return
	}
	sites, err := bulkJobSites(req.Sites)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	by := getUsername(r)
	payload := models.FileModsPayload{Disallow: *req.Disallow}
	jobs := make([]models.UpdateJob, len(sites))
	for i, siteID := range sites {
		jobs[i] = models.NewUpdateJob(models.UpdateJobKindFileMods, siteID, payload)
		jobs[i].CreatedBy = by
		jobs[i].Priority = defaultJobPriority(len(sites))
	}
	n, err := updatejobs.EnqueueAll(jobs)
	if err != nil {
		verb.LogPrintf(verb.Normal, "FileModsHandler: %v", err)
		if n == 0 {
			WriteError(w, http.StatusInternalServerError, "Failed to queue the changes")
			return
		}
	}
	WriteJSON(w, http.StatusAccepted, map[string]any{"jobs": jobs[:n]})
}
