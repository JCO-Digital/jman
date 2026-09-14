package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"golang.org/x/crypto/bcrypt"
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

func TestAgentToken_VerificationAndUpgrade(t *testing.T) {
	setupAgentTest(t)

	// 1. Create a new token (should be sha256)
	token, plaintext, err := db.CreateAgentToken(10, "test-server", "test desc", "admin")
	if err != nil {
		t.Fatalf("failed to create agent token: %v", err)
	}

	claims, err := db.VerifyAgentToken(plaintext)
	if err != nil {
		t.Fatalf("failed to verify sha256 token: %v", err)
	}
	if claims.ServerID != 10 || claims.TokenID != token.ID {
		t.Errorf("unexpected claims: %+v", claims)
	}

	// 2. Insert a legacy bcrypt token
	legacySecret := "legacySecretValue1234567890123456"
	legacyBcryptHash, err := bcrypt.GenerateFromPassword([]byte(legacySecret), 10)
	if err != nil {
		t.Fatalf("failed to generate bcrypt hash: %v", err)
	}

	dbConn := db.GetAPIDB()
	res, err := dbConn.Exec(
		`INSERT INTO agent_tokens (server_id, server_name, token_hash, token_prefix, description, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		20, "legacy-server", string(legacyBcryptHash), legacySecret[:8], "legacy", "admin",
	)
	if err != nil {
		t.Fatalf("failed to insert legacy token: %v", err)
	}
	legacyID, _ := res.LastInsertId()
	legacyPlaintext := fmt.Sprintf("%d.%s", legacyID, legacySecret)

	// Verify legacy token
	legacyClaims, err := db.VerifyAgentToken(legacyPlaintext)
	if err != nil {
		t.Fatalf("failed to verify legacy token: %v", err)
	}
	if legacyClaims.ServerID != 20 || legacyClaims.TokenID != int(legacyID) {
		t.Errorf("unexpected claims for legacy token: %+v", legacyClaims)
	}

	// Check that the hash in the database was transparently migrated to sha256
	var updatedHash string
	err = dbConn.QueryRow("SELECT token_hash FROM agent_tokens WHERE id = ?", legacyID).Scan(&updatedHash)
	if err != nil {
		t.Fatalf("failed to query updated token hash: %v", err)
	}
	if !strings.HasPrefix(updatedHash, "sha256:") {
		t.Errorf("expected hash to be upgraded to sha256 prefix, got %s", updatedHash)
	}

	// Verify again using the newly upgraded sha256 hash
	secondVerifyClaims, err := db.VerifyAgentToken(legacyPlaintext)
	if err != nil {
		t.Fatalf("failed to verify upgraded token: %v", err)
	}
	if secondVerifyClaims.ServerID != 20 {
		t.Errorf("unexpected claims on second verify: %+v", secondVerifyClaims)
	}
}
