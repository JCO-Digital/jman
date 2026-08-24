package reports

import (
	"net/url"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestAssetBillingReport_Run(t *testing.T) {
	setupReportsTest(t)

	org := models.Organization{Name: "Acme Inc"}
	if err := db.SaveOrganization(&org, "tester"); err != nil {
		t.Fatalf("failed to seed organization: %v", err)
	}

	// No linked asset template, to exercise the LEFT JOIN's NULL handling.
	oa := models.OrganizationAsset{
		OrganizationID: org.ID,
		Identifier:     "acme.example.com",
		Price:          2000,
		BillingFreq:    models.BillingFrequencyMonthly,
		Status:         models.AssetStatusActive,
	}
	if err := db.SaveOrganizationAsset(&oa, "tester"); err != nil {
		t.Fatalf("failed to seed organization asset: %v", err)
	}

	payment := models.AssetPayment{
		OrgAssetID:  oa.ID,
		Amount:      2000,
		PaymentDate: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		Info:        "January renewal",
	}
	if err := db.SaveAssetPayment(&payment, "tester"); err != nil {
		t.Fatalf("failed to seed payment: %v", err)
	}

	r := &assetBillingReport{}
	result, err := r.Run(url.Values{"start": {"2026-01-01"}, "end": {"2026-01-31"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %+v", len(result.Rows), result.Rows)
	}
	row := result.Rows[0]
	if row["organization"] != "Acme Inc" {
		t.Errorf("row[organization] = %v, want Acme Inc", row["organization"])
	}
	if row["amount"] != 2000 {
		t.Errorf("row[amount] = %v, want 2000", row["amount"])
	}
	if row["asset_name"] != "" {
		t.Errorf("row[asset_name] = %v, want empty (no linked template)", row["asset_name"])
	}
}

func TestAssetBillingReport_Run_InvalidDateIsBadRequest(t *testing.T) {
	setupReportsTest(t)

	r := &assetBillingReport{}
	if _, err := r.Run(url.Values{"end": {"not-a-date"}}); err == nil {
		t.Error("Run() with an invalid end date should error")
	}
}
