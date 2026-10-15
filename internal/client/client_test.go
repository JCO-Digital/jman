package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestNormalizeURL(t *testing.T) {
	tests := map[string]string{
		"https://jman.example.com":       "https://jman.example.com",
		"https://jman.example.com/":      "https://jman.example.com",
		"https://jman.example.com/api":   "https://jman.example.com",
		"https://jman.example.com/api/":  "https://jman.example.com",
		"jman.example.com":               "https://jman.example.com",
		"http://localhost:8080":          "http://localhost:8080",
		"https://example.com/jman/api?x": "https://example.com/jman",
	}
	for in, want := range tests {
		if got, err := NormalizeURL(in); err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x.example.com", "https://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) should fail", bad)
		}
	}
}

func TestVersionChecks(t *testing.T) {
	if CheckMinVersion("6.11.0", "6.12.0") == nil {
		t.Error("6.11.0 is below 6.12.0")
	}
	if err := CheckMinVersion("v6.12.0", "6.12.0"); err != nil {
		t.Errorf("equal versions: %v", err)
	}
	if err := CheckMinVersion("dev", "6.12.0"); err != nil {
		t.Errorf("dev builds aren't blocked: %v", err)
	}
	if err := CheckMinVersion("6.11.0", ""); err != nil {
		t.Errorf("no minimum: %v", err)
	}
	if !NewerAvailable("6.11.0", "6.11.1") || NewerAvailable("6.11.1", "6.11.1") || NewerAvailable("dev", "7.0.0") {
		t.Error("NewerAvailable")
	}
}

func TestWaitForJobs(t *testing.T) {
	jobPollInterval = time.Millisecond
	polls := map[string]int{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		polls[r.URL.Path]++
		n := polls[r.URL.Path]
		mu.Unlock()
		id := strings.TrimPrefix(r.URL.Path, "/api/update-jobs/")
		status := models.UpdateJobRunning
		// Job 1 finishes on the second poll, job 2 on the fourth.
		if (id == "1" && n >= 2) || (id == "2" && n >= 4) {
			status = models.UpdateJobDone
		}
		w.Header().Set(cliproto.LatestCLIVersionHeader, "dev")
		_ = json.NewEncoder(w).Encode(models.UpdateJob{ID: map[string]int64{"1": 1, "2": 2}[id], Status: status})
	}))
	defer srv.Close()

	var order []int64
	err := New(srv.URL, "t").WaitForJobs(context.Background(), []models.UpdateJob{{ID: 1}, {ID: 2}}, func(j models.UpdateJob) {
		order = append(order, j.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("jobs finished in order %v, want [1 2]", order)
	}
}

func TestPostFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my-plugin.zip")
	if err := os.WriteFile(path, []byte("PK\x03\x04zip"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(file)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name": header.Filename, "body": string(body), "activate": r.FormValue("activate"),
		})
	}))
	defer srv.Close()

	var got map[string]string
	if err := New(srv.URL, "t").PostFile(context.Background(), "/sites/x/plugin-install", path, map[string]string{"activate": "true"}, &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "my-plugin.zip" || got["body"] != "PK\x03\x04zip" || got["activate"] != "true" {
		t.Errorf("server saw %+v", got)
	}
}
