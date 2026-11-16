package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
)

const copySiteID = "22222222-2222-2222-2222-222222222222"

// cleanScan records a successful scan of the copy test site at t.
func cleanScan(t *testing.T, at time.Time) {
	t.Helper()
	if err := db.RecordSiteScan(copySiteID, &models.AgentScan{Fingerprint: "f"}, at); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSourceSiteScan(t *testing.T) {
	setupZipTest(t)
	if _, status, err := checkSourceSiteScan(copySiteID, "alpha"); err == nil || status != http.StatusConflict {
		t.Errorf("never scanned: %d, %v", status, err)
	}

	cleanScan(t, time.Now().Add(-5*time.Hour))
	if _, _, err := checkSourceSiteScan(copySiteID, "alpha"); err == nil || !strings.Contains(err.Error(), "last scanned") {
		t.Errorf("old scan: %v", err)
	}

	if err := db.RecordSiteScan(copySiteID, &models.AgentScan{Error: "boom"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := checkSourceSiteScan(copySiteID, "alpha"); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Errorf("failed scan: %v", err)
	}

	scannedAt := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	cleanScan(t, scannedAt)
	got, _, err := checkSourceSiteScan(copySiteID, "alpha")
	if err != nil || !got.Equal(scannedAt) {
		t.Errorf("clean scan: %v, %v (want %v)", got, err, scannedAt)
	}

	finding := models.ScanFinding{Engine: "builtin", Rule: "php-in-uploads", Severity: "high", Path: "wp-content/uploads/x.php", SHA256: "a"}
	if err := db.UpsertScanFinding(copySiteID, finding, "1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := checkSourceSiteScan(copySiteID, "alpha"); err == nil || !strings.Contains(err.Error(), "1 open malware scan finding") {
		t.Errorf("open finding: %v", err)
	}
}

func TestTarToZip(t *testing.T) {
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tarOf := func(entries ...tar.Header) []byte {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		for _, h := range entries {
			body := ""
			if h.Typeflag == tar.TypeReg {
				body = "content of " + h.Name
				h.Size = int64(len(body))
			}
			if err := tw.WriteHeader(&h); err != nil {
				t.Fatal(err)
			}
			_, _ = tw.Write([]byte(body))
		}
		_ = tw.Close()
		return buf.Bytes()
	}

	var out bytes.Buffer
	res, err := tarToZip(bytes.NewReader(tarOf(
		tar.Header{Name: "paid/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: old},
		tar.Header{Name: "paid/paid.php", Typeflag: tar.TypeReg, Mode: 0o644, ModTime: old},
		tar.Header{Name: "paid/lib/new.php", Typeflag: tar.TypeReg, Mode: 0o644, ModTime: old.Add(time.Hour)},
		tar.Header{Name: "paid/config.php", Typeflag: tar.TypeSymlink, Linkname: "../../../wp-config.php", ModTime: old},
	)), "paid", &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.files != 2 || res.newestPath != "paid/lib/new.php" || len(res.skipped) != 1 || res.skipped[0] != "paid/config.php" {
		t.Errorf("result = %+v", res)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "paid/,paid/paid.php,paid/lib/new.php" {
		t.Errorf("ZIP entries = %v", names)
	}

	for _, name := range []string{"other/x.php", "paid/../x.php", "/paid/x.php", "paidx/x.php"} {
		_, err := tarToZip(bytes.NewReader(tarOf(tar.Header{Name: name, Typeflag: tar.TypeReg, ModTime: old})), "paid", &bytes.Buffer{})
		if !errors.Is(err, errCopyRefused) {
			t.Errorf("%s: err = %v, want refused", name, err)
		}
	}

	_, err = tarToZip(bytes.NewReader(tarOf(tar.Header{Name: "paid/a.php", Typeflag: tar.TypeReg, ModTime: old})), "paid",
		&countingWriter{w: &bytes.Buffer{}, max: 10})
	if !errors.Is(err, errZipTooLarge) {
		t.Errorf("too large: err = %v", err)
	}
}

// TestStagePluginZipFromSite copies a plugin directory with the real tar,
// locally (an empty SSH spec).
func TestStagePluginZipFromSite(t *testing.T) {
	setupZipTest(t)
	if err := db.SaveSitePlugin(models.WPPlugin{SiteID: copySiteID, Name: "paid", Status: "active", Version: "2.0"}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	dir := filepath.Join(root, "paid")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"paid.php":   "<?php\n/**\n * Plugin Name: Paid\n * Version: 2.0\n */\n",
		"debug.log":  "secret",
		".git/HEAD":  "ref",
		"assets.css": "a{}",
	}
	past := time.Now().Add(-time.Hour)
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, past, past)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(dir, "link.php")); err != nil {
		t.Fatal(err)
	}

	oldLookup, oldDir := lookupCliSite, pluginDir
	lookupCliSite = func(id string) (*models.CliSite, error) {
		return &models.CliSite{ID: id, Name: "alpha.test"}, nil
	}
	pluginDir = func(_, _, _, slug string) (string, error) { return filepath.Join(root, slug), nil }
	t.Cleanup(func() { lookupCliSite, pluginDir = oldLookup, oldDir })

	stage := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		StagePluginZipHandler(rec, httptest.NewRequest(http.MethodPost, "/api/plugin-zips",
			strings.NewReader(`{"site_id": "`+copySiteID+`", "plugin": "paid"}`)))
		return rec
	}

	if rec := stage(); rec.Code != http.StatusConflict {
		t.Errorf("unscanned site: status %d: %s", rec.Code, rec.Body.String())
	}

	cleanScan(t, time.Now().Add(-time.Minute))
	rec := stage()
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var z updatejobs.StagedZip
	if err := json.Unmarshal(rec.Body.Bytes(), &z); err != nil {
		t.Fatal(err)
	}
	if z.Filename != "paid.zip" || z.Plugin.Slug != "paid" || z.Plugin.Version != "2.0" ||
		z.SourceSite == nil || z.SourceSite.Name != "alpha.test" || len(z.SourceSite.Skipped) != 1 {
		t.Fatalf("staged = %+v (source %+v)", z, z.SourceSite)
	}
	taken, ok := updatejobs.TakeStagedZip(z.ID, z.CreatedBy)
	if !ok {
		t.Fatal("staged ZIP not found")
	}
	zr, err := zip.OpenReader(taken.Path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	zr.Close()
	got := strings.Join(names, ",")
	if !strings.Contains(got, "paid/paid.php") || !strings.Contains(got, "paid/assets.css") ||
		strings.Contains(got, "debug.log") || strings.Contains(got, ".git") || strings.Contains(got, "link.php") {
		t.Errorf("ZIP entries = %s", got)
	}

	// A file changed after the scan hasn't been scanned.
	_ = os.Chtimes(filepath.Join(dir, "assets.css"), time.Now(), time.Now())
	if rec := stage(); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "paid/assets.css") {
		t.Errorf("changed after scan: status %d: %s", rec.Code, rec.Body.String())
	}

	// Not installed on the source site.
	rec = httptest.NewRecorder()
	StagePluginZipHandler(rec, httptest.NewRequest(http.MethodPost, "/api/plugin-zips",
		strings.NewReader(`{"site_id": "`+copySiteID+`", "plugin": "other"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("other plugin: status %d", rec.Code)
	}
}
