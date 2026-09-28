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

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
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

	if err := db.InitInventory(); err != nil {
		t.Fatalf("failed to init inventory DB: %v", err)
	}
}

// seedAgentSites stores SpinupWP-style inventory sites; each entry is
// {legacy site ID, legacy server ID, domain, status, is WordPress}.
func seedAgentSites(t *testing.T, sites ...agentSite) {
	t.Helper()
	for _, s := range sites {
		serverUUID := utils.SpinupWPServerUUID(s.serverID)
		if err := db.SaveManagedServer(models.ManagedServer{ID: serverUUID, Name: fmt.Sprintf("server-%d", s.serverID), Provider: "spinupwp", SSHPort: 22}); err != nil {
			t.Fatalf("failed to seed server: %v", err)
		}
		if err := db.SaveManagedSite(models.ManagedSite{
			ID:             utils.SpinupWPSiteUUID(s.id),
			ServerID:       &serverUUID,
			Provider:       "spinupwp",
			ProviderSiteID: fmt.Sprint(s.id),
			Domain:         s.domain,
			Environment:    models.SiteEnvironmentProduction,
			IsWordpress:    s.isWordpress,
			SSHUser:        "user",
			Status:         s.status,
		}); err != nil {
			t.Fatalf("failed to seed site: %v", err)
		}
	}
}

type agentSite = struct {
	id, serverID int
	domain       string
	status       string
	isWordpress  bool
}

func TestAgentManifestHandler_IncludesNonWPSites(t *testing.T) {
	setupAgentTest(t)

	seedAgentSites(t,
		agentSite{101, 5, "wp-site.com", "deployed", true},
		agentSite{102, 5, "app-site.com", "deployed", false},
		agentSite{103, 5, "pending-site.com", "deploying", true},
		agentSite{104, 9, "other-server.com", "deployed", true},
	)
	server5 := utils.SpinupWPServerUUID(5)

	req := httptest.NewRequest("GET", "/api/agent/manifest", nil)
	ctx := contextWithAgentClaims(context.Background(), &AgentClaims{TokenID: 1, ServerID: server5})
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

	if manifest.ServerID != server5 {
		t.Errorf("ServerID = %s, want %s", manifest.ServerID, server5)
	}
	if len(manifest.Sites) != 2 {
		t.Fatalf("expected 2 deployed sites in manifest, got %d", len(manifest.Sites))
	}

	byDomain := map[string]models.AgentManifestSite{}
	for _, s := range manifest.Sites {
		byDomain[s.Domain] = s
	}
	if s := byDomain["wp-site.com"]; s.SiteID != utils.SpinupWPSiteUUID(101) || s.LegacySiteID != 101 || !s.IsWordpress {
		t.Errorf("unexpected wp-site.com entry: %+v", s)
	}
	if s := byDomain["app-site.com"]; s.SiteID != utils.SpinupWPSiteUUID(102) || s.LegacySiteID != 102 || s.IsWordpress {
		t.Errorf("unexpected app-site.com entry: %+v", s)
	}
}

func TestAgentReportHandler_AcceptsNonWPSiteData(t *testing.T) {
	setupAgentTest(t)

	seedAgentSites(t,
		agentSite{201, 5, "app.example.com", "deployed", false},
		agentSite{202, 9, "elsewhere.example.com", "deployed", true},
	)
	site201 := utils.SpinupWPSiteUUID(201)

	bytesUsed := int64(12345678)
	report := models.AgentReport{
		CollectedAt:  time.Now().UTC().Format(time.RFC3339),
		AgentVersion: "1.2.3",
		Sites: []models.AgentReportSite{
			{
				SiteID:         site201,
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
	ctx := contextWithAgentClaims(context.Background(), &AgentClaims{TokenID: 1, ServerID: utils.SpinupWPServerUUID(5)})
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
	if usage, ok := diskUsageMap[site201]; !ok || usage.BytesUsed != 12345678 {
		t.Errorf("expected disk usage 12345678 for site 201, got %+v", usage)
	}
}

func TestAgentToken_VerificationAndUpgrade(t *testing.T) {
	setupAgentTest(t)

	// 1. Create a new token (should be sha256)
	token, plaintext, err := db.CreateAgentToken(utils.SpinupWPServerUUID(10), "test-server", "test desc", "admin")
	if err != nil {
		t.Fatalf("failed to create agent token: %v", err)
	}

	claims, err := db.VerifyAgentToken(plaintext)
	if err != nil {
		t.Fatalf("failed to verify sha256 token: %v", err)
	}
	if claims.ServerID != utils.SpinupWPServerUUID(10) || claims.TokenID != token.ID {
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
		utils.SpinupWPServerUUID(20), "legacy-server", string(legacyBcryptHash), legacySecret[:8], "legacy", "admin",
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
	if legacyClaims.ServerID != utils.SpinupWPServerUUID(20) || legacyClaims.TokenID != int(legacyID) {
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
	if secondVerifyClaims.ServerID != utils.SpinupWPServerUUID(20) {
		t.Errorf("unexpected claims on second verify: %+v", secondVerifyClaims)
	}
}
