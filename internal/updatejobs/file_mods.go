package updatejobs

import (
	"encoding/json"
	"log"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

const interruptedFileModsReason = "jman-api restarted while this change was running; check DISALLOW_FILE_MODS in the site's wp-config.php"

// fileModsName is the result name of file_mods jobs.
const fileModsName = "DISALLOW_FILE_MODS"

// setFileMods sets DISALLOW_FILE_MODS on a site. A seam for tests.
var setFileMods = func(site models.CliSite, disallow bool, rec *[]models.WPCLICall) error {
	_, err := wpcli.RunWP(wpcli.CliOptions{SiteID: site.ID, SSH: site.SSH, Path: site.Path, Timeout: wpcli.WriteTimeout, Record: rec},
		"config", "set", "--raw", fileModsName, strconv.FormatBool(disallow))
	return err
}

func jobFileMods(job models.UpdateJob) models.FileModsPayload {
	p, _ := models.DecodePayload[models.FileModsPayload](job)
	return p
}

// runFileMods sets DISALLOW_FILE_MODS in the site's wp-config.php to the
// job's value and stores the new flag, so jman-api shows it before the
// site's next collection.
func runFileMods(job models.UpdateJob, site models.CliSite) {
	p := jobFileMods(job)
	var calls []models.WPCLICall
	r := models.UpdateResult{Name: fileModsName, Status: models.UpdateDone, Note: "Set to " + strconv.FormatBool(p.Disallow)}
	jobErr := ""
	if err := setFileMods(site, p.Disallow, &calls); err != nil {
		jobErr = err.Error()
		r = models.UpdateResult{Name: fileModsName, Status: models.UpdateFailed, Error: jobErr}
		log.Printf("File mods job %d on %s: %v", job.ID, site.Name, err)
	} else if err := db.SetSiteDisallowFileMods(site.ID, p.Disallow); err != nil {
		log.Printf("File mods job %d: %v", job.ID, err)
	}
	finishFileMods(job, models.UpdateJobDone, r, calls, jobErr)
}

// failFileMods finishes a file_mods job that changed nothing.
func failFileMods(job models.UpdateJob, msg string) {
	r := models.UpdateResult{Name: fileModsName, Status: models.UpdateFailed, Error: msg}
	finishFileMods(job, models.UpdateJobFailed, r, nil, msg)
}

func finishFileMods(job models.UpdateJob, status string, r models.UpdateResult, calls []models.WPCLICall, errMsg string) {
	if len(calls) > 0 {
		if err := db.SetUpdateJobCalls(job.ID, calls); err != nil {
			log.Printf("File mods job %d: failed to store wp-cli output: %v", job.ID, err)
		}
	}
	if err := db.FinishUpdateJob(job.ID, status, []models.UpdateResult{r}, nil, errMsg); err != nil {
		log.Printf("File mods job %d: failed to record result: %v", job.ID, err)
	}
	writeFileModsLedger(job, r, errMsg)
}

// writeFileModsLedger records a finished file_mods job in the site update
// ledger.
func writeFileModsLedger(job models.UpdateJob, r models.UpdateResult, jobErr string) {
	data := map[string]any{"action": job.Kind, "disallow_file_mods": jobFileMods(job).Disallow}
	status := models.LedgerChanged
	if r.Status == models.UpdateFailed {
		status = models.LedgerFailed
	}
	if jobErr != "" {
		data["error"] = jobErr
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("File mods job %d: failed to encode ledger entry: %v", job.ID, err)
		return
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     job.SiteID,
		UpdateType: "config",
		Status:     status,
		DataJSON:   string(encoded),
		UpdatedBy:  ledgerUser(job),
	}); err != nil {
		log.Printf("File mods job %d: failed to write ledger entry: %v", job.ID, err)
	}
}
