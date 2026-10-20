package updatejobs

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

const (
	// passwordLostReason fails a user create job whose password jman-api
	// no longer has, as it was only held in memory.
	passwordLostReason     = "jman-api restarted before the user was created; the password was only held in memory. Create the user again."
	interruptedUserReason  = "jman-api restarted while this change was running; check the site's users"
	multisiteDeleteRefusal = "deleting users on multisite sites isn't supported"
)

// Seams for tests.
var (
	createUser      = wpcli.CreateUser
	checkUserDelete = wpcli.CheckUserDelete
	deleteUser      = wpcli.DeleteUser
	// refreshUsers is the refresh after a user job, so the user lists show
	// the change without waiting for the next collection.
	refreshUsers = func(site models.CliSite) error {
		return cache.RefreshSiteUsersFrom(site, sitestate.SourceJob)
	}
	siteMultisite = func(siteID string) (bool, error) {
		flags, err := db.GetAllSiteWpFlags()
		if err != nil {
			return false, err
		}
		return flags[siteID].IsMultisite, nil
	}
)

// passwords holds the passwords of queued user create jobs by job ID. They
// are never stored: a job whose password is gone (jman-api restarted)
// fails with passwordLostReason.
var passwords = struct {
	sync.Mutex
	byJob map[int64]string
}{byJob: map[int64]string{}}

// EnqueueUserCreate stores a new user create job and queues it to run,
// keeping password in memory only until the job has run.
func EnqueueUserCreate(job *models.UpdateJob, password string) error {
	if err := db.CreateUpdateJob(job); err != nil {
		return err
	}
	passwords.Lock()
	passwords.byJob[job.ID] = password
	passwords.Unlock()
	q.push(*job)
	return nil
}

func jobPassword(id int64) (string, bool) {
	passwords.Lock()
	defer passwords.Unlock()
	p, ok := passwords.byJob[id]
	return p, ok
}

func forgetPassword(id int64) {
	passwords.Lock()
	delete(passwords.byJob, id)
	passwords.Unlock()
}

func jobUserCreate(job models.UpdateJob) models.UserCreatePayload {
	p, _ := models.DecodePayload[models.UserCreatePayload](job)
	return p
}

func jobUserDelete(job models.UpdateJob) models.UserDeletePayload {
	p, _ := models.DecodePayload[models.UserDeletePayload](job)
	return p
}

// userLogin is the login a user job acts on, its result's name.
func userLogin(job models.UpdateJob) string {
	if job.Kind == models.UpdateJobKindUserCreate {
		return jobUserCreate(job).Login
	}
	return jobUserDelete(job).Login
}

// runUserCreate creates the job's user with the password held for it.
func runUserCreate(job models.UpdateJob, site models.CliSite) {
	p := jobUserCreate(job)
	password, ok := jobPassword(job.ID)
	if !ok {
		failUserJob(job, passwordLostReason)
		return
	}

	r := models.UpdateResult{Name: p.Login, Status: models.UpdateDone}
	id, err := createUser(site, p.Login, p.Email, p.Role, password)
	jobErr := ""
	if err != nil {
		jobErr = err.Error()
		r.Status = models.UpdateFailed
		r.Error = jobErr
		log.Printf("User create job %d on %s: %v", job.ID, site.Name, err)
	} else {
		r.Note = fmt.Sprintf("User ID %d", id)
		refreshUsersAfter(job, site)
	}
	finishUserJob(job, models.UpdateJobDone, r, jobErr)
}

// runUserDelete deletes the job's user after checking on the site that it
// is still the user jman listed and that its content has somewhere to go.
func runUserDelete(job models.UpdateJob, site models.CliSite) {
	p := jobUserDelete(job)
	multisite, err := siteMultisite(job.SiteID)
	if err != nil {
		failUserJob(job, fmt.Sprintf("failed to check whether the site is a multisite: %v", err))
		return
	}
	if multisite {
		failUserJob(job, multisiteDeleteRefusal)
		return
	}

	check, err := checkUserDelete(site, p.UserID, p.Reassign)
	if err != nil {
		failUserJob(job, err.Error())
		return
	}
	reassignTo, problem := deleteProblem(p, check)
	if problem != "" {
		failUserJob(job, problem)
		return
	}

	r := models.UpdateResult{Name: p.Login, Status: models.UpdateDone}
	jobErr := ""
	if err := deleteUser(site, p.UserID, reassignTo); err != nil {
		jobErr = err.Error()
		r.Status = models.UpdateFailed
		r.Error = jobErr
		log.Printf("User delete job %d on %s: %v", job.ID, site.Name, err)
	} else if reassignTo != 0 {
		r.Note = fmt.Sprintf("%d posts reassigned to %s", check.Posts, p.Reassign)
	}
	// Refresh also after a failure: the user may be gone anyway.
	refreshUsersAfter(job, site)
	finishUserJob(job, models.UpdateJobDone, r, jobErr)
}

// deleteProblem checks a user delete against what the site reported, and
// returns the ID of the user to reassign the content to (0 for none) or why
// the user mustn't be deleted.
func deleteProblem(p models.UserDeletePayload, check wpcli.UserDeleteCheck) (int, string) {
	if !check.Exists {
		return 0, fmt.Sprintf("user %d (%s) no longer exists", p.UserID, p.Login)
	}
	if check.Login != p.Login || !strings.EqualFold(check.Email, p.Email) {
		return 0, fmt.Sprintf("user %d is now %s <%s>, not %s <%s>; list the users again", p.UserID, check.Login, check.Email, p.Login, p.Email)
	}
	if p.Reassign == "" {
		if check.Posts > 0 {
			return 0, fmt.Sprintf("%s has %d posts, which deleting the user would delete; give a user to reassign them to", p.Login, check.Posts)
		}
		return 0, ""
	}
	switch len(check.ReassignIDs) {
	case 0:
		return 0, fmt.Sprintf("no user with the login or email %s to reassign the content to", p.Reassign)
	case 1:
	default:
		return 0, fmt.Sprintf("%s matches %d users; give the login or email of one", p.Reassign, len(check.ReassignIDs))
	}
	if check.ReassignIDs[0] == p.UserID {
		return 0, "the content can't be reassigned to the user being deleted"
	}
	return check.ReassignIDs[0], ""
}

func refreshUsersAfter(job models.UpdateJob, site models.CliSite) {
	if err := refreshUsers(site); err != nil {
		log.Printf("User job %d: failed to refresh users of %s: %v", job.ID, site.Name, err)
	}
}

// failUserJob finishes a user job that changed nothing.
func failUserJob(job models.UpdateJob, msg string) {
	r := models.UpdateResult{Name: userLogin(job), Status: models.UpdateFailed, Error: msg}
	finishUserJob(job, models.UpdateJobFailed, r, msg)
}

func finishUserJob(job models.UpdateJob, status string, r models.UpdateResult, errMsg string) {
	if err := db.FinishUpdateJob(job.ID, status, []models.UpdateResult{r}, nil, errMsg); err != nil {
		log.Printf("User job %d: failed to record result: %v", job.ID, err)
	}
	writeUserLedger(job, r, errMsg)
}

// writeUserLedger records a finished user job in the site update ledger.
func writeUserLedger(job models.UpdateJob, r models.UpdateResult, jobErr string) {
	data := map[string]any{"action": job.Kind}
	status := models.LedgerFailed
	if job.Kind == models.UpdateJobKindUserCreate {
		p := jobUserCreate(job)
		data["login"], data["email"], data["role"] = p.Login, p.Email, p.Role
		if r.Status != models.UpdateFailed {
			status = models.LedgerCreated
		}
	} else {
		p := jobUserDelete(job)
		data["login"], data["email"] = p.Login, p.Email
		if p.Reassign != "" {
			data["reassign"] = p.Reassign
		}
		if r.Status != models.UpdateFailed {
			status = models.LedgerDeleted
		}
	}
	if r.Note != "" {
		data["note"] = r.Note
	}
	if jobErr != "" {
		data["error"] = jobErr
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("User job %d: failed to encode ledger entry: %v", job.ID, err)
		return
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     job.SiteID,
		UpdateType: "user",
		Status:     status,
		DataJSON:   string(encoded),
		UpdatedBy:  ledgerUser(job),
	}); err != nil {
		log.Printf("User job %d: failed to write ledger entry: %v", job.ID, err)
	}
}
