package reports

import (
	"fmt"
	"net/url"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

type upcomingBillingReport struct{}

func init() {
	Register(&upcomingBillingReport{})
}

func (r *upcomingBillingReport) ID() string   { return "upcoming-billing" }
func (r *upcomingBillingReport) Name() string { return "Upcoming Asset Billing" }
func (r *upcomingBillingReport) Description() string {
	return "Active organization assets due to be billed by the selected date, including any already overdue."
}

func (r *upcomingBillingReport) Params() []ParamDef {
	return []ParamDef{
		{Key: "end", Type: ParamEndDate, Label: "Show billing due before", Required: false},
	}
}

func (r *upcomingBillingReport) Run(q url.Values) (*Result, error) {
	end, err := ParseEndDate(q, time.Now().UTC().AddDate(0, 1, 0))
	if err != nil {
		return nil, err
	}

	// No lower bound — overdue assets (next_billing far in the past) are
	// deliberately included alongside upcoming ones. before is extended to
	// the end of its day, matching the asset-billing report's handling of
	// text-compared DATETIME columns (see GetAssetPaymentsInRange), so an
	// asset due exactly on the cutoff day isn't excluded by its time-of-day.
	assets, err := db.GetAllOrganizationAssets("", string(models.AssetStatusActive), end+"T23:59:59Z")
	if err != nil {
		return nil, fmt.Errorf("failed to load organization assets: %w", err)
	}

	rows := make([]map[string]any, 0, len(assets))
	for _, a := range assets {
		nextBilling := ""
		if a.NextBilling != nil {
			nextBilling = a.NextBilling.Format(dateLayout)
		}
		rows = append(rows, map[string]any{
			"organization": a.OrganizationName,
			"identifier":   a.Identifier,
			"asset_name":   a.AssetName,
			"asset_type":   a.AssetType,
			"billing_freq": a.BillingFreq,
			"price":        a.Price,
			"next_billing": nextBilling,
			"status":       a.Status,
		})
	}

	return &Result{
		Columns: []Column{
			{Key: "organization", Label: "Organization", Type: ColumnText},
			{Key: "identifier", Label: "Identifier", Type: ColumnText},
			{Key: "asset_name", Label: "Asset", Type: ColumnText},
			{Key: "asset_type", Label: "Type", Type: ColumnText},
			{Key: "billing_freq", Label: "Billing Frequency", Type: ColumnText},
			{Key: "price", Label: "Price", Type: ColumnCurrency},
			{Key: "next_billing", Label: "Next Billing", Type: ColumnDate},
			{Key: "status", Label: "Status", Type: ColumnText},
		},
		Rows: rows,
	}, nil
}
