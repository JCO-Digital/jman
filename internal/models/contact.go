package models

import "time"

// AuditFields contains common fields for tracking record creation and modifications.
type AuditFields struct {
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// Organization represents an organization record in the database.
type Organization struct {
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

// Contact represents a contact person tied to an organization.
type Contact struct {
	ID             int         `json:"id"`
	OrganizationID int         `json:"organization_id"`
	Name           string      `json:"name"`
	Email          string      `json:"email"`
	Phone          string      `json:"phone"`
	Type           ContactType `json:"type"`
	AuditFields
}

// SiteOrganizationMap represents the link between a site and an organization.
type SiteOrganizationMap struct {
	SiteID         int       `json:"site_id"`
	OrganizationID int       `json:"organization_id"`
	CreatedAt      time.Time `json:"created_at"`
	CreatedBy      string    `json:"created_by"`
}

// NoteParentType defines what kind of record a note is attached to.
type NoteParentType string

const (
	NoteParentTypeOrganization NoteParentType = "Organization"
	NoteParentTypeSite         NoteParentType = "Site"
	NoteParentTypePlugin       NoteParentType = "Plugin"
)

// Note represents a free-text record linked to an organization or site.
type Note struct {
	ID         int            `json:"id"`
	ParentType NoteParentType `json:"parent_type"`
	ParentID   string         `json:"parent_id"`
	Content    string         `json:"content"`
	AuditFields
}
