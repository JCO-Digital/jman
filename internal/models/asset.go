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
	ID                int              `json:"id"`
	Type              AssetType        `json:"type"`
	Identifier        string           `json:"identifier"` // TLD for domains, slug for plugins
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	DefaultPrice      int              `json:"default_price"` // stored in cents, what we bill the client
	DefaultFreq       BillingFrequency `json:"default_freq"`
	Active            bool             `json:"active"`
	PaymentMethodID   *int             `json:"payment_method_id,omitempty"`
	PaymentMethodName string           `json:"payment_method_name,omitempty"` // denormalized, not persisted
	PurchasePrice     int              `json:"purchase_price"`                // stored in cents, what we pay the vendor
	Quantity          int              `json:"quantity"`
	NextPayment       *time.Time       `json:"next_payment"`
	ManagementURL     string           `json:"management_url"`
	ManagementAccount string           `json:"management_account"` // email the purchase/account is managed under
	UsageCount        int              `json:"usage_count"`        // number of linked organization_assets, only populated by GetAllAssets
	AuditFields
}

// OrganizationAsset represents a specific instance of an asset linked to an organization.
type OrganizationAsset struct {
	ID                int              `json:"id"`
	OrganizationID    int              `json:"organization_id"`
	SiteID            *int             `json:"site_id,omitempty"`  // Optional link to a site
	AssetID           *int             `json:"asset_id,omitempty"` // Reference to the template
	Identifier        string           `json:"identifier"`         // Specific domain name, product name, etc.
	Price             int              `json:"price"`              // stored in cents
	BillingFreq       BillingFrequency `json:"billing_freq"`
	NextBilling       *time.Time       `json:"next_billing"`
	Status            AssetStatus      `json:"status"`
	Description       string           `json:"description"`
	PaymentMethodID   *int             `json:"payment_method_id,omitempty"`
	OrganizationName  string           `json:"organization_name,omitempty"`
	AssetName         string           `json:"asset_name,omitempty"`
	AssetType         string           `json:"asset_type,omitempty"`
	PaymentMethodName string           `json:"payment_method_name,omitempty"`
	// Read-only mirrors of the linked template's cost fields, for display only.
	AssetPurchasePrice     int        `json:"asset_purchase_price,omitempty"`
	AssetQuantity          int        `json:"asset_quantity,omitempty"`
	AssetNextPayment       *time.Time `json:"asset_next_payment,omitempty"`
	AssetManagementURL     string     `json:"asset_management_url,omitempty"`
	AssetManagementAccount string     `json:"asset_management_account,omitempty"`
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
