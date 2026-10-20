package commands

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/credentials"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/zalando/go-keyring"
)

// fakeJmanAPI is a jman-api stand-in for the commands that queue jobs.
// It records the write requests it gets, and every job it queues has
// finished by the first poll.
type fakeJmanAPI struct {
	t     *testing.T
	mu    sync.Mutex
	calls []string // "METHOD path body"
	jobs  map[int64]models.UpdateJob
	next  int64
}

const testSiteID = "site-a"

func newFakeJmanAPI(t *testing.T) *httptest.Server {
	f := &fakeJmanAPI{t: t, jobs: map[int64]models.UpdateJob{}, next: 1}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	old := config.Cfg
	oldRun := config.RunData
	oldVersion := config.AppVersion
	dir := t.TempDir()
	config.RunData.DataDir, config.RunData.ConfigDir, config.RunData.CacheDir = dir, dir, dir
	config.Cfg.APIURL = srv.URL
	config.Cfg.CacheTTL = time.Minute
	config.Cfg.APITimeout = 5 * time.Second
	config.AppVersion = "dev"
	keyring.MockInit()
	if _, err := credentials.Set(srv.URL, "jman_cli_test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.Cfg, config.RunData, config.AppVersion = old, oldRun, oldVersion })

	apis[srv.URL] = f
	return srv
}

var apis = map[string]*fakeJmanAPI{}

func (f *fakeJmanAPI) record(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	call := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/api") + " " + strings.TrimSpace(string(body))
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	return string(body)
}

func (f *fakeJmanAPI) queue(w http.ResponseWriter, job models.UpdateJob) {
	f.mu.Lock()
	job.ID = f.next
	f.next++
	job.SiteID = testSiteID
	job.Status = models.UpdateJobQueued
	f.jobs[job.ID] = job
	f.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(job)
}

func (f *fakeJmanAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(cliproto.LatestCLIVersionHeader, "dev")
	if r.Header.Get("Authorization") != "Bearer jman_cli_test" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api")
	sitePath := "/sites/" + testSiteID

	switch {
	case path == "/cli/snapshot":
		_ = json.NewEncoder(w).Encode(cliproto.Snapshot{
			SchemaVersion: 1,
			Sites: []cliproto.Site{{
				ID: testSiteID, Provider: "manual", Domain: "a.example.com", IsWordpress: true, CanWPCLI: true,
				SSHHost: "host", SSHPort: 22, SSHUser: "u", SitePath: "public",
				WPCore:      &models.SiteCore{SiteID: testSiteID, Version: "6.8.2"},
				UpdateLocks: []models.UpdateLock{{SiteID: testSiteID, Plugin: "woocommerce"}},
			}},
			Plugins: []models.WPPlugin{
				{SiteID: testSiteID, Name: "akismet", Status: "active", Version: "5.0", Update: "5.1"},
				{SiteID: testSiteID, Name: "hello", Status: "inactive", Version: "1.7"},
				{SiteID: testSiteID, Name: "woocommerce", Status: "active", Version: "9.0", Update: "10.0"},
			},
		})

	case r.Method == http.MethodGet && path == sitePath+"/plugin-updates":
		f.record(r)
		_ = json.NewEncoder(w).Encode([]models.WPPlugin{
			{SiteID: testSiteID, Name: "akismet", Version: "5.0", Update: "5.1"},
			{SiteID: testSiteID, Name: "woocommerce", Version: "9.0", Update: "10.0"},
		})

	case r.Method == http.MethodPost && path == "/plugin-update-jobs":
		var body struct {
			Jobs []struct {
				Plugins []string `json:"plugins"`
			} `json:"jobs"`
		}
		_ = json.Unmarshal([]byte(f.record(r)), &body)
		var payload models.PluginsPayload
		for _, p := range body.Jobs[0].Plugins {
			payload.Plugins = append(payload.Plugins, models.PluginUpdateRequest{Name: p})
		}
		job := models.NewUpdateJob(models.UpdateJobKindPlugins, "", payload)
		f.mu.Lock()
		job.ID = f.next
		f.next++
		job.SiteID = testSiteID
		f.jobs[job.ID] = job
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode([]models.UpdateJob{job})

	case r.Method == http.MethodPost && path == sitePath+"/plugin-actions":
		var body struct {
			Action  string   `json:"action"`
			Plugins []string `json:"plugins"`
		}
		_ = json.Unmarshal([]byte(f.record(r)), &body)
		f.queue(w, models.NewUpdateJob(body.Action, "", models.PluginsPayload{Plugins: []models.PluginUpdateRequest{{Name: body.Plugins[0]}}}))

	case r.Method == http.MethodPost && path == sitePath+"/plugin-install":
		body := f.record(r)
		if strings.Contains(body, `"akismet"`) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"Plugin akismet is already installed on this site"}`))
			return
		}
		f.queue(w, models.NewUpdateJob(models.UpdateJobKindInstall, "", models.InstallPayload{Source: "jetpack"}))

	case r.Method == http.MethodPost && path == sitePath+"/core-update":
		f.record(r)
		f.queue(w, models.UpdateJob{Kind: models.UpdateJobKindCore})

	case r.Method == http.MethodGet && strings.HasPrefix(path, "/update-jobs/"):
		f.mu.Lock()
		var job models.UpdateJob
		for id, j := range f.jobs {
			if path == "/update-jobs/"+jsonID(id) {
				job = j
			}
		}
		f.mu.Unlock()
		job.Status = models.UpdateJobDone
		payload, _ := models.DecodePayload[models.PluginsPayload](job)
		switch job.Kind {
		case models.UpdateJobKindPlugins:
			for _, p := range payload.Plugins {
				job.Results = append(job.Results, models.UpdateResult{Name: p.Name, OldVersion: "5.0", NewVersion: "5.1", Status: models.UpdateUpdated})
			}
		case models.UpdateJobKindCore:
			job.Results = []models.UpdateResult{{Name: "WordPress", OldVersion: "6.8.2", NewVersion: "6.8.3", Status: models.UpdateUpdated}}
		case models.UpdateJobKindInstall:
			job.Results = []models.UpdateResult{{Name: "jetpack", NewVersion: "14.0", Status: models.UpdateDone}}
		default:
			job.Results = []models.UpdateResult{{Name: payload.Plugins[0].Name, Status: models.UpdateDone}}
		}
		_ = json.NewEncoder(w).Encode(job)

	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func jsonID(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}

// runJman runs a jman command with "y" piped to the site confirmation
// prompt and returns the write requests the fake API received.
func runJman(t *testing.T, srv *httptest.Server, args ...string) []string {
	t.Helper()
	return runJmanWithInput(t, srv, "y\n", args...)
}

// runJmanWithInput is runJman with input on stdin instead of "y".
func runJmanWithInput(t *testing.T, srv *httptest.Server, input string, args ...string) []string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString(input)
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	f := apis[srv.URL]
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()

	rootCmd.SetArgs(args)
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("jman %s: %v", strings.Join(args, " "), err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

func TestPluginUpdateQueuesJob(t *testing.T) {
	srv := newFakeJmanAPI(t)
	calls := runJman(t, srv, "plugin", "update", "a.example.com", "akismet")
	want := []string{
		"GET /sites/site-a/plugin-updates ",
		`POST /plugin-update-jobs {"jobs":[{"allow_major":false,"plugins":["akismet"],"site_id":"site-a"}]}`,
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}

	// Without a plugin name, every plugin with an update is queued.
	calls = runJman(t, srv, "plugin", "update", "a.example.com")
	if len(calls) != 2 || !strings.Contains(calls[1], `"plugins":["akismet","woocommerce"]`) {
		t.Errorf("update all: %v", calls)
	}
}

func TestPluginRemoveDeactivatesFirst(t *testing.T) {
	srv := newFakeJmanAPI(t)
	calls := runJman(t, srv, "plugin", "remove", "a.example.com", "akismet")
	want := []string{
		`POST /sites/site-a/plugin-actions {"action":"deactivate","plugins":["akismet"]}`,
		`POST /sites/site-a/plugin-actions {"action":"uninstall","plugins":["akismet"]}`,
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}

	// An inactive plugin is uninstalled straight away.
	calls = runJman(t, srv, "plugin", "remove", "a.example.com", "hello")
	if len(calls) != 1 || !strings.Contains(calls[0], `"action":"uninstall"`) {
		t.Errorf("inactive plugin: %v", calls)
	}

	// A plugin that isn't installed isn't sent to the API.
	if calls := runJman(t, srv, "plugin", "remove", "a.example.com", "nope"); len(calls) != 0 {
		t.Errorf("missing plugin: %v", calls)
	}
}

func TestPluginInstall(t *testing.T) {
	srv := newFakeJmanAPI(t)
	calls := runJman(t, srv, "plugin", "install", "a.example.com", "jetpack")
	if len(calls) != 1 || calls[0] != `POST /sites/site-a/plugin-install {"activate":true,"source":"jetpack"}` {
		t.Errorf("calls: %v", calls)
	}
	// Already installed (409) is reported, not an error.
	runJman(t, srv, "plugin", "install", "a.example.com", "akismet")
}

func TestCoreUpdate(t *testing.T) {
	srv := newFakeJmanAPI(t)
	defer func() { coreMajor, coreAllowMajor = false, false }()

	calls := runJman(t, srv, "core", "update", "a.example.com")
	if len(calls) != 1 || calls[0] != `POST /sites/site-a/core-update {"allow_major":false,"target":"minor"}` {
		t.Errorf("calls: %v", calls)
	}
	calls = runJman(t, srv, "core", "update", "a.example.com", "--major", "--allow-major")
	if len(calls) != 1 || calls[0] != `POST /sites/site-a/core-update {"allow_major":true,"target":"major"}` {
		t.Errorf("major: %v", calls)
	}
}

func TestYesSkipsConfirmation(t *testing.T) {
	srv := newFakeJmanAPI(t)
	t.Cleanup(func() { flagYes = false })

	// No input at all: without --yes the confirmation can't be read.
	calls := runJmanWithInput(t, srv, "", "--yes", "core", "update", "a.example.com")
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "POST /sites/site-a/core-update") {
		t.Errorf("calls: %v", calls)
	}
}
