package models

import "time"

// AssetType defines the category of the asset.
type AssetType string

const (
	AssetTypePlugin         AssetType = "Plugin"
	AssetTypeDomain         AssetType = "Domain"
	AssetTypeHostingPackage AssetType = "Hosting Package"
	AssetTypeServicePackage AssetType = "Service Package"
	AssetTypeGeneral        AssetType = "General"
)

// BillingFrequency defines how often an asset is billed.
type BillingFrequency string

const (
	BillingFrequencyYearly    BillingFrequency = "Yearly"
	BillingFrequencyQuarterly BillingFrequency = "Quarterly"
	BillingFrequencyMonthly   BillingFrequency = "Monthly"
	BillingFrequencyOneTime   BillingFrequency = "One-time"
)

// AssetStatus defines the status of a linked asset.
type AssetStatus string

const (
	AssetStatusActive    AssetStatus = "active"
	AssetStatusCancelled AssetStatus = "cancelled"
	AssetStatusPaused    AssetStatus = "paused"
)

// Asset represents a generic product or service template.
type Asset struct {
	ID           int              `json:"id"`
	Type         AssetType        `json:"type"`
	Identifier   string           `json:"identifier"` // TLD for domains, slug for plugins
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	DefaultPrice int              `json:"default_price"` // stored in cents
	DefaultFreq  BillingFrequency `json:"default_freq"`
	Active       bool             `json:"active"`
	AuditFields
}

// OrganizationAsset represents a specific instance of an asset linked to an organization.
type OrganizationAsset struct {
	ID             int              `json:"id"`
	OrganizationID int              `json:"organization_id"`
	SiteID         *int             `json:"site_id,omitempty"`  // Optional link to a site
	AssetID        *int             `json:"asset_id,omitempty"` // Reference to the template
	Identifier     string           `json:"identifier"`         // Specific domain name, product name, etc.
	Price          int              `json:"price"`              // stored in cents
	BillingFreq    BillingFrequency `json:"billing_freq"`
	NextBilling    *time.Time       `json:"next_billing"`
	Status         AssetStatus      `json:"status"`
	Description    string           `json:"description"`
	AuditFields
}

// AssetPayment tracks historical payment records for an organization asset.
type AssetPayment struct {
	ID          int       `json:"id"`
	OrgAssetID  int       `json:"org_asset_id"`
	Amount      int       `json:"amount"` // stored in cents
	PaymentDate time.Time `json:"payment_date"`
	Info        string    `json:"info"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
}
