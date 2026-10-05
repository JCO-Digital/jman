package updatejobs

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

const interruptedActionReason = "jman-api restarted while this change was running; check the site's plugins"

// Seams for tests.
var (
	pluginAction  = wpcli.PluginAction
	installPlugin = wpcli.AddPlugin
	uploadFile    = wpcli.UploadFile
	runSSH        = wpcli.RunSSH
)

// UploadDir is where uploaded plugin ZIPs wait for their install job.
func UploadDir() string {
	return filepath.Join(config.RunData.DataDir, "plugin-uploads")
}

// isPluginManagementJob reports whether a job activates, deactivates,
// deletes, uninstalls or installs plugins (as opposed to updating them).
func isPluginManagementJob(job models.UpdateJob) bool {
	return models.IsPluginActionKind(job.Kind) || job.Kind == models.UpdateJobKindInstall
}

// actionTargets is what a plugin management job reports results for: its
// plugins, or for an install job its source.
func actionTargets(job models.UpdateJob) []models.PluginUpdateRequest {
	if job.Kind == models.UpdateJobKindInstall {
		return []models.PluginUpdateRequest{{Name: job.Source}}
	}
	return job.Plugins
}

// runPluginAction activates, deactivates, deletes or uninstalls the job's
// plugins with one WP-CLI call, then checks each plugin's resulting state
// in the refreshed plugin cache.
func runPluginAction(job models.UpdateJob, site models.CliSite) {
	names := make([]string, len(job.Plugins))
	for i, p := range job.Plugins {
		names[i] = p.Name
	}
	actionErr := pluginAction(site, job.Kind, names)
	jobErr := ""
	if actionErr != nil {
		jobErr = actionErr.Error()
		log.Printf("Plugin %s job %d on %s: %v", job.Kind, job.ID, site.Name, actionErr)
	}

	cacheErr := refreshPluginCache(site)
	if cacheErr != nil {
		log.Printf("Plugin %s job %d: failed to refresh plugin cache for %s: %v", job.Kind, job.ID, site.Name, cacheErr)
	}

	results := actionResults(job, actionErr, cacheErr == nil)
	finishAction(job, models.UpdateJobDone, results, jobErr)
	writeActionLedger(job, results, jobErr)
}

// actionResults works out each plugin's outcome. With a fresh plugin cache
// the plugin's actual state decides, since WP-CLI may have acted on some
// plugins of a batch before failing; otherwise the call's error does.
func actionResults(job models.UpdateJob, actionErr error, cacheFresh bool) []models.UpdateResult {
	var status map[string]string
	if cacheFresh {
		if plugins, err := db.GetSitePlugins(job.SiteID); err == nil {
			status = make(map[string]string, len(plugins))
			for _, p := range plugins {
				status[p.Name] = p.Status
			}
		}
	}

	results := make([]models.UpdateResult, len(job.Plugins))
	for i, p := range job.Plugins {
		r := models.UpdateResult{Name: p.Name, OldVersion: p.OldVersion, NewVersion: p.OldVersion, Status: models.UpdateDone}
		var problem string
		if status == nil {
			if actionErr != nil {
				problem = actionErr.Error()
			}
		} else {
			problem = stateProblem(job.Kind, status, p.Name)
			if problem != "" && actionErr != nil {
				problem = actionErr.Error()
			}
		}
		if problem != "" {
			r.Status = models.UpdateFailed
			r.Error = problem
		}
		results[i] = r
	}
	return results
}

// stateProblem describes why a plugin's state after the action isn't the
// expected one, or returns "" if it is.
func stateProblem(kind string, status map[string]string, name string) string {
	st, installed := status[name]
	switch kind {
	case models.UpdateJobKindActivate:
		if !installed {
			return "plugin is not installed"
		}
		if st != "active" && st != "active-network" {
			return fmt.Sprintf("plugin is still %s", st)
		}
	case models.UpdateJobKindDeactivate:
		if !installed {
			return "plugin is not installed"
		}
		if st != "inactive" {
			return fmt.Sprintf("plugin is still %s", st)
		}
	case models.UpdateJobKindDelete, models.UpdateJobKindUninstall:
		if installed {
			return "plugin is still installed"
		}
	}
	return ""
}

// runInstall installs the job's source, uploading it first if it's a ZIP
// uploaded to jman-api, and reports the installed plugin.
func runInstall(job models.UpdateJob, site models.CliSite) {
	source := job.Source
	if job.UploadPath != "" {
		if site.SSH == "" {
			source = job.UploadPath
		} else {
			// The .zip suffix makes WP-CLI install it as a ZIP (with --force).
			remote := fmt.Sprintf("/tmp/jman-plugin-%d.zip", job.ID)
			if err := uploadFile(site.SSH, job.UploadPath, remote); err != nil {
				failInstall(job, err.Error())
				return
			}
			defer func() {
				if _, err := runSSH(site.SSH, "rm", "-f", remote); err != nil {
					log.Printf("Plugin install job %d: failed to remove %s on %s: %v", job.ID, remote, site.Name, err)
				}
			}()
			source = remote
		}
	}

	before := map[string]bool{}
	if plugins, err := db.GetSitePlugins(job.SiteID); err == nil {
		for _, p := range plugins {
			before[p.Name] = true
		}
	}

	ok, installErr := installPlugin(site, source, job.Activate)
	if installErr == nil && !ok {
		installErr = fmt.Errorf("WP-CLI did not report a successful install")
	}

	cacheErr := refreshPluginCache(site)
	if cacheErr != nil {
		log.Printf("Plugin install job %d: failed to refresh plugin cache for %s: %v", job.ID, site.Name, cacheErr)
	}

	r := models.UpdateResult{Name: job.Source, Status: models.UpdateDone}
	if cacheErr == nil {
		if p := installedPlugin(job, before); p != nil {
			r.Name = p.Name
			r.NewVersion = p.Version
		}
	}
	jobErr := ""
	if installErr != nil {
		jobErr = installErr.Error()
		r.Status = models.UpdateFailed
		r.Error = jobErr
		log.Printf("Plugin install job %d on %s: %v", job.ID, site.Name, installErr)
	}

	results := []models.UpdateResult{r}
	finishAction(job, models.UpdateJobDone, results, jobErr)
	writeActionLedger(job, results, jobErr)
}

// installedPlugin finds the plugin an install job installed in the
// refreshed cache: the slug it named, or a plugin that wasn't there before.
func installedPlugin(job models.UpdateJob, before map[string]bool) *models.WPPlugin {
	plugins, err := db.GetSitePlugins(job.SiteID)
	if err != nil {
		return nil
	}
	for i, p := range plugins {
		if p.Name == job.Source {
			return &plugins[i]
		}
	}
	for i, p := range plugins {
		if !before[p.Name] && p.Status != "must-use" && p.Status != "dropin" {
			return &plugins[i]
		}
	}
	return nil
}

func failInstall(job models.UpdateJob, msg string) {
	results := failAll(actionTargets(job), msg)
	finishAction(job, models.UpdateJobFailed, results, msg)
	writeActionLedger(job, results, msg)
}

func finishAction(job models.UpdateJob, status string, results []models.UpdateResult, errMsg string) {
	if err := db.FinishUpdateJob(job.ID, status, results, nil, errMsg); err != nil {
		log.Printf("Plugin %s job %d: failed to record result: %v", job.Kind, job.ID, err)
	}
}

// actionLedgerStatus is the ledger status of a plugin management job whose
// plugins all ended up in the expected state.
var actionLedgerStatus = map[string]string{
	models.UpdateJobKindActivate:   models.LedgerActivated,
	models.UpdateJobKindDeactivate: models.LedgerDeactivated,
	models.UpdateJobKindDelete:     models.LedgerDeleted,
	models.UpdateJobKindUninstall:  models.LedgerDeleted,
	models.UpdateJobKindInstall:    models.LedgerInstalled,
}

// writeActionLedger records a finished plugin management job in the site
// update ledger: the action's status (e.g. "activated") if every plugin
// ended up in the expected state, "failed" if none did, otherwise
// "partial".
func writeActionLedger(job models.UpdateJob, results []models.UpdateResult, jobErr string) {
	succeeded := 0
	entries := make([]map[string]any, len(results))
	for i, r := range results {
		entry := map[string]any{"plugin": r.Name, "status": "success"}
		if r.NewVersion != "" {
			entry["version"] = r.NewVersion
		}
		if r.Status == models.UpdateFailed {
			entry["status"] = "failed"
			entry["error"] = r.Error
		} else {
			succeeded++
		}
		entries[i] = entry
	}
	status := models.LedgerPartial
	switch succeeded {
	case len(results):
		status = actionLedgerStatus[job.Kind]
	case 0:
		status = models.LedgerFailed
	}

	data := map[string]any{"action": job.Kind, "plugins": entries}
	if job.Kind == models.UpdateJobKindInstall {
		data["source"] = job.Source
		data["activate"] = job.Activate
	}
	if jobErr != "" {
		data["error"] = jobErr
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("Plugin %s job %d: failed to encode ledger entry: %v", job.Kind, job.ID, err)
		return
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     job.SiteID,
		UpdateType: "plugin",
		Status:     status,
		DataJSON:   string(encoded),
		UpdatedBy:  ledgerUser(job),
	}); err != nil {
		log.Printf("Plugin %s job %d: failed to write ledger entry: %v", job.Kind, job.ID, err)
	}
}

func removeUpload(job models.UpdateJob) {
	if err := os.Remove(job.UploadPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Plugin install job %d: failed to remove upload %s: %v", job.ID, job.UploadPath, err)
	}
}

// cleanupUploads removes uploaded ZIPs that no queued or running job needs,
// e.g. those of jobs interrupted by a restart.
func cleanupUploads() {
	entries, err := os.ReadDir(UploadDir())
	if err != nil {
		return
	}
	keep, err := db.ListUpdateJobUploadPaths()
	if err != nil {
		log.Printf("Failed to list plugin uploads in use: %v", err)
		return
	}
	for _, e := range entries {
		path := filepath.Join(UploadDir(), e.Name())
		if e.IsDir() || keep[path] {
			continue
		}
		if err := os.Remove(path); err != nil {
			log.Printf("Failed to remove leftover plugin upload %s: %v", path, err)
		}
	}
}
