package models

import "time"

// AuditFields contains common fields for tracking record creation and modifications.
type AuditFields struct {
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// Company represents a company record in the database.
type Company struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	VATNumber string `json:"vat_number"`
	Info      string `json:"info"`
	AuditFields
}

// ContactType defines the role of a contact person.
type ContactType string

const (
	ContactTypeMain      ContactType = "Main"
	ContactTypeTechnical ContactType = "Technical"
	ContactTypeBilling   ContactType = "Billing"
)

// Contact represents a contact person tied to a company.
type Contact struct {
	ID        int         `json:"id"`
	CompanyID int         `json:"company_id"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	Phone     string      `json:"phone"`
	Type      ContactType `json:"type"`
	AuditFields
}

// SiteCompanyMap represents the link between a site and a company.
type SiteCompanyMap struct {
	SiteID    int       `json:"site_id"`
	CompanyID int       `json:"company_id"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

// NoteParentType defines what kind of record a note is attached to.
type NoteParentType string

const (
	NoteParentTypeCompany NoteParentType = "Company"
	NoteParentTypeSite    NoteParentType = "Site"
)

// Note represents a free-text record linked to a company or site.
type Note struct {
	ID         int            `json:"id"`
	ParentType NoteParentType `json:"parent_type"`
	ParentID   int            `json:"parent_id"`
	Content    string         `json:"content"`
	AuditFields
}
