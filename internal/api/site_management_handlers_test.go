package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func setupSiteManagementTest(t *testing.T) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "jman-site-mgmt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = tempDir

	if err := db.InitInventory(); err != nil {
		t.Fatalf("Failed to init inventory DB: %v", err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatalf("Failed to init API DB: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tempDir)
		config.RunData.DataDir = oldDataDir
	})
}

func TestCreateAndGetSiteHandler(t *testing.T) {
	setupSiteManagementTest(t)
	claims := &AuthClaims{Username: "admin", Level: config.LevelEdit}
	ctx := contextWithClaims(context.Background(), claims)

	// 1. Missing domain should return 400
	badBody := []byte(`{"domain": ""}`)
	req := httptest.NewRequest("POST", "/api/sites", bytes.NewReader(badBody)).WithContext(ctx)
	w := httptest.NewRecorder()
	CreateSiteHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	// 2. Successful creation
	createPayload := CreateSiteRequest{
		Domain:         "external.example.com",
		Provider:       "wpengine",
		Environment:    models.SiteEnvironmentProduction,
		SSHHost:        "external.ssh.wpengine.net",
		SSHPort:        2222,
		SSHUser:        "extuser",
		SitePath:       "sites/extuser",
		ConnectionType: "ssh",
	}
	body, _ := json.Marshal(createPayload)
	req = httptest.NewRequest("POST", "/api/sites", bytes.NewReader(body)).WithContext(ctx)
	w = httptest.NewRecorder()
	CreateSiteHandler(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created models.ManagedSite
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if created.Domain != "external.example.com" || created.SSHHost != "external.ssh.wpengine.net" || created.SSHPort != 2222 {
		t.Fatalf("unexpected created site: %+v", created)
	}
	if !utils.IsValidUUID(created.ID) {
		t.Fatalf("expected valid UUID, got %s", created.ID)
	}

	// 3. Duplicate domain should return 409 Conflict
	req = httptest.NewRequest("POST", "/api/sites", bytes.NewReader(body)).WithContext(ctx)
	w = httptest.NewRecorder()
	CreateSiteHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}

	// 4. Get by UUID
	req = httptest.NewRequest("GET", "/api/sites/"+created.ID, nil).WithContext(ctx)
	req.SetPathValue("id", created.ID)
	w = httptest.NewRecorder()
	GetSiteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	var fetched models.ManagedSite
	if err := json.Unmarshal(w.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}
	if fetched.ID != created.ID || fetched.Domain != "external.example.com" {
		t.Fatalf("fetched site mismatch: %+v", fetched)
	}
}

func TestUpdateAndDeleteSiteHandler(t *testing.T) {
	setupSiteManagementTest(t)
	claims := &AuthClaims{Username: "admin", Level: config.LevelEdit}
	ctx := contextWithClaims(context.Background(), claims)

	siteID := utils.NewV7UUID()
	site := models.ManagedSite{
		ID:             siteID,
		Domain:         "update-test.com",
		Provider:       "manual",
		Environment:    models.SiteEnvironmentStaging,
		SSHHost:        "host1.example.com",
		SSHPort:        22,
		SSHUser:        "user1",
		SitePath:       "files",
		ConnectionType: "ssh",
	}
	if err := db.SaveManagedSite(site); err != nil {
		t.Fatalf("failed to seed site: %v", err)
	}

	// Update site
	updatePayload := CreateSiteRequest{
		Environment: models.SiteEnvironmentProduction,
		SSHPort:     2200,
	}
	body, _ := json.Marshal(updatePayload)
	req := httptest.NewRequest("PUT", "/api/sites/"+siteID, bytes.NewReader(body)).WithContext(ctx)
	req.SetPathValue("id", siteID)
	w := httptest.NewRecorder()
	UpdateSiteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.ManagedSite
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("failed to decode updated response: %v", err)
	}
	if updated.Environment != models.SiteEnvironmentProduction || updated.SSHPort != 2200 {
		t.Fatalf("unexpected updated fields: %+v", updated)
	}

	// Delete site
	req = httptest.NewRequest("DELETE", "/api/sites/"+siteID, nil).WithContext(ctx)
	req.SetPathValue("id", siteID)
	w = httptest.NewRecorder()
	DeleteSiteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify gone
	req = httptest.NewRequest("GET", "/api/sites/"+siteID, nil).WithContext(ctx)
	req.SetPathValue("id", siteID)
	w = httptest.NewRecorder()
	GetSiteHandler(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateAndDeleteServerHandler(t *testing.T) {
	setupSiteManagementTest(t)
	claims := &AuthClaims{Username: "admin", Level: config.LevelEdit}
	ctx := contextWithClaims(context.Background(), claims)

	// Create server
	createPayload := CreateServerRequest{
		Name:      "WPEngine Cluster",
		Provider:  "wpengine",
		IsLogical: true,
	}
	body, _ := json.Marshal(createPayload)
	req := httptest.NewRequest("POST", "/api/servers", bytes.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	CreateServerHandler(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created models.ManagedServer
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if created.Name != "WPEngine Cluster" || !created.IsLogical {
		t.Fatalf("unexpected server: %+v", created)
	}

	// Delete server
	req = httptest.NewRequest("DELETE", "/api/servers/"+created.ID, nil).WithContext(ctx)
	req.SetPathValue("id", created.ID)
	w = httptest.NewRecorder()
	DeleteServerHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSitesHandler_FormatManaged(t *testing.T) {
	setupSiteManagementTest(t)
	claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	site := models.ManagedSite{
		ID:             utils.NewV7UUID(),
		Domain:         "managed-format-test.com",
		Provider:       "manual",
		Environment:    models.SiteEnvironmentProduction,
		SSHHost:        "host.example.com",
		SSHUser:        "user",
		SitePath:       "files",
		ConnectionType: "ssh",
	}
	if err := db.SaveManagedSite(site); err != nil {
		t.Fatalf("failed to seed site: %v", err)
	}

	// Call GET /api/sites?format=managed
	req := httptest.NewRequest("GET", "/api/sites?format=managed", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	SitesHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var sites []models.ManagedSite
	if err := json.Unmarshal(w.Body.Bytes(), &sites); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(sites) != 1 || sites[0].Domain != "managed-format-test.com" {
		t.Fatalf("unexpected sites list: %+v", sites)
	}
}

func TestServersHandler_FormatManaged(t *testing.T) {
	setupSiteManagementTest(t)
	claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	// Empty inventory returns an empty array, not null
	req := httptest.NewRequest("GET", "/api/servers?format=managed", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	ServersHandler(w, req)
	if w.Code != http.StatusOK || bytes.TrimSpace(w.Body.Bytes())[0] != '[' {
		t.Fatalf("expected 200 with empty array, got %d: %s", w.Code, w.Body.String())
	}

	server := models.ManagedServer{ID: utils.NewV7UUID(), Name: "Kinsta", Provider: "kinsta", IsLogical: true, SSHPort: 22}
	if err := db.SaveManagedServer(server); err != nil {
		t.Fatalf("failed to seed server: %v", err)
	}

	req = httptest.NewRequest("GET", "/api/servers?format=managed", nil).WithContext(ctx)
	w = httptest.NewRecorder()
	ServersHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var servers []models.ManagedServer
	if err := json.Unmarshal(w.Body.Bytes(), &servers); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "Kinsta" || !servers[0].IsLogical {
		t.Fatalf("unexpected servers list: %+v", servers)
	}
}

func TestSpinupWPEntitiesAreReadOnly(t *testing.T) {
	setupSiteManagementTest(t)
	claims := &AuthClaims{Username: "admin", Level: config.LevelEdit}
	ctx := contextWithClaims(context.Background(), claims)

	serverID := utils.SpinupWPServerUUID(10)
	if err := db.SaveManagedServer(models.ManagedServer{ID: serverID, Name: "spinup1", Provider: "spinupwp", SSHPort: 22}); err != nil {
		t.Fatalf("failed to seed server: %v", err)
	}
	siteID := utils.SpinupWPSiteUUID(20)
	if err := db.SaveManagedSite(models.ManagedSite{
		ID:             siteID,
		ServerID:       &serverID,
		Provider:       "spinupwp",
		Domain:         "spinup-site.com",
		Environment:    models.SiteEnvironmentProduction,
		SSHHost:        "spinup1",
		SSHUser:        "u",
		SitePath:       "files",
		ConnectionType: "agent",
	}); err != nil {
		t.Fatalf("failed to seed site: %v", err)
	}

	body := []byte(`{"ssh_port": 2222}`)
	req := httptest.NewRequest("PUT", "/api/sites/"+siteID, bytes.NewReader(body)).WithContext(ctx)
	req.SetPathValue("id", siteID)
	w := httptest.NewRecorder()
	UpdateSiteHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 on update, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/api/sites/"+siteID, nil).WithContext(ctx)
	req.SetPathValue("id", siteID)
	w = httptest.NewRecorder()
	DeleteSiteHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 on site delete, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/api/servers/"+serverID, nil).WithContext(ctx)
	req.SetPathValue("id", serverID)
	w = httptest.NewRecorder()
	DeleteServerHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 on server delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSiteServerAssignment(t *testing.T) {
	setupSiteManagementTest(t)
	claims := &AuthClaims{Username: "admin", Level: config.LevelEdit}
	ctx := contextWithClaims(context.Background(), claims)

	serverID := utils.NewV7UUID()
	if err := db.SaveManagedServer(models.ManagedServer{ID: serverID, Name: "WPEngine", Provider: "wpengine", IsLogical: true, SSHPort: 22}); err != nil {
		t.Fatalf("failed to seed server: %v", err)
	}

	// Empty server_id on create is treated as unassigned
	body := []byte(`{"domain": "unassigned.com", "server_id": ""}`)
	req := httptest.NewRequest("POST", "/api/sites", bytes.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	CreateSiteHandler(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	body, _ = json.Marshal(CreateSiteRequest{Domain: "assigned.com", ServerID: &serverID})
	req = httptest.NewRequest("POST", "/api/sites", bytes.NewReader(body)).WithContext(ctx)
	w = httptest.NewRecorder()
	CreateSiteHandler(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created models.ManagedSite
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if created.ServerID == nil || *created.ServerID != serverID {
		t.Fatalf("expected server assignment, got %+v", created.ServerID)
	}

	// Server with assigned sites cannot be deleted
	req = httptest.NewRequest("DELETE", "/api/servers/"+serverID, nil).WithContext(ctx)
	req.SetPathValue("id", serverID)
	w = httptest.NewRecorder()
	DeleteServerHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 deleting server with sites, got %d: %s", w.Code, w.Body.String())
	}

	// Empty server_id on update unassigns the site
	body = []byte(`{"server_id": ""}`)
	req = httptest.NewRequest("PUT", "/api/sites/"+created.ID, bytes.NewReader(body)).WithContext(ctx)
	req.SetPathValue("id", created.ID)
	w = httptest.NewRecorder()
	UpdateSiteHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated models.ManagedSite
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if updated.ServerID != nil {
		t.Fatalf("expected server to be unassigned, got %v", *updated.ServerID)
	}

	// Now the server can be deleted
	req = httptest.NewRequest("DELETE", "/api/servers/"+serverID, nil).WithContext(ctx)
	req.SetPathValue("id", serverID)
	w = httptest.NewRecorder()
	DeleteServerHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
