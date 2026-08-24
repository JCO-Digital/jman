package reports

import (
	"fmt"
	"net/url"

	"github.com/JCO-Digital/jman/internal/db"
)

// maxAssetBillingReportDays bounds the report's date range; billing history
// is cheap to query so this is generous compared to the traffic report.
const maxAssetBillingReportDays = 3660

type assetBillingReport struct{}

func init() {
	Register(&assetBillingReport{})
}

func (r *assetBillingReport) ID() string   { return "asset-billing" }
func (r *assetBillingReport) Name() string { return "Asset & Billing Ledger" }
func (r *assetBillingReport) Description() string {
	return "Payments recorded against billable organization assets for the selected date range."
}

func (r *assetBillingReport) Params() []ParamDef {
	return []ParamDef{
		{Key: "range", Type: ParamDateRange, Label: "Date range", Required: false},
	}
}

func (r *assetBillingReport) Run(q url.Values) (*Result, error) {
	start, end, err := ParseDateRange(q, maxAssetBillingReportDays)
	if err != nil {
		return nil, err
	}

	payments, err := db.GetAssetPaymentsInRange(start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to load asset payments: %w", err)
	}

	rows := make([]map[string]any, 0, len(payments))
	for _, p := range payments {
		rows = append(rows, map[string]any{
			"organization": p.OrganizationName,
			"identifier":   p.Identifier,
			"asset_name":   p.AssetName,
			"asset_type":   p.AssetType,
			"billing_freq": p.BillingFreq,
			"status":       p.Status,
			"amount":       p.Amount,
			"payment_date": p.PaymentDate.Format(dateLayout),
			"info":         p.Info,
		})
	}

	return &Result{
		Columns: []Column{
			{Key: "organization", Label: "Organization", Type: ColumnText},
			{Key: "identifier", Label: "Identifier", Type: ColumnText},
			{Key: "asset_name", Label: "Asset", Type: ColumnText},
			{Key: "asset_type", Label: "Type", Type: ColumnText},
			{Key: "billing_freq", Label: "Billing Frequency", Type: ColumnText},
			{Key: "status", Label: "Status", Type: ColumnText},
			{Key: "amount", Label: "Amount", Type: ColumnCurrency},
			{Key: "payment_date", Label: "Payment Date", Type: ColumnDate},
			{Key: "info", Label: "Info", Type: ColumnText},
		},
		Rows: rows,
	}, nil
}
