package updatejobs

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

const testPassword = "correct-horse-battery-staple"

// setupUserTest is setupRunnerTest with the user job seams stubbed: every
// site is a single site and refreshes count in *refreshes.
func setupUserTest(t *testing.T) *int {
	t.Helper()
	setupRunnerTest(t)
	oldCreate, oldCheck, oldDelete, oldRefresh, oldMultisite := createUser, checkUserDelete, deleteUser, refreshUsers, siteMultisite
	refreshes := 0
	refreshUsers = func(models.CliSite) error { refreshes++; return nil }
	siteMultisite = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() {
		createUser, checkUserDelete, deleteUser, refreshUsers, siteMultisite = oldCreate, oldCheck, oldDelete, oldRefresh, oldMultisite
	})
	return &refreshes
}

func userCreateJob(t *testing.T, password string) models.UpdateJob {
	t.Helper()
	job := models.NewUpdateJob(models.UpdateJobKindUserCreate, testSite, models.UserCreatePayload{Login: "jco", Email: "jco@example.com", Role: "administrator"})
	job.CreatedBy = "alice"
	if password == "" {
		if err := db.CreateUpdateJob(&job); err != nil {
			t.Fatal(err)
		}
		return job
	}
	if err := EnqueueUserCreate(&job, password); err != nil {
		t.Fatal(err)
	}
	return job
}

// assertNoPassword fails if the stored job or ledger mentions the password.
func assertNoPassword(t *testing.T, id int64) {
	t.Helper()
	stored, err := json.Marshal(storedJob(t, id))
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := db.GetSiteUpdateLedger(testSite)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ledger {
		stored = append(stored, e.DataJSON...)
	}
	if strings.Contains(string(stored), testPassword) {
		t.Errorf("the password was stored: %s", stored)
	}
}

func TestRunUserCreate(t *testing.T) {
	refreshes := setupUserTest(t)
	var got []string
	createUser = func(_ models.CliSite, login, email, role, password string) (int, error) {
		got = []string{login, email, role, password}
		return 42, nil
	}

	job := userCreateJob(t, testPassword)
	run(job)

	if strings.Join(got, " ") != "jco jco@example.com administrator "+testPassword {
		t.Errorf("created %v", got)
	}
	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobDone || len(stored.Results) != 1 || stored.Results[0].Status != models.UpdateDone || stored.Results[0].Note != "User ID 42" {
		t.Errorf("stored job = %+v", stored)
	}
	if *refreshes != 1 {
		t.Errorf("users refreshed %d times, want 1", *refreshes)
	}
	entry, data := actionLedger(t)
	if entry.UpdateType != "user" || entry.Status != models.LedgerCreated || data["login"] != "jco" || data["role"] != "administrator" || entry.UpdatedBy != "alice" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
	if _, ok := jobPassword(job.ID); ok {
		t.Error("the password is still held after the job ran")
	}
	assertNoPassword(t, job.ID)
}

func TestRunUserCreateFails(t *testing.T) {
	setupUserTest(t)
	createUser = func(models.CliSite, string, string, string, string) (int, error) {
		return 0, errors.New("Error: Sorry, that username already exists!")
	}

	job := userCreateJob(t, testPassword)
	run(job)

	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobDone || stored.Results[0].Status != models.UpdateFailed || !strings.Contains(stored.Results[0].Error, "already exists") {
		t.Errorf("stored job = %+v", stored)
	}
	if entry, _ := actionLedger(t); entry.Status != models.LedgerFailed {
		t.Errorf("ledger status = %q, want failed", entry.Status)
	}
	assertNoPassword(t, job.ID)
}

// A job queued before a restart has lost its password and must not run.
func TestRunUserCreateWithoutPassword(t *testing.T) {
	setupUserTest(t)
	createUser = func(models.CliSite, string, string, string, string) (int, error) {
		t.Error("created a user without its password")
		return 0, nil
	}

	job := userCreateJob(t, "")
	run(job)

	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobFailed || stored.Error != passwordLostReason || stored.Results[0].Name != "jco" {
		t.Errorf("stored job = %+v", stored)
	}
}

func userDeleteJob(t *testing.T, reassign string) models.UpdateJob {
	t.Helper()
	job := models.NewUpdateJob(models.UpdateJobKindUserDelete, testSite, models.UserDeletePayload{UserID: 7, Login: "mainwp", Email: "mainwp@example.com", Reassign: reassign})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestRunUserDelete(t *testing.T) {
	refreshes := setupUserTest(t)
	checkUserDelete = func(_ models.CliSite, userID int, reassign string) (wpcli.UserDeleteCheck, error) {
		if userID != 7 || reassign != "admin" {
			t.Errorf("checked user %d reassigning to %q", userID, reassign)
		}
		return wpcli.UserDeleteCheck{Exists: true, Login: "mainwp", Email: "MainWP@example.com", Posts: 3, ReassignIDs: []int{1}}, nil
	}
	var deleted []int
	deleteUser = func(_ models.CliSite, userID, reassignTo int) error {
		deleted = []int{userID, reassignTo}
		return nil
	}

	job := userDeleteJob(t, "admin")
	run(job)

	if len(deleted) != 2 || deleted[0] != 7 || deleted[1] != 1 {
		t.Errorf("deleted %v, want user 7 reassigning to 1", deleted)
	}
	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobDone || stored.Results[0].Status != models.UpdateDone || stored.Results[0].Note != "3 posts reassigned to admin" {
		t.Errorf("stored job = %+v", stored)
	}
	if *refreshes != 1 {
		t.Errorf("users refreshed %d times, want 1", *refreshes)
	}
	entry, data := actionLedger(t)
	if entry.UpdateType != "user" || entry.Status != models.LedgerDeleted || data["reassign"] != "admin" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
}

func TestRunUserDeleteRefusesMultisite(t *testing.T) {
	setupUserTest(t)
	siteMultisite = func(string) (bool, error) { return true, nil }
	checkUserDelete = func(models.CliSite, int, string) (wpcli.UserDeleteCheck, error) {
		t.Error("checked a user on a multisite")
		return wpcli.UserDeleteCheck{}, nil
	}

	job := userDeleteJob(t, "")
	run(job)

	if stored := storedJob(t, job.ID); stored.Status != models.UpdateJobFailed || stored.Error != multisiteDeleteRefusal {
		t.Errorf("stored job = %+v", stored)
	}
}

func TestDeleteProblem(t *testing.T) {
	p := models.UserDeletePayload{UserID: 7, Login: "mainwp", Email: "mainwp@example.com"}
	same := wpcli.UserDeleteCheck{Exists: true, Login: "mainwp", Email: "mainwp@example.com"}
	withReassign := func(r string) models.UserDeletePayload { q := p; q.Reassign = r; return q }
	withCheck := func(f func(*wpcli.UserDeleteCheck)) wpcli.UserDeleteCheck { c := same; f(&c); return c }

	tests := []struct {
		name       string
		payload    models.UserDeletePayload
		check      wpcli.UserDeleteCheck
		reassignTo int
		problem    string
	}{
		{"no posts", p, same, 0, ""},
		{"gone", p, wpcli.UserDeleteCheck{}, 0, "no longer exists"},
		{"other login", p, withCheck(func(c *wpcli.UserDeleteCheck) { c.Login = "editor" }), 0, "is now editor"},
		{"other email", p, withCheck(func(c *wpcli.UserDeleteCheck) { c.Email = "x@example.com" }), 0, "list the users again"},
		{"posts without reassign", p, withCheck(func(c *wpcli.UserDeleteCheck) { c.Posts = 2 }), 0, "has 2 posts"},
		{"reassign", withReassign("admin"), withCheck(func(c *wpcli.UserDeleteCheck) { c.Posts = 2; c.ReassignIDs = []int{1} }), 1, ""},
		{"reassign unknown", withReassign("nobody"), same, 0, "no user with the login or email nobody"},
		{"reassign ambiguous", withReassign("a@example.com"), withCheck(func(c *wpcli.UserDeleteCheck) { c.ReassignIDs = []int{1, 2} }), 0, "matches 2 users"},
		{"reassign to self", withReassign("mainwp"), withCheck(func(c *wpcli.UserDeleteCheck) { c.ReassignIDs = []int{7} }), 0, "user being deleted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reassignTo, problem := deleteProblem(tt.payload, tt.check)
			if reassignTo != tt.reassignTo || (tt.problem == "") != (problem == "") || !strings.Contains(problem, tt.problem) {
				t.Errorf("deleteProblem = %d, %q; want %d, %q", reassignTo, problem, tt.reassignTo, tt.problem)
			}
		})
	}
}
