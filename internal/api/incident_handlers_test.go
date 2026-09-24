package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupIncidentTest(t *testing.T) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "jman-incident-test-*")
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

func TestIncidentsAPI(t *testing.T) {
	setupIncidentTest(t)
	username := "testops"
	claims := &AuthClaims{Username: username, Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	// 1. Initially no incidents
	req := httptest.NewRequest("GET", "/api/incidents", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	ListIncidentsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}
	var listResp IncidentsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if listResp.Total != 0 || listResp.ActiveCount != 0 {
		t.Errorf("Expected 0 total and active, got total=%d active=%d", listResp.Total, listResp.ActiveCount)
	}

	// 2. Create an incident in DB
	inc, err := db.CreateIncident("api-site.com", "HTTP 502 Bad Gateway", 502, time.Now())
	if err != nil {
		t.Fatalf("Failed to create incident: %v", err)
	}

	// 3. List active incidents
	req = httptest.NewRequest("GET", "/api/incidents", nil).WithContext(ctx)
	w = httptest.NewRecorder()
	ListIncidentsHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.Total != 1 || listResp.ActiveCount != 1 {
		t.Errorf("Expected 1 incident, got total=%d active=%d", listResp.Total, listResp.ActiveCount)
	}

	// 4. Acknowledge incident
	ackURL := fmt.Sprintf("/api/incidents/%d/acknowledge", inc.ID)
	req = httptest.NewRequest("POST", ackURL, nil).WithContext(ctx)
	req.SetPathValue("id", fmt.Sprintf("%d", inc.ID))
	w = httptest.NewRecorder()
	AcknowledgeIncidentHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on ack, got %d: %s", w.Code, w.Body.String())
	}
	var acked models.Incident
	json.Unmarshal(w.Body.Bytes(), &acked)
	if acked.Status != models.IncidentStatusAcknowledged {
		t.Errorf("Expected status acknowledged, got %s", acked.Status)
	}
	if acked.AcknowledgedBy == nil || *acked.AcknowledgedBy != username {
		t.Errorf("Expected ack by %s, got %v", username, acked.AcknowledgedBy)
	}

	// 5. Close incident
	closeURL := fmt.Sprintf("/api/incidents/%d/close", inc.ID)
	req = httptest.NewRequest("POST", closeURL, nil).WithContext(ctx)
	req.SetPathValue("id", fmt.Sprintf("%d", inc.ID))
	w = httptest.NewRecorder()
	CloseIncidentHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on close, got %d: %s", w.Code, w.Body.String())
	}
	var closed models.Incident
	json.Unmarshal(w.Body.Bytes(), &closed)
	if closed.Status != models.IncidentStatusClosed {
		t.Errorf("Expected status closed, got %s", closed.Status)
	}

	// 6. Check active count is now 0
	req = httptest.NewRequest("GET", "/api/incidents", nil).WithContext(ctx)
	w = httptest.NewRecorder()
	ListIncidentsHandler(w, req)
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.ActiveCount != 0 {
		t.Errorf("Expected active count 0, got %d", listResp.ActiveCount)
	}
}
