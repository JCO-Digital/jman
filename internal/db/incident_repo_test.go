package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupTestAPIDB(t *testing.T) {
	t.Helper()
	tempDir := t.TempDir()
	config.RunData.ConfigDir = tempDir
	config.RunData.DataDir = tempDir

	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Initialize test database
	var err error
	apiDB, err = openDB(filepath.Join(tempDir, "api.db"))
	if err != nil {
		t.Fatalf("failed to open api test db: %v", err)
	}

	if err := initAPISchema(); err != nil {
		t.Fatalf("failed to init api schema: %v", err)
	}
}

func TestIncidentLifecycle(t *testing.T) {
	setupTestAPIDB(t)

	domain := "example.com"
	downSince := time.Now().Add(-5 * time.Minute)

	// 1. Create incident
	inc, err := CreateIncident(domain, "HTTP 500 Internal Server Error", 500, downSince)
	if err != nil {
		t.Fatalf("CreateIncident failed: %v", err)
	}
	if inc.Status != models.IncidentStatusOpen {
		t.Errorf("expected status 'open', got '%s'", inc.Status)
	}
	if inc.Domain != domain {
		t.Errorf("expected domain '%s', got '%s'", domain, inc.Domain)
	}

	// 2. Count active
	count, err := GetActiveIncidentCount()
	if err != nil {
		t.Fatalf("GetActiveIncidentCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected active count 1, got %d", count)
	}

	// 3. Acknowledge incident
	ackUser := "johndoe"
	ackInc, err := AcknowledgeIncident(inc.ID, ackUser)
	if err != nil {
		t.Fatalf("AcknowledgeIncident failed: %v", err)
	}
	if ackInc.Status != models.IncidentStatusAcknowledged {
		t.Errorf("expected status 'acknowledged', got '%s'", ackInc.Status)
	}
	if ackInc.AcknowledgedBy == nil || *ackInc.AcknowledgedBy != ackUser {
		t.Errorf("expected acknowledged_by '%s', got '%v'", ackUser, ackInc.AcknowledgedBy)
	}

	// 4. Resolve incident
	resInc, err := ResolveIncident(inc.ID, "system")
	if err != nil {
		t.Fatalf("ResolveIncident failed: %v", err)
	}
	if resInc.Status != models.IncidentStatusResolved {
		t.Errorf("expected status 'resolved', got '%s'", resInc.Status)
	}

	// 5. Active count should be 0
	count, err = GetActiveIncidentCount()
	if err != nil {
		t.Fatalf("GetActiveIncidentCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected active count 0, got %d", count)
	}

	// 6. Test Close
	inc2, err := CreateIncident("test2.com", "Timeout", 0, time.Now())
	if err != nil {
		t.Fatalf("CreateIncident 2 failed: %v", err)
	}
	closedInc, err := CloseIncident(inc2.ID, "admin")
	if err != nil {
		t.Fatalf("CloseIncident failed: %v", err)
	}
	if closedInc.Status != models.IncidentStatusClosed {
		t.Errorf("expected status 'closed', got '%s'", closedInc.Status)
	}
}
