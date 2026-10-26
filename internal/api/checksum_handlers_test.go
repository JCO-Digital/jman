package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCO-Digital/jman/internal/fetch/wporg"
)

func TestAgentCoreChecksums(t *testing.T) {
	setupAgentTest(t)
	calls := 0
	wporgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("version") != "6.8.3" {
			fmt.Fprint(w, `{"checksums":false}`)
			return
		}
		fmt.Fprint(w, `{"checksums":{"index.php":"aa","wp-includes/version.php":"bb","wp-content/index.php":"cc"}}`)
	}))
	defer wporgSrv.Close()
	old := wporg.CoreChecksumsAPIURL
	wporg.CoreChecksumsAPIURL = wporgSrv.URL + "/"
	t.Cleanup(func() { wporg.CoreChecksumsAPIURL = old })

	get := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		AgentCoreChecksumsHandler(w, httptest.NewRequest("GET", "/api/agent/core-checksums?"+query, nil))
		return w
	}

	// wp-content is the site's own, so its checksums are left out; the
	// second request is served from the cache.
	for range 2 {
		w := get("version=6.8.3&locale=fi")
		var resp struct {
			Version, Locale string
			MD5             map[string]string
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); w.Code != 200 || err != nil {
			t.Fatalf("checksums: %d %s", w.Code, w.Body.String())
		}
		if resp.Version != "6.8.3" || resp.Locale != "fi" || len(resp.MD5) != 2 || resp.MD5["index.php"] != "aa" {
			t.Fatalf("checksums = %+v", resp)
		}
	}
	if calls != 1 {
		t.Fatalf("wordpress.org asked %d times, want 1", calls)
	}

	// Unknown releases are 404, and remembered for a while.
	for range 2 {
		if w := get("version=9.9.9&locale=en_US"); w.Code != http.StatusNotFound {
			t.Fatalf("unknown version: %d", w.Code)
		}
	}
	if calls != 2 {
		t.Fatalf("wordpress.org asked %d times, want 2", calls)
	}

	for _, q := range []string{"version=6.8&locale=../x", "version=x;y&locale=fi", "locale=fi"} {
		if w := get(q); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, w.Code)
		}
	}
}

func TestAgentPluginChecksums(t *testing.T) {
	setupAgentTest(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/akismet/5.3.json" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"plugin":"akismet","version":"5.3","files":{"akismet.php":{"md5":"x","sha256":"aa"},"class.akismet.php":{"md5":["y","z"],"sha256":["bb","cc"]}}}`)
	}))
	defer srv.Close()
	old := wporg.PluginChecksumsURL
	wporg.PluginChecksumsURL = srv.URL + "/"
	t.Cleanup(func() { wporg.PluginChecksumsURL = old })

	get := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		AgentPluginChecksumsHandler(w, httptest.NewRequest("GET", "/api/agent/plugin-checksums?"+query, nil))
		return w
	}
	for range 2 {
		w := get("slug=akismet&version=5.3")
		var resp struct {
			Slug, Version string
			SHA256        map[string][]string
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); w.Code != 200 || err != nil {
			t.Fatalf("checksums: %d %s", w.Code, w.Body.String())
		}
		if resp.Slug != "akismet" || len(resp.SHA256["akismet.php"]) != 1 || len(resp.SHA256["class.akismet.php"]) != 2 {
			t.Fatalf("checksums = %+v", resp)
		}
	}
	for range 2 {
		if w := get("slug=some-premium-plugin&version=1.0"); w.Code != http.StatusNotFound {
			t.Fatalf("premium plugin: %d", w.Code)
		}
	}
	if calls != 2 {
		t.Fatalf("wordpress.org asked %d times, want 2", calls)
	}
	for _, q := range []string{"slug=../x&version=1", "slug=akismet&version=../1", "slug=akismet"} {
		if w := get(q); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, w.Code)
		}
	}
}
