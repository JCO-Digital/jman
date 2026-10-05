package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
)

func TestCheckPluginAction(t *testing.T) {
	cached := []models.WPPlugin{
		{Name: "on", Status: "active"},
		{Name: "off", Status: "inactive"},
		{Name: "mu", Status: "must-use"},
		{Name: "net", Status: "active-network"},
	}
	cases := []struct {
		action, plugin string
		ok             bool
	}{
		{"deactivate", "on", true},
		{"activate", "off", true},
		{"delete", "off", true},
		{"uninstall", "off", true},
		{"delete", "on", false},
		{"uninstall", "on", false},
		{"deactivate", "mu", false},
		{"activate", "missing", false},
		{"deactivate", "net", false},
	}
	for _, c := range cases {
		err := checkPluginAction(c.action, []string{c.plugin}, cached)
		if (err == nil) != c.ok {
			t.Errorf("%s %s: err = %v, want ok=%v", c.action, c.plugin, err, c.ok)
		}
	}
}

func TestSitePluginActionValidation(t *testing.T) {
	site := "11111111-1111-1111-1111-111111111111"
	for _, body := range []string{
		`{"action": "explode", "plugins": ["a"]}`,
		`{"action": "activate", "plugins": []}`,
		`{"action": "activate", "plugins": ["../x"]}`,
		`{`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site+"/plugin-actions", strings.NewReader(body))
		req.SetPathValue("id", site)
		rec := httptest.NewRecorder()
		SitePluginActionHandler(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status %d, want 400", body, rec.Code)
		}
	}
}

func TestValidateInstallSource(t *testing.T) {
	valid := map[string]string{
		"akismet":                             "akismet",
		"  woocommerce ":                      "woocommerce",
		"https://example.com/plugin.zip":      "https://example.com/plugin.zip",
		"https://example.com/p.ZIP?token=abc": "https://example.com/p.ZIP?token=abc",
	}
	for in, want := range valid {
		if got, err := validateInstallSource(in); err != nil || got != want {
			t.Errorf("validateInstallSource(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "a/b", "ftp://example.com/p.zip", "https://example.com/plugin", "file:///etc/passwd.zip", "--activate", "https://"} {
		if _, err := validateInstallSource(in); err == nil {
			t.Errorf("validateInstallSource(%q) accepted", in)
		}
	}
}

func TestIsPluginUpload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/sites/abc/plugin-install", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	if !isPluginUpload(req) {
		t.Error("multipart plugin-install POST not recognized")
	}
	req.Header.Set("Content-Type", "application/json")
	if isPluginUpload(req) {
		t.Error("JSON plugin-install treated as an upload")
	}
	other := httptest.NewRequest(http.MethodPost, "/api/sites/abc/notes", nil)
	other.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	if isPluginUpload(other) {
		t.Error("other route treated as a plugin upload")
	}
}

func TestSavePluginUploadRejectsNonZip(t *testing.T) {
	cases := map[string][]byte{
		"plugin.txt": []byte("PK\x03\x04rest"),
		"plugin.zip": []byte("not a zip"),
	}
	for name, content := range cases {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write(content)
		_ = mw.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/sites/abc/plugin-install", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		_, _, _, status, err := savePluginUpload(httptest.NewRecorder(), req)
		if err == nil || status != http.StatusBadRequest {
			t.Errorf("%s: status %d, err %v; want 400", name, status, err)
		}
	}
}

func TestSavePluginUploadStoresZip(t *testing.T) {
	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = t.TempDir()
	t.Cleanup(func() { config.RunData.DataDir = oldDataDir })

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "../../My Plugin.zip")
	_, _ = fw.Write([]byte("PK\x03\x04content"))
	_ = mw.WriteField("activate", "true")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/sites/abc/plugin-install", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	name, path, activate, _, err := savePluginUpload(httptest.NewRecorder(), req)
	if err != nil {
		t.Fatal(err)
	}
	if name != "My Plugin.zip" || !activate || filepath.Dir(path) != updatejobs.UploadDir() {
		t.Errorf("name %q activate %v path %q", name, activate, path)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "PK\x03\x04content" {
		t.Errorf("stored file = %q, %v", got, err)
	}
}

func TestSitePluginInstallRejectsInstalledSlug(t *testing.T) {
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

	site := "11111111-1111-1111-1111-111111111111"
	if err := db.SaveSitePlugin(models.WPPlugin{SiteID: site, Name: "akismet", Status: "inactive"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+site+"/plugin-install", strings.NewReader(`{"source": "akismet"}`))
	req.SetPathValue("id", site)
	rec := httptest.NewRecorder()
	SitePluginInstallHandler(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("status %d, want 409 (body %s)", rec.Code, rec.Body.String())
	}
}
