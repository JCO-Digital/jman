package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
)

func pluginZip(t *testing.T, slug, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, err := w.Create(slug + "/" + slug + ".php")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("<?php\n/**\n * Plugin Name: Paid\n * Version: " + version + "\n */\n"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// allowZipURLs sets the allowed ZIP URL prefixes for a test.
func allowZipURLs(t *testing.T, prefixes ...string) {
	t.Helper()
	old := zipURLPrefixes
	zipURLPrefixes = func() []string { return prefixes }
	t.Cleanup(func() { zipURLPrefixes = old })
}

func setupZipTest(t *testing.T) {
	t.Helper()
	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = t.TempDir()
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

func stageFromURL(t *testing.T, url string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	StagePluginZipHandler(rec, httptest.NewRequest(http.MethodPost, "/api/plugin-zips", strings.NewReader(`{"url": "`+url+`"}`)))
	return rec
}

func TestStagePluginZipFromURL(t *testing.T) {
	setupZipTest(t)
	body := pluginZip(t, "paid", "2.1.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download":
			w.Header().Set("Content-Disposition", `attachment; filename="paid-2.1.0.zip"`)
			_, _ = w.Write(body)
		case "/page":
			_, _ = w.Write([]byte("<html>login</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	allowZipURLs(t, srv.URL)

	rec := stageFromURL(t, srv.URL+"/download?key=x")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var z updatejobs.StagedZip
	if err := json.Unmarshal(rec.Body.Bytes(), &z); err != nil {
		t.Fatal(err)
	}
	if z.ID == "" || z.Filename != "paid-2.1.0.zip" || z.Plugin.Slug != "paid" || z.Plugin.Version != "2.1.0" || z.Size != int64(len(body)) {
		t.Errorf("staged = %+v", z)
	}

	for path, want := range map[string]int{"/page": http.StatusBadRequest, "/missing": http.StatusBadGateway} {
		if rec := stageFromURL(t, srv.URL+path); rec.Code != want {
			t.Errorf("%s: status %d, want %d (%s)", path, rec.Code, want, rec.Body.String())
		}
	}
	if rec := stageFromURL(t, "file:///etc/passwd"); rec.Code != http.StatusBadRequest {
		t.Errorf("file URL: status %d", rec.Code)
	}
}

func TestPluginZipUpdateValidation(t *testing.T) {
	setupZipTest(t)
	site := "11111111-1111-1111-1111-111111111111"
	if err := db.SaveSitePlugin(models.WPPlugin{SiteID: site, Name: "paid", Status: "active", Version: "1.0"}); err != nil {
		t.Fatal(err)
	}
	if v, err := zipUpdateTarget(site, "paid"); err != nil || v != "1.0" {
		t.Errorf("zipUpdateTarget = %q, %v", v, err)
	}
	if _, err := zipUpdateTarget(site, "other"); err == nil {
		t.Error("zipUpdateTarget accepted a plugin the site doesn't have")
	}

	for _, c := range []struct{ slug, body string }{
		{"paid", `{"zip_id": "x", "site_ids": []}`},
		{"../x", `{"zip_id": "x", "site_ids": ["` + site + `"]}`},
		{"paid", `{`},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/plugins/"+c.slug+"/zip-update", strings.NewReader(c.body))
		req.SetPathValue("slug", c.slug)
		rec := httptest.NewRecorder()
		PluginZipUpdateHandler(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status %d, want 400", c.slug, c.body, rec.Code)
		}
	}
}

func TestStagePluginZipRejectsNonPlugin(t *testing.T) {
	setupZipTest(t)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, _ := w.Create("docs/readme.txt")
	_, _ = fw.Write([]byte("hello"))
	_ = w.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(buf.Bytes()) }))
	defer srv.Close()
	allowZipURLs(t, srv.URL)

	if rec := stageFromURL(t, srv.URL+"/x.zip"); rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	entries, _ := os.ReadDir(updatejobs.UploadDir())
	if len(entries) != 0 {
		t.Errorf("rejected ZIP left on disk: %v", entries)
	}
}

func TestZipURLAllowed(t *testing.T) {
	prefixes := []string{"https://github.com/JCO-Digital", "https://downloads.wordpress.org/plugin/", "https://zips.example.com", "not a url", "https://evil.example.com/?x=1"}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/JCO-Digital/jcore-turva/releases/download/v1.2.0/jcore-turva.zip", true},
		{"https://GitHub.com/JCO-Digital/x.zip", true},
		{"https://github.com/JCO-Digital", true},
		{"https://github.com/jco-digital/x.zip", false},
		{"https://github.com/JCO-Digital-evil/x.zip", false},
		{"https://github.com/other/x.zip", false},
		{"https://github.com/JCO-Digital/../other/x.zip", false},
		{"https://github.com/JCO-Digital/%2E%2E/other/x.zip", false},
		{"https://github.com/JCO-Digital%2F..%2Fother/x.zip", false},
		{"https://github.com/JCO-Digital%2Fx.zip", false},
		{"http://github.com/JCO-Digital/x.zip", false},
		{"https://user@github.com/JCO-Digital/x.zip", false},
		{"https://github.com:8443/JCO-Digital/x.zip", false},
		{"https://github.com.evil.com/JCO-Digital/x.zip", false},
		{"https://downloads.wordpress.org/plugin/akismet.5.3.zip", true},
		{"https://downloads.wordpress.org/plugins/akismet.zip", false},
		{"https://zips.example.com/any/thing.zip?token=1", true},
		{"https://evil.example.com/?x=1", false},
	}
	for _, tt := range tests {
		u, err := url.Parse(tt.url)
		if err != nil {
			t.Fatalf("%s: %v", tt.url, err)
		}
		if got := zipURLAllowed(u, prefixes); got != tt.want {
			t.Errorf("zipURLAllowed(%s) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestStagePluginZipURLAllowlist(t *testing.T) {
	setupZipTest(t)
	fetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fetched = true }))
	defer srv.Close()

	allowZipURLs(t)
	if rec := stageFromURL(t, srv.URL+"/x.zip"); rec.Code != http.StatusForbidden {
		t.Errorf("no prefixes: status %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
	allowZipURLs(t, srv.URL+"/allowed/")
	if rec := stageFromURL(t, srv.URL+"/other/x.zip"); rec.Code != http.StatusForbidden {
		t.Errorf("outside the prefix: status %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
	if fetched {
		t.Error("a URL that isn't allowed was fetched")
	}
}

func TestZipDownloadRedirects(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://github.com/JCO-Digital/x.zip", nil)
	if err := zipHTTPClient.CheckRedirect(httptest.NewRequest(http.MethodGet, "https://release-assets.githubusercontent.com/x", nil), []*http.Request{req}); err != nil {
		t.Errorf("HTTPS redirect refused: %v", err)
	}
	if err := zipHTTPClient.CheckRedirect(httptest.NewRequest(http.MethodGet, "http://10.0.0.1/x", nil), []*http.Request{req}); err == nil {
		t.Error("redirect to plain HTTP allowed")
	}
}
