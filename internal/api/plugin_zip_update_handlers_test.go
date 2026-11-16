package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	if rec := stageFromURL(t, srv.URL+"/x.zip"); rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	entries, _ := os.ReadDir(updatejobs.UploadDir())
	if len(entries) != 0 {
		t.Errorf("rejected ZIP left on disk: %v", entries)
	}
}
