package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

const liveTestSite = "33333333-3333-3333-3333-333333333333"

// setupLiveTest has one WP-CLI site, liveTestSite, and stubs the live reads.
func setupLiveTest(t *testing.T) {
	t.Helper()
	setupZipTest(t)
	oldCache := config.RunData.CacheDir
	config.RunData.CacheDir = t.TempDir()
	if err := db.SaveManagedSite(models.ManagedSite{
		ID: liveTestSite, Provider: "manual", Domain: "live.example.org", IsWordpress: true,
		CanWPCLI: true, SSHHost: "host.example.org", SSHUser: "live", SitePath: "files", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	oldPlugins, oldDetails, oldInfo, oldUsers := livePlugins, livePluginDetails, livePluginInfo, liveUsers
	t.Cleanup(func() {
		config.RunData.CacheDir = oldCache
		livePlugins, livePluginDetails, livePluginInfo, liveUsers = oldPlugins, oldDetails, oldInfo, oldUsers
	})
}

func liveRequest(t *testing.T, h http.HandlerFunc, path string, values map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range values {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestSiteLivePluginsHandlerStoresPlugins(t *testing.T) {
	setupLiveTest(t)
	livePlugins = func(site models.CliSite) ([]models.WPPlugin, error) {
		if site.ID != liveTestSite || site.SSH != "live@host.example.org" {
			t.Errorf("read site %+v", site)
		}
		return []models.WPPlugin{{SiteID: site.ID, Name: "akismet", Status: "active", Version: "5.0", Update: "5.1"}}, nil
	}

	rec := liveRequest(t, SiteLivePluginsHandler, "/api/sites/x/plugins/live", map[string]string{"id": liveTestSite})
	var got []models.WPPlugin
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 1 || got[0].Update != "5.1" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	stored, err := db.GetSitePlugins(liveTestSite)
	if err != nil || len(stored) != 1 || stored[0].Name != "akismet" {
		t.Errorf("stored plugins = %+v, %v", stored, err)
	}

	livePlugins = func(models.CliSite) ([]models.WPPlugin, error) { return nil, errors.New("ssh: connect refused") }
	if rec := liveRequest(t, SiteLivePluginsHandler, "/", map[string]string{"id": liveTestSite}); rec.Code != http.StatusBadGateway {
		t.Errorf("failed read: status %d", rec.Code)
	}
	if rec := liveRequest(t, SiteLivePluginsHandler, "/", map[string]string{"id": "44444444-4444-4444-4444-444444444444"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown site: status %d", rec.Code)
	}
}

func TestSitePluginInfoHandler(t *testing.T) {
	setupLiveTest(t)
	livePluginInfo = func(_ models.CliSite, slug string) (*models.PluginInfo, error) {
		switch slug {
		case "akismet":
			return &models.PluginInfo{Slug: slug, Name: "Akismet", Version: "5.1"}, nil
		case "gone":
			return nil, fmt.Errorf("plugin gone: %w", wpcli.ErrPluginNotFound)
		case "broken":
			return nil, errors.New("ssh: connect refused")
		}
		return nil, nil
	}
	for _, c := range []struct {
		slug   string
		status int
	}{
		{"akismet", http.StatusOK},
		{"missing", http.StatusNotFound},
		{"gone", http.StatusNotFound},
		{"broken", http.StatusBadGateway},
		{"..", http.StatusBadRequest},
		{"Upper", http.StatusBadRequest},
	} {
		rec := liveRequest(t, SitePluginInfoHandler, "/", map[string]string{"id": liveTestSite, "slug": c.slug})
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d (%s)", c.slug, rec.Code, c.status, rec.Body.String())
		}
	}
}

func TestSitePluginDetailsHandler(t *testing.T) {
	setupLiveTest(t)
	livePluginDetails = func(models.CliSite) ([]models.PluginDetails, error) {
		return []models.PluginDetails{{Slug: "akismet", Title: "Akismet", Version: "5.0", UpdateVersion: "5.1"}}, nil
	}
	rec := liveRequest(t, SitePluginDetailsHandler, "/", map[string]string{"id": liveTestSite})
	var got []models.PluginDetails
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || len(got) != 1 || got[0].UpdateVersion != "5.1" {
		t.Errorf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSiteLiveUsersHandlerStoresUsers(t *testing.T) {
	setupLiveTest(t)
	liveUsers = func(site models.CliSite) (models.SiteUsers, error) {
		return models.SiteUsers{Users: []models.SiteUser{{SiteID: site.ID, ID: 1, Login: "admin", Roles: "administrator"}}, ExcludedCounts: map[string]int{"subscriber": 2}}, nil
	}
	rec := liveRequest(t, SiteLiveUsersHandler, "/", map[string]string{"id": liveTestSite})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"admin"`) {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	stored, found, err := db.GetSiteUsers(liveTestSite)
	if err != nil || !found || len(stored.Users) != 1 || stored.ExcludedCounts["subscriber"] != 2 {
		t.Errorf("stored users = %+v, %v, %v", stored, found, err)
	}
}

func TestFileModsHandler(t *testing.T) {
	setupLiveTest(t)
	for _, body := range []string{
		`{`,
		`{"sites": ["` + liveTestSite + `"]}`,
		`{"sites": [], "disallow": true}`,
		`{"sites": ["44444444-4444-4444-4444-444444444444"], "disallow": true}`,
	} {
		rec := httptest.NewRecorder()
		FileModsHandler(rec, httptest.NewRequest(http.MethodPost, "/api/file-mods", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", body, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	FileModsHandler(rec, httptest.NewRequest(http.MethodPost, "/api/file-mods",
		strings.NewReader(`{"sites": ["`+liveTestSite+`", "`+liveTestSite+`"], "disallow": false}`)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Jobs []models.UpdateJob `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Jobs) != 1 {
		t.Fatalf("jobs = %+v, %v", resp.Jobs, err)
	}
	job := resp.Jobs[0]
	p, err := models.DecodePayload[models.FileModsPayload](job)
	if job.Kind != models.UpdateJobKindFileMods || job.SiteID != liveTestSite || err != nil || p.Disallow {
		t.Errorf("job = %+v, payload %+v, %v", job, p, err)
	}
	if stored, err := db.GetUpdateJob(job.ID); err != nil || stored == nil || stored.Status != models.UpdateJobQueued {
		t.Errorf("stored job = %+v, %v", stored, err)
	}
}
