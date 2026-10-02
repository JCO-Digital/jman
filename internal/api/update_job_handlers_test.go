package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestCreatePluginUpdateJobsValidation(t *testing.T) {
	site := "11111111-1111-1111-1111-111111111111"
	cases := []struct {
		name, body string
	}{
		{"no jobs", `{"jobs": []}`},
		{"no plugins", `{"jobs": [{"site_id": "` + site + `", "plugins": []}]}`},
		{"bad slug", `{"jobs": [{"site_id": "` + site + `", "plugins": ["../evil"]}]}`},
		{"duplicate site", `{"jobs": [{"site_id": "` + site + `", "plugins": ["a"]}, {"site_id": "` + site + `", "plugins": ["b"]}]}`},
		{"malformed", `{"jobs": `},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		CreatePluginUpdateJobsHandler(rec, httptest.NewRequest(http.MethodPost, "/api/plugin-update-jobs", strings.NewReader(c.body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (body %s)", c.name, rec.Code, rec.Body.String())
		}
	}
}

func TestUniquePluginSlugsDeduplicates(t *testing.T) {
	got, err := uniquePluginSlugs([]string{"a", "b", "a"})
	if err != nil || len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("uniquePluginSlugs = %v, %v", got, err)
	}
}

func coreUpdateRequest(site, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site+"/core-update", strings.NewReader(body))
	req.SetPathValue("id", site)
	return req
}

func TestSiteCoreUpdateValidation(t *testing.T) {
	site := "11111111-1111-1111-1111-111111111111"
	for _, body := range []string{`{"target": "latest"}`, `{"target": ""}`, `{`} {
		rec := httptest.NewRecorder()
		SiteCoreUpdateHandler(rec, coreUpdateRequest(site, body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status %d, want 400", body, rec.Code)
		}
	}
}

func TestSiteCoreUpdateRejectsDuplicateJob(t *testing.T) {
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

	site := "11111111-1111-1111-1111-111111111111"
	existing := models.UpdateJob{Kind: models.UpdateJobKindCore, SiteID: site, Target: "minor"}
	if err := db.CreateUpdateJob(&existing); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	SiteCoreUpdateHandler(rec, coreUpdateRequest(site, `{"target": "major"}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("status %d, want 409 (body %s)", rec.Code, rec.Body.String())
	}

	// Once the existing job finishes, a new one is no longer a duplicate;
	// the site isn't in the (empty) inventory, so it's rejected as unknown.
	if err := db.FinishUpdateJob(existing.ID, models.UpdateJobDone, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	SiteCoreUpdateHandler(rec, coreUpdateRequest(site, `{"target": "major"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
}
