package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestSiteTrafficHandler_Monthly(t *testing.T) {
	setupSettingsTest(t)

	const siteID = 3
	day := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if err := db.UpsertSiteTrafficDaily(siteID, models.TrafficDailyEntry{
		Day: day, RequestsTotal: 42, RequestsHuman: 40, RequestsBot: 2, UniqueVisitors: 5,
	}); err != nil {
		t.Fatalf("failed to seed daily entry: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/sites/3/traffic?period=monthly&days=400", nil)
	req.SetPathValue("id", "3")
	w := httptest.NewRecorder()
	SiteTrafficHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	var traffic []models.SiteTrafficPeriod
	if err := json.Unmarshal(w.Body.Bytes(), &traffic); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(traffic) != 1 {
		t.Fatalf("expected 1 monthly row, got %d", len(traffic))
	}
	if traffic[0].RequestsTotal != 42 {
		t.Errorf("RequestsTotal = %d, want 42", traffic[0].RequestsTotal)
	}
	// days=400 exceeds maxSiteTrafficMonthlyDays but must still be honored
	// up to the cap rather than rejected, since the seeded day is well
	// within maxSiteTrafficMonthlyDays regardless.
}

func TestSiteTrafficHandler_UnknownPeriodDefaultsToHourly(t *testing.T) {
	setupSettingsTest(t)

	req := httptest.NewRequest("GET", "/api/sites/3/traffic?period=weekly", nil)
	req.SetPathValue("id", "3")
	w := httptest.NewRecorder()
	SiteTrafficHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	var traffic []models.SiteTrafficPeriod
	if err := json.Unmarshal(w.Body.Bytes(), &traffic); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(traffic) != 0 {
		t.Fatalf("expected no rows for an unseeded site, got %d", len(traffic))
	}
}
