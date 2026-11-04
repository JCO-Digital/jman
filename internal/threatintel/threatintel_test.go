package threatintel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
)

func setup(t *testing.T, handler http.HandlerFunc) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	oldRun, oldKEV, oldEPSS := config.RunData, KEVURL, EPSSURL
	config.RunData.CacheDir = t.TempDir()
	KEVURL, EPSSURL = srv.URL+"/kev", srv.URL+"/epss"
	t.Cleanup(func() { config.RunData, KEVURL, EPSSURL = oldRun, oldKEV, oldEPSS })
	return &calls
}

func TestKEVIsCached(t *testing.T) {
	calls := setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"vulnerabilities":[{"cveID":"CVE-2024-1","dateAdded":"2024-02-01","knownRansomwareCampaignUse":"Known"}]}`))
	})
	for i := 0; i < 2; i++ {
		kev, err := KEV(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if e := kev["CVE-2024-1"]; e.DateAdded != "2024-02-01" || !e.KnownRansomware {
			t.Errorf("entry = %+v", e)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("fetched %d times", calls.Load())
	}
}

func TestEPSSFetchesOnlyMissing(t *testing.T) {
	var asked []string
	setup(t, func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Query().Get("cve"))
		w.Write([]byte(`{"data":[{"cve":"CVE-2024-1","epss":"0.5","percentile":"0.97","date":"2026-11-02"}]}`))
	})
	got, err := EPSSScores(context.Background(), []string{"CVE-2024-1", "CVE-2024-2"})
	if err != nil {
		t.Fatal(err)
	}
	if got["CVE-2024-1"].Score != 0.5 || len(got) != 1 {
		t.Errorf("scores = %+v", got)
	}
	// Both are cached now, the unscored one too.
	if _, err := EPSSScores(context.Background(), []string{"CVE-2024-2", "CVE-2024-1"}); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "CVE-2024-1,CVE-2024-2" {
		t.Errorf("asked = %v", asked)
	}
}

func TestFetchFailure(t *testing.T) {
	setup(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	if _, err := KEV(context.Background()); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("KEV error = %v", err)
	}
	if _, err := EPSSScores(context.Background(), []string{"CVE-2024-1"}); err == nil {
		t.Error("EPSS error is nil")
	}
}

func TestCVEIDs(t *testing.T) {
	got := CVEIDs("cve-2024-12345 and CVE-2023-0001", "CVE-2024-12345", "WPVDB-1")
	if strings.Join(got, ",") != "CVE-2023-0001,CVE-2024-12345" {
		t.Errorf("CVEIDs = %v", got)
	}
}
