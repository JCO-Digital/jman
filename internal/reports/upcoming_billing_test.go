package reports

import (
	"net/url"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func seedOrgAsset(t *testing.T, orgID int, identifier string, nextBilling time.Time, status models.AssetStatus) models.OrganizationAsset {
	t.Helper()
	oa := models.OrganizationAsset{
		OrganizationID: orgID,
		Identifier:     identifier,
		Price:          1000,
		BillingFreq:    models.BillingFrequencyMonthly,
		NextBilling:    &nextBilling,
		Status:         status,
	}
	if err := db.SaveOrganizationAsset(&oa, "tester"); err != nil {
		t.Fatalf("failed to seed organization asset %q: %v", identifier, err)
	}
	return oa
}

func TestUpcomingBillingReport_Run_DefaultsToOneMonthAndIncludesOverdue(t *testing.T) {
	setupReportsTest(t)

	org := models.Organization{Name: "Acme Inc"}
	if err := db.SaveOrganization(&org, "tester"); err != nil {
		t.Fatalf("failed to seed organization: %v", err)
	}

	now := time.Now().UTC()
	overdue := seedOrgAsset(t, org.ID, "overdue.example.com", now.AddDate(0, 0, -10), models.AssetStatusActive)
	upcoming := seedOrgAsset(t, org.ID, "upcoming.example.com", now.AddDate(0, 0, 15), models.AssetStatusActive)
	tooFar := seedOrgAsset(t, org.ID, "too-far.example.com", now.AddDate(0, 0, 45), models.AssetStatusActive)
	paused := seedOrgAsset(t, org.ID, "paused.example.com", now.AddDate(0, 0, 5), models.AssetStatusPaused)

	r := &upcomingBillingReport{}
	result, err := r.Run(url.Values{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	identifiers := map[string]bool{}
	for _, row := range result.Rows {
		identifiers[row["identifier"].(string)] = true
	}

	if !identifiers[overdue.Identifier] {
		t.Errorf("expected overdue asset %q to be included (no lower bound), got rows: %+v", overdue.Identifier, result.Rows)
	}
	if !identifiers[upcoming.Identifier] {
		t.Errorf("expected upcoming asset %q (due in 15 days) to be included within the default one-month window", upcoming.Identifier)
	}
	if identifiers[tooFar.Identifier] {
		t.Errorf("expected asset %q (due in 45 days) to be excluded from the default one-month window", tooFar.Identifier)
	}
	if identifiers[paused.Identifier] {
		t.Errorf("expected paused asset %q to be excluded (only active assets should show)", paused.Identifier)
	}
}

func TestUpcomingBillingReport_Run_ExplicitEndDateWidensWindow(t *testing.T) {
	setupReportsTest(t)

	org := models.Organization{Name: "Acme Inc"}
	if err := db.SaveOrganization(&org, "tester"); err != nil {
		t.Fatalf("failed to seed organization: %v", err)
	}

	now := time.Now().UTC()
	farOut := seedOrgAsset(t, org.ID, "far-out.example.com", now.AddDate(0, 3, 0), models.AssetStatusActive)

	r := &upcomingBillingReport{}
	end := now.AddDate(0, 4, 0).Format("2006-01-02")
	result, err := r.Run(url.Values{"end": {end}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	found := false
	for _, row := range result.Rows {
		if row["identifier"] == farOut.Identifier {
			found = true
		}
	}
	if !found {
		t.Errorf("expected asset due in 3 months to be included when end=%s (4 months out)", end)
	}
}

func TestUpcomingBillingReport_Run_InvalidDateIsBadRequest(t *testing.T) {
	setupReportsTest(t)

	r := &upcomingBillingReport{}
	if _, err := r.Run(url.Values{"end": {"not-a-date"}}); err == nil {
		t.Error("Run() with an invalid end date should error")
	}
}
