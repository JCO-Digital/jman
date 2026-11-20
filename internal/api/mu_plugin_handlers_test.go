package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// stubMuSources serves files by URL for a test; other URLs are 404.
func stubMuSources(t *testing.T, files map[string]string) *[]string {
	t.Helper()
	oldFetch, oldLatest := fetchMuSource, latestMuPluginURL
	var fetched []string
	fetchMuSource = func(url string) ([]byte, error) {
		fetched = append(fetched, url)
		if body, ok := files[url]; ok {
			return []byte(body), nil
		}
		return nil, errMuSourceNotFound
	}
	latestMuPluginURL = func() (string, error) {
		return muPluginDownloadPrefix + "v2.0.0/bojaco.php", nil
	}
	t.Cleanup(func() { fetchMuSource, latestMuPluginURL = oldFetch, oldLatest })
	return &fetched
}

func TestFetchMuPlugin(t *testing.T) {
	latest := muPluginDownloadPrefix + "v2.0.0/bojaco.php"
	pinned := muPluginDownloadPrefix + "v1.0.0/bojaco.php"
	masterCompat := "https://raw.githubusercontent.com/JCO-Digital/bojaco-compat/master/compat-old.php"
	stubMuSources(t, map[string]string{
		latest:       "<?php // 2",
		pinned:       "<?php // 1",
		masterCompat: "<?php // old",
		"https://raw.githubusercontent.com/JCO-Digital/bojaco-compat/main/compat-woocommerce.php": "<?php // wc",
	})

	ok := []struct {
		req                muPluginRequest
		name, source, body string
	}{
		{muPluginRequest{Plugin: "bojaco"}, "bojaco.php", latest, "<?php // 2"},
		{muPluginRequest{Plugin: "bojaco", Source: pinned}, "bojaco.php", pinned, "<?php // 1"},
		{muPluginRequest{Plugin: "compat", Slug: "woocommerce"}, "compat-woocommerce.php", "https://raw.githubusercontent.com/JCO-Digital/bojaco-compat/main/compat-woocommerce.php", "<?php // wc"},
		{muPluginRequest{Plugin: "compat", Slug: "old"}, "compat-old.php", masterCompat, "<?php // old"},
	}
	for _, c := range ok {
		f, status, err := fetchMuPlugin(c.req)
		if err != nil {
			t.Errorf("%+v: status %d, %v", c.req, status, err)
			continue
		}
		if f.name != c.name || f.source != c.source || string(f.data) != c.body {
			t.Errorf("%+v: got %s from %s (%q)", c.req, f.name, f.source, f.data)
		}
	}

	bad := []struct {
		req    muPluginRequest
		status int
	}{
		{muPluginRequest{Plugin: "other"}, http.StatusBadRequest},
		{muPluginRequest{Plugin: "bojaco", Source: "https://evil.example/bojaco.php"}, http.StatusBadRequest},
		{muPluginRequest{Plugin: "bojaco", Source: muPluginDownloadPrefix + "../../x"}, http.StatusBadRequest},
		{muPluginRequest{Plugin: "bojaco", Slug: "x"}, http.StatusBadRequest},
		{muPluginRequest{Plugin: "bojaco", Source: muPluginDownloadPrefix + "v9/missing.php"}, http.StatusBadGateway},
		{muPluginRequest{Plugin: "compat", Slug: "../x"}, http.StatusBadRequest},
		{muPluginRequest{Plugin: "compat", Slug: "Upper"}, http.StatusBadRequest},
		{muPluginRequest{Plugin: "compat", Slug: "x", Source: pinned}, http.StatusBadRequest},
		{muPluginRequest{Plugin: "compat", Slug: "missing"}, http.StatusNotFound},
	}
	for _, c := range bad {
		if _, status, err := fetchMuPlugin(c.req); err == nil || status != c.status {
			t.Errorf("%+v: status %d, err %v; want %d", c.req, status, err, c.status)
		}
	}
}

func TestFetchMuPluginLatestFails(t *testing.T) {
	stubMuSources(t, nil)
	latestMuPluginURL = func() (string, error) { return "", errors.New("rate limited") }
	if _, status, err := fetchMuPlugin(muPluginRequest{Plugin: "bojaco"}); err == nil || status != http.StatusBadGateway {
		t.Errorf("status %d, err %v", status, err)
	}
}

func TestInstallMuPluginValidation(t *testing.T) {
	setupZipTest(t)
	fetched := stubMuSources(t, nil)
	for _, body := range []string{
		`{`,
		`{"plugin": "bojaco", "sites": []}`,
		`{"plugin": "bojaco", "sites": ["not-a-site"]}`,
	} {
		rec := httptest.NewRecorder()
		InstallMuPluginHandler(rec, httptest.NewRequest(http.MethodPost, "/api/mu-plugins", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", body, rec.Code, rec.Body.String())
		}
	}
	if len(*fetched) != 0 {
		t.Errorf("fetched %v before the sites were checked", *fetched)
	}
}

func TestAllowlistMuPlugin(t *testing.T) {
	setupZipTest(t)
	const site = "11111111-1111-1111-1111-111111111111"
	const hash = "aa11"
	now := time.Now()
	for _, f := range []models.ScanFinding{
		{Engine: "agent", Rule: "mu-plugin-change", Severity: "medium", Path: "wp-content/mu-plugins/bojaco.php", SHA256: hash},
		{Engine: "agent", Rule: "mu-plugin-change", Severity: "medium", Path: "wp-content/mu-plugins/other.php", SHA256: "bb22"},
	} {
		if err := db.UpsertScanFinding(site, f, "1", now); err != nil {
			t.Fatal(err)
		}
	}

	file := &muPluginFile{name: "bojaco.php", source: muPluginDownloadPrefix + "v2/bojaco.php"}
	if err := allowlistMuPlugin(file, hash, site, "alice"); err != nil {
		t.Fatal(err)
	}
	// A second install of the same file keeps the one entry.
	if err := allowlistMuPlugin(file, hash, site, "bob"); err != nil {
		t.Fatal(err)
	}

	entry, err := db.GetScanAllowlistEntryByHash(hash)
	if err != nil || entry == nil {
		t.Fatalf("allowlist entry = %v, %v", entry, err)
	}
	if entry.CreatedBy != "alice" || entry.Path != "wp-content/mu-plugins/bojaco.php" || !strings.Contains(entry.Reason, file.source) {
		t.Errorf("entry = %+v", entry)
	}
	findings, err := db.ListScanFindings(db.ScanFindingFilter{SiteID: site})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		want := models.ScanFindingOpen
		if f.SHA256 == hash {
			want = models.ScanFindingAllowlisted
		}
		if f.Status != want {
			t.Errorf("%s: status %s, want %s", f.Path, f.Status, want)
		}
	}
}
