package updatejobs

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

const interruptedMuReason = "jman-api restarted while this change was running; check the site's mu-plugins"

func jobMuInstall(job models.UpdateJob) models.MuInstallPayload {
	p, _ := models.DecodePayload[models.MuInstallPayload](job)
	return p
}

// runMuInstall writes the job's upload into the site's mu-plugins as
// payload.File. It's written next to the file under a temporary name and
// then renamed, so WordPress never loads a half-written file.
func runMuInstall(job models.UpdateJob, site models.CliSite) {
	p := jobMuInstall(job)
	if job.UploadPath == "" {
		failMuInstall(job, "the file to install is missing")
		return
	}
	if site.Path == "" {
		failMuInstall(job, "the site's path is unknown")
		return
	}
	dir := path.Join(site.Path, "wp-content", "mu-plugins")
	dest := path.Join(dir, p.File)
	tmp := path.Join(dir, fmt.Sprintf(".%s.jman-%d", p.File, job.ID))

	var err error
	if site.SSH == "" {
		err = writeMuLocal(job.UploadPath, dir, tmp, dest)
	} else {
		err = writeMuSSH(job, site, dir, tmp, dest)
	}

	r := models.UpdateResult{Name: p.File, Status: models.UpdateDone}
	jobErr := ""
	if err != nil {
		jobErr = err.Error()
		r.Status = models.UpdateFailed
		r.Error = jobErr
		log.Printf("Mu-plugin install job %d on %s: %v", job.ID, site.Name, err)
	} else {
		// Must-use plugins are part of the cached plugin list.
		if err := refreshPluginCache(site); err != nil {
			log.Printf("Mu-plugin install job %d: failed to refresh plugin cache for %s: %v", job.ID, site.Name, err)
		}
	}
	finishMuInstall(job, models.UpdateJobDone, r, jobErr)
}

func writeMuSSH(job models.UpdateJob, site models.CliSite, dir, tmp, dest string) error {
	if _, err := runSSH(site.SSH, "mkdir", "-p", dir); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	if err := uploadFile(site.SSH, job.UploadPath, tmp); err != nil {
		return err
	}
	if res, err := runSSH(site.SSH, "mv", "-f", tmp, dest); err != nil {
		if _, rmErr := runSSH(site.SSH, "rm", "-f", tmp); rmErr != nil {
			log.Printf("Mu-plugin install job %d: failed to remove %s on %s: %v", job.ID, tmp, site.Name, rmErr)
		}
		return fmt.Errorf("failed to move the file into place: %w (stderr: %s)", err, res.Error)
	}
	return nil
}

func writeMuLocal(src, dir, tmp, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	if err := os.WriteFile(filepath.FromSlash(tmp), data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(filepath.FromSlash(tmp), filepath.FromSlash(dest)); err != nil {
		_ = os.Remove(filepath.FromSlash(tmp))
		return err
	}
	return nil
}

// failMuInstall finishes a mu-plugin install job that changed nothing.
func failMuInstall(job models.UpdateJob, msg string) {
	r := models.UpdateResult{Name: jobMuInstall(job).File, Status: models.UpdateFailed, Error: msg}
	finishMuInstall(job, models.UpdateJobFailed, r, msg)
}

func finishMuInstall(job models.UpdateJob, status string, r models.UpdateResult, errMsg string) {
	if err := db.FinishUpdateJob(job.ID, status, []models.UpdateResult{r}, nil, errMsg); err != nil {
		log.Printf("Mu-plugin install job %d: failed to record result: %v", job.ID, err)
	}
	writeMuLedger(job, r, errMsg)
}

// writeMuLedger records a finished mu-plugin install job in the site
// update ledger.
func writeMuLedger(job models.UpdateJob, r models.UpdateResult, jobErr string) {
	p := jobMuInstall(job)
	data := map[string]any{"action": job.Kind, "file": p.File, "source": p.Source, "sha256": p.SHA256}
	status := models.LedgerInstalled
	if r.Status == models.UpdateFailed {
		status = models.LedgerFailed
	}
	if jobErr != "" {
		data["error"] = jobErr
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("Mu-plugin install job %d: failed to encode ledger entry: %v", job.ID, err)
		return
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     job.SiteID,
		UpdateType: "mu-plugin",
		Status:     status,
		DataJSON:   string(encoded),
		UpdatedBy:  ledgerUser(job),
	}); err != nil {
		log.Printf("Mu-plugin install job %d: failed to write ledger entry: %v", job.ID, err)
	}
}
