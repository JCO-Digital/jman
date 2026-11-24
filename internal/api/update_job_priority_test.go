package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
)

func setupPriorityTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = dir
	if err := db.InitInventory(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		config.RunData.DataDir = oldDataDir
	})
}

func requestAs(req *http.Request, level config.UserLevel) *http.Request {
	return req.WithContext(contextWithClaims(req.Context(), &AuthClaims{Username: "alice", Level: level}))
}

func TestParseJobPriority(t *testing.T) {
	req := httptest.NewRequest(http.MethodPatch, "/", nil)
	if p, err := parseJobPriority(requestAs(req, config.LevelExecute), "high"); err != nil || p != models.UpdateJobPriorityHigh {
		t.Errorf("high = %v, %v", p, err)
	}
	if _, err := parseJobPriority(requestAs(req, config.LevelExecute), "urgent"); !errors.Is(err, errUrgentNeedsAdmin) {
		t.Errorf("urgent as execute = %v, want errUrgentNeedsAdmin", err)
	}
	if p, err := parseJobPriority(requestAs(req, config.LevelAdmin), "urgent"); err != nil || p != models.UpdateJobPriorityUrgent {
		t.Errorf("urgent as admin = %v, %v", p, err)
	}
	if _, err := parseJobPriority(requestAs(req, config.LevelAdmin), "asap"); err == nil {
		t.Error("asap was accepted")
	}
}

func TestDefaultJobPriority(t *testing.T) {
	if p := defaultJobPriority(1); p != models.UpdateJobPriorityHigh {
		t.Errorf("one site = %v, want high", p)
	}
	if p := defaultJobPriority(3); p != models.UpdateJobPriorityNormal {
		t.Errorf("three sites = %v, want normal", p)
	}
}

func TestVulnerablePluginsRaisePriority(t *testing.T) {
	setupPriorityTestDB(t)
	site := "11111111-1111-1111-1111-111111111111"
	now := time.Now()
	for _, e := range []models.VulnExposure{
		{Kind: models.VulnExposurePlugin, Slug: "contact-form-7", SiteID: site, Version: "5.0", FixState: models.VulnFixAvailable, FirstSeenAt: now, LastSeenAt: now},
		{Kind: models.VulnExposureCore, Slug: models.CoreExposureSlug, SiteID: site, Version: "6.0", FixState: models.VulnFixAvailable, FirstSeenAt: now, LastSeenAt: now},
	} {
		if err := db.InsertVulnExposure(&e); err != nil {
			t.Fatal(err)
		}
	}
	vulnerable := vulnerablePluginsBySite()
	if !vulnerable[site]["contact-form-7"] || vulnerable[site][models.CoreExposureSlug] {
		t.Fatalf("vulnerable plugins = %v, want contact-form-7 only", vulnerable)
	}

	normal := models.UpdateJobPriorityNormal
	if p := withVulnerableBoost(normal, []string{"akismet", "contact-form-7"}, vulnerable[site]); p != models.UpdateJobPriorityHigh {
		t.Errorf("with a vulnerable plugin = %v, want high", p)
	}
	if p := withVulnerableBoost(normal, []string{"akismet"}, vulnerable[site]); p != normal {
		t.Errorf("without one = %v, want normal", p)
	}
	if p := withVulnerableBoost(models.UpdateJobPriorityUrgent, []string{"contact-form-7"}, vulnerable[site]); p != models.UpdateJobPriorityUrgent {
		t.Errorf("urgent job = %v, want it kept urgent", p)
	}
}

func patchPriority(t *testing.T, id string, body string, level config.UserLevel) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/update-jobs/"+id, strings.NewReader(body))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	SetUpdateJobPriorityHandler(rec, requestAs(req, level))
	return rec
}

func TestSetUpdateJobPriorityHandler(t *testing.T) {
	setupPriorityTestDB(t)
	jobs := []models.UpdateJob{
		models.NewUpdateJob(models.UpdateJobKindCore, "11111111-1111-1111-1111-111111111111", models.CorePayload{Target: "minor"}),
		models.NewUpdateJob(models.UpdateJobKindCore, "22222222-2222-2222-2222-222222222222", models.CorePayload{Target: "minor"}),
	}
	// The runner isn't started, so the jobs stay queued.
	if _, err := updatejobs.EnqueueAll(jobs); err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(jobs[1].ID, 10)

	// Urgent, so it is first even if other tests left jobs in the queue.
	rec := patchPriority(t, id, `{"priority": "urgent"}`, config.LevelAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got models.UpdateJob
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Priority != models.UpdateJobPriorityUrgent || got.Position != 1 {
		t.Errorf("job = priority %v, position %d; want urgent, first in the queue", got.Priority, got.Position)
	}
	if !strings.Contains(rec.Body.String(), `"priority":"urgent"`) {
		t.Errorf("priority isn't encoded by name: %s", rec.Body.String())
	}

	if rec := patchPriority(t, id, `{"priority": "high"}`, config.LevelExecute); rec.Code != http.StatusOK {
		t.Errorf("high as execute: status %d (%s)", rec.Code, rec.Body.String())
	}

	for _, c := range []struct {
		name, id, body string
		level          config.UserLevel
		want           int
	}{
		{"urgent below admin", id, `{"priority": "urgent"}`, config.LevelExecute, http.StatusForbidden},
		{"unknown priority", id, `{"priority": "asap"}`, config.LevelAdmin, http.StatusBadRequest},
		{"no such job", "999999", `{"priority": "high"}`, config.LevelExecute, http.StatusConflict},
		{"bad id", "x", `{"priority": "high"}`, config.LevelExecute, http.StatusBadRequest},
	} {
		if rec := patchPriority(t, c.id, c.body, c.level); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}
}
