package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
