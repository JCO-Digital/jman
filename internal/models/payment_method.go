package models

import "time"

// PaymentMethodType defines whether a payment method is used for buying or selling assets.
type PaymentMethodType string

const (
	PaymentMethodTypeBuy  PaymentMethodType = "Buy"
	PaymentMethodTypeSell PaymentMethodType = "Sell"
)

// PaymentMethod represents an account used to pay for (or get paid for) assets,
// e.g. a specific credit card or an invoice account.
type PaymentMethod struct {
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	Type       PaymentMethodType `json:"type"`
	ExpiryDate *time.Time        `json:"expiry_date"`
	AuditFields
}
