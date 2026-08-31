package db

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestGetAssetPaymentsInRange(t *testing.T) {
	setupTaskRepoTest(t)

	org := models.Organization{Name: "Acme Inc"}
	if err := SaveOrganization(&org, "tester"); err != nil {
		t.Fatalf("failed to seed organization: %v", err)
	}

	asset := models.Asset{Type: models.AssetTypePlugin, Name: "Yoast SEO", Identifier: "wordpress-seo"}
	if err := SaveAsset(&asset, "tester"); err != nil {
		t.Fatalf("failed to seed asset template: %v", err)
	}

	oaWithTemplate := models.OrganizationAsset{
		OrganizationID: org.ID,
		AssetID:        &asset.ID,
		Identifier:     "acme.example.com",
		Price:          1000,
		BillingFreq:    models.BillingFrequencyMonthly,
		Status:         models.AssetStatusActive,
	}
	if err := SaveOrganizationAsset(&oaWithTemplate, "tester"); err != nil {
		t.Fatalf("failed to seed organization asset (with template): %v", err)
	}

	// No linked template — exercises the LEFT JOIN's NULL asset_name/asset_type path.
	oaCustom := models.OrganizationAsset{
		OrganizationID: org.ID,
		Identifier:     "custom-service",
		Price:          500,
		BillingFreq:    models.BillingFrequencyOneTime,
		Status:         models.AssetStatusActive,
	}
	if err := SaveOrganizationAsset(&oaCustom, "tester"); err != nil {
		t.Fatalf("failed to seed organization asset (custom): %v", err)
	}

	inRange := models.AssetPayment{
		OrgAssetID:  oaWithTemplate.ID,
		Amount:      1000,
		PaymentDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Info:        "January renewal",
	}
	if err := SaveAssetPayment(&inRange, "tester"); err != nil {
		t.Fatalf("failed to seed in-range payment: %v", err)
	}

	customInRange := models.AssetPayment{
		OrgAssetID:  oaCustom.ID,
		Amount:      500,
		PaymentDate: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
		Info:        "One-off",
	}
	if err := SaveAssetPayment(&customInRange, "tester"); err != nil {
		t.Fatalf("failed to seed custom in-range payment: %v", err)
	}

	outOfRange := models.AssetPayment{
		OrgAssetID:  oaWithTemplate.ID,
		Amount:      1000,
		PaymentDate: time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC),
		Info:        "December renewal",
	}
	if err := SaveAssetPayment(&outOfRange, "tester"); err != nil {
		t.Fatalf("failed to seed out-of-range payment: %v", err)
	}

	rows, err := GetAssetPaymentsInRange("2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatalf("GetAssetPaymentsInRange() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows in range, got %d: %+v", len(rows), rows)
	}

	byInfo := map[string]models.AssetPaymentReportRow{}
	for _, r := range rows {
		byInfo[r.Info] = r
	}

	templated, ok := byInfo["January renewal"]
	if !ok {
		t.Fatalf("expected a row for the templated payment, got %+v", rows)
	}
	if templated.OrganizationName != "Acme Inc" {
		t.Errorf("templated.OrganizationName = %q, want %q", templated.OrganizationName, "Acme Inc")
	}
	if templated.AssetName != "Yoast SEO" || templated.AssetType != string(models.AssetTypePlugin) {
		t.Errorf("templated asset name/type = %q/%q, want Yoast SEO/Plugin", templated.AssetName, templated.AssetType)
	}
	if templated.Amount != 1000 {
		t.Errorf("templated.Amount = %d, want 1000", templated.Amount)
	}

	custom, ok := byInfo["One-off"]
	if !ok {
		t.Fatalf("expected a row for the custom (no-template) payment, got %+v", rows)
	}
	if custom.AssetName != "" || custom.AssetType != "" {
		t.Errorf("custom asset name/type = %q/%q, want both empty (no linked template)", custom.AssetName, custom.AssetType)
	}
	if custom.OrganizationName != "Acme Inc" {
		t.Errorf("custom.OrganizationName = %q, want %q", custom.OrganizationName, "Acme Inc")
	}
}

func TestAssetLicenseKeys(t *testing.T) {
	setupTaskRepoTest(t)

	org := models.Organization{Name: "Acme Corp"}
	if err := SaveOrganization(&org, "tester"); err != nil {
		t.Fatalf("failed to seed organization: %v", err)
	}

	// 1. Create template with a license key
	template := models.Asset{
		Type:       models.AssetTypePlugin,
		Name:       "WP Rocket",
		Identifier: "wp-rocket",
		LicenseKey: "template_key_123",
	}
	if err := SaveAsset(&template, "tester"); err != nil {
		t.Fatalf("failed to save asset template: %v", err)
	}

	retrievedTemplate, err := GetAsset(template.ID)
	if err != nil {
		t.Fatalf("failed to get asset template: %v", err)
	}
	if retrievedTemplate.LicenseKey != "template_key_123" {
		t.Errorf("expected template LicenseKey 'template_key_123', got %q", retrievedTemplate.LicenseKey)
	}

	// 2. Create organization asset with empty license key (should inherit from template)
	oaInherited := models.OrganizationAsset{
		OrganizationID: org.ID,
		AssetID:        &template.ID,
		Identifier:     "inherited-license.example.com",
		Price:          1500,
		BillingFreq:    models.BillingFrequencyMonthly,
		Status:         models.AssetStatusActive,
		LicenseKey:     "", // empty means inherit
	}
	if err := SaveOrganizationAsset(&oaInherited, "tester"); err != nil {
		t.Fatalf("failed to save organization asset: %v", err)
	}

	retrievedOaInherited, err := GetOrganizationAsset(oaInherited.ID)
	if err != nil {
		t.Fatalf("failed to get organization asset: %v", err)
	}
	if retrievedOaInherited.LicenseKey != "" {
		t.Errorf("expected organization asset LicenseKey to be empty, got %q", retrievedOaInherited.LicenseKey)
	}
	if retrievedOaInherited.AssetLicenseKey != "template_key_123" {
		t.Errorf("expected mirrored AssetLicenseKey 'template_key_123', got %q", retrievedOaInherited.AssetLicenseKey)
	}

	// 3. Create organization asset with an overridden license key
	oaOverridden := models.OrganizationAsset{
		OrganizationID: org.ID,
		AssetID:        &template.ID,
		Identifier:     "overridden-license.example.com",
		Price:          1500,
		BillingFreq:    models.BillingFrequencyMonthly,
		Status:         models.AssetStatusActive,
		LicenseKey:     "overridden_key_456",
	}
	if err := SaveOrganizationAsset(&oaOverridden, "tester"); err != nil {
		t.Fatalf("failed to save organization asset with overridden key: %v", err)
	}

	retrievedOaOverridden, err := GetOrganizationAsset(oaOverridden.ID)
	if err != nil {
		t.Fatalf("failed to get organization asset: %v", err)
	}
	if retrievedOaOverridden.LicenseKey != "overridden_key_456" {
		t.Errorf("expected overridden LicenseKey 'overridden_key_456', got %q", retrievedOaOverridden.LicenseKey)
	}
	if retrievedOaOverridden.AssetLicenseKey != "template_key_123" {
		t.Errorf("expected mirrored AssetLicenseKey 'template_key_123' even when overridden, got %q", retrievedOaOverridden.AssetLicenseKey)
	}
}
