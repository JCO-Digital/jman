package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupAgentTest(t *testing.T) {
	t.Helper()
	setupSettingsTest(t)

	cacheDir, err := os.MkdirTemp("", "jman-agent-cache-*")
	if err != nil {
		t.Fatalf("failed to create temp cache dir: %v", err)
	}
	oldCacheDir := config.RunData.CacheDir
	config.RunData.CacheDir = cacheDir

	t.Cleanup(func() {
		os.RemoveAll(cacheDir)
		config.RunData.CacheDir = oldCacheDir
	})
}

func TestAgentManifestHandler_IncludesNonWPSites(t *testing.T) {
	setupAgentTest(t)

	sites := []models.Site{
		{ID: 101, ServerID: 5, Domain: "wp-site.com", Status: "deployed", IsWordpress: true, SiteUser: "wpuser"},
		{ID: 102, ServerID: 5, Domain: "app-site.com", Status: "deployed", IsWordpress: false, SiteUser: "appuser"},
		{ID: 103, ServerID: 5, Domain: "pending-site.com", Status: "deploying", IsWordpress: true, SiteUser: "pendinguser"},
		{ID: 104, ServerID: 9, Domain: "other-server.com", Status: "deployed", IsWordpress: true, SiteUser: "otheruser"},
	}
	if err := cache.WriteJSONCache("sites", sites); err != nil {
		t.Fatalf("failed to seed sites cache: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/agent/manifest", nil)
	ctx := contextWithAgentClaims(context.Background(), &AgentClaims{TokenID: 1, ServerID: 5})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	AgentManifestHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var manifest models.AgentManifest
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("failed to decode manifest response: %v", err)
	}

	if manifest.ServerID != 5 {
		t.Errorf("ServerID = %d, want 5", manifest.ServerID)
	}
	if len(manifest.Sites) != 2 {
		t.Fatalf("expected 2 deployed sites in manifest, got %d", len(manifest.Sites))
	}

	site1 := manifest.Sites[0]
	if site1.SiteID != 101 || site1.Domain != "wp-site.com" || !site1.IsWordpress {
		t.Errorf("unexpected site 0: %+v", site1)
	}

	site2 := manifest.Sites[1]
	if site2.SiteID != 102 || site2.Domain != "app-site.com" || site2.IsWordpress {
		t.Errorf("unexpected site 1: %+v", site2)
	}
}

func TestAgentReportHandler_AcceptsNonWPSiteData(t *testing.T) {
	setupAgentTest(t)

	sites := []models.Site{
		{ID: 201, ServerID: 5, Domain: "app.example.com", Status: "deployed", IsWordpress: false},
	}
	if err := cache.WriteJSONCache("sites", sites); err != nil {
		t.Fatalf("failed to seed sites cache: %v", err)
	}

	bytesUsed := int64(12345678)
	report := models.AgentReport{
		CollectedAt:  time.Now().UTC().Format(time.RFC3339),
		AgentVersion: "1.2.3",
		Sites: []models.AgentReportSite{
			{
				SiteID:         201,
				DiskUsageBytes: &bytesUsed,
				TrafficHourly: []models.TrafficHourlyEntry{
					{
						Hour:          time.Now().UTC().Add(-time.Hour).Truncate(time.Hour).Format(time.RFC3339),
						RequestsTotal: 100,
						RequestsHuman: 80,
						RequestsBot:   20,
					},
				},
			},
		},
	}

	body, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/agent/report", bytes.NewBuffer(body))
	ctx := contextWithAgentClaims(context.Background(), &AgentClaims{TokenID: 1, ServerID: 5})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	AgentReportHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode report response: %v", err)
	}
	if resp["accepted"] != 1 || resp["rejected"] != 0 {
		t.Errorf("expected 1 accepted, 0 rejected, got %+v", resp)
	}

	diskUsageMap, err := db.GetLatestSiteDiskUsage()
	if err != nil {
		t.Fatalf("failed to query disk usage: %v", err)
	}
	if usage, ok := diskUsageMap[201]; !ok || usage.BytesUsed != 12345678 {
		t.Errorf("expected disk usage 12345678 for site 201, got %+v", usage)
	}
}
