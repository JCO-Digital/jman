package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestPauseAndResumeMonitorCheck(t *testing.T) {
	setupIncidentTest(t)
	ctx := contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit})

	check := models.MonitorCheck{Host: "api.example", Path: "/healthz", Enabled: true}
	if err := db.CreateMonitorCheck(&check, "test"); err != nil {
		t.Fatal(err)
	}
	inc, err := db.CreateIncident(check.ID, "api.example", "HTTP 503", 503, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/monitor/checks/"+check.ID+"/pause", strings.NewReader(`{"reason":"maintenance"}`)).WithContext(ctx)
	req.SetPathValue("id", check.ID)
	w := httptest.NewRecorder()
	PauseMonitorCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pause: %d %s", w.Code, w.Body.String())
	}
	var paused MonitorCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &paused); err != nil {
		t.Fatal(err)
	}
	if paused.Enabled || paused.URL != "https://api.example/healthz" {
		t.Errorf("pause response = %+v", paused)
	}
	if got, _ := db.GetIncidentByID(inc.ID); got == nil || got.Status != models.IncidentStatusResolved || got.ResolvedBy == nil || *got.ResolvedBy != "alice" {
		t.Errorf("incident after pause = %+v, want resolved by alice", got)
	}

	req = httptest.NewRequest("POST", "/api/monitor/checks/"+check.ID+"/resume", nil).WithContext(ctx)
	req.SetPathValue("id", check.ID)
	w = httptest.NewRecorder()
	ResumeMonitorCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", w.Code, w.Body.String())
	}
	if got, _ := db.GetMonitorCheck(check.ID); got == nil || !got.Enabled {
		t.Errorf("check after resume = %+v, want enabled", got)
	}
}

func TestPauseIncidentCheck(t *testing.T) {
	setupIncidentTest(t)
	ctx := contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit})

	check := models.MonitorCheck{Host: "shop.example", Enabled: true}
	if err := db.CreateMonitorCheck(&check, "test"); err != nil {
		t.Fatal(err)
	}
	inc, err := db.CreateIncident(check.ID, "shop.example", "HTTP 500", 500, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("/api/incidents/%d/pause", inc.ID)
	req := httptest.NewRequest("POST", url, nil).WithContext(ctx)
	req.SetPathValue("id", fmt.Sprintf("%d", inc.ID))
	w := httptest.NewRecorder()
	PauseIncidentCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pause from incident: %d %s", w.Code, w.Body.String())
	}
	var got models.Incident
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != models.IncidentStatusResolved {
		t.Errorf("incident status = %s, want resolved", got.Status)
	}
	if c, _ := db.GetMonitorCheck(check.ID); c == nil || c.Enabled {
		t.Errorf("check after pause = %+v, want paused", c)
	}
}
