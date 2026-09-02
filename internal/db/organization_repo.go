package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// --- Organization Repository ---

func SaveOrganization(org *models.Organization, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if org.ID == 0 {
		query := `
		INSERT INTO organizations (name, vat_number, info, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, org.Name, org.VATNumber, org.Info, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert organization: %w", err)
		}
		id, _ := result.LastInsertId()
		org.ID = int(id)
		org.CreatedAt = now
		org.CreatedBy = username
		org.UpdatedAt = now
		org.UpdatedBy = username
	} else {
		query := `
		UPDATE organizations SET name = ?, vat_number = ?, info = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, org.Name, org.VATNumber, org.Info, now, username, org.ID)
		if err != nil {
			return fmt.Errorf("failed to update organization: %w", err)
		}
		org.UpdatedAt = now
		org.UpdatedBy = username
	}
	return nil
}

func GetOrganization(id int) (*models.Organization, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, name, vat_number, info, created_at, created_by, updated_at, updated_by FROM organizations WHERE id = ?`
	var o models.Organization
	err := db.QueryRow(query, id).Scan(
		&o.ID, &o.Name, &o.VATNumber, &o.Info, &o.CreatedAt, &o.CreatedBy, &o.UpdatedAt, &o.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	return &o, nil
}

func GetAllOrganizations(search string) ([]models.Organization, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, name, vat_number, info, created_at, created_by, updated_at, updated_by FROM organizations`
	var args []interface{}
	if search != "" {
		query += " WHERE name LIKE ? OR vat_number LIKE ?"
		term := "%" + search + "%"
		args = append(args, term, term)
	}
	query += " ORDER BY name ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query organizations: %w", err)
	}
	defer rows.Close()

	organizations := []models.Organization{}
	for rows.Next() {
		var o models.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.VATNumber, &o.Info, &o.CreatedAt, &o.CreatedBy, &o.UpdatedAt, &o.UpdatedBy); err != nil {
			return nil, err
		}
		organizations = append(organizations, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return organizations, nil
}

func DeleteOrganization(id int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM organizations WHERE id = ?", id)
	return err
}

// --- Contact Repository ---

func SaveContact(contact *models.Contact, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if contact.ID == 0 {
		query := `
		INSERT INTO contacts (organization_id, name, email, phone, type, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, contact.OrganizationID, contact.Name, contact.Email, contact.Phone, contact.Type, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert contact: %w", err)
		}
		id, _ := result.LastInsertId()
		contact.ID = int(id)
		contact.CreatedAt = now
		contact.CreatedBy = username
		contact.UpdatedAt = now
		contact.UpdatedBy = username
	} else {
		query := `
		UPDATE contacts SET name = ?, email = ?, phone = ?, type = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, contact.Name, contact.Email, contact.Phone, contact.Type, now, username, contact.ID)
		if err != nil {
			return fmt.Errorf("failed to update contact: %w", err)
		}
		contact.UpdatedAt = now
		contact.UpdatedBy = username
	}
	return nil
}

func GetContact(id int) (*models.Contact, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, organization_id, name, email, phone, type, created_at, created_by, updated_at, updated_by FROM contacts WHERE id = ?`
	var c models.Contact
	err := db.QueryRow(query, id).Scan(
		&c.ID, &c.OrganizationID, &c.Name, &c.Email, &c.Phone, &c.Type, &c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	return &c, nil
}

func GetContactsByOrganization(organizationID int) ([]models.Contact, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, organization_id, name, email, phone, type, created_at, created_by, updated_at, updated_by FROM contacts WHERE organization_id = ? ORDER BY name ASC`
	rows, err := db.Query(query, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contacts := []models.Contact{}
	for rows.Next() {
		var c models.Contact
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Email, &c.Phone, &c.Type, &c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contacts, nil
}

func DeleteContact(id int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM contacts WHERE id = ?", id)
	return err
}

// --- Site-Organization Mapping Repository ---

func LinkSiteToOrganization(siteID, organizationID int, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_organization_map (site_id, organization_id, created_by)
	VALUES (?, ?, ?)
	ON CONFLICT(site_id, organization_id) DO NOTHING
	`
	_, err := db.Exec(query, siteID, organizationID, username)
	return err
}

func UnlinkSiteFromOrganization(siteID, organizationID int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM site_organization_map WHERE site_id = ? AND organization_id = ?`
	_, err := db.Exec(query, siteID, organizationID)
	return err
}

func GetOrganizationBySite(siteID int) (*models.Organization, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT o.id, o.name, o.vat_number, o.info, o.created_at, o.created_by, o.updated_at, o.updated_by
	FROM organizations o
	JOIN site_organization_map m ON o.id = m.organization_id
	WHERE m.site_id = ?
	`
	var o models.Organization
	err := db.QueryRow(query, siteID).Scan(
		&o.ID, &o.Name, &o.VATNumber, &o.Info, &o.CreatedAt, &o.CreatedBy, &o.UpdatedAt, &o.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get organization for site: %w", err)
	}
	return &o, nil
}

func GetSitesByOrganization(organizationID int) ([]int, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT site_id FROM site_organization_map WHERE organization_id = ?`
	rows, err := db.Query(query, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var siteIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		siteIDs = append(siteIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return siteIDs, nil
}

// --- Notes Repository ---

func SaveNote(note *models.Note, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if note.ID == 0 {
		query := `
		INSERT INTO notes (parent_type, parent_id, content, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, note.ParentType, note.ParentID, note.Content, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert note: %w", err)
		}
		id, _ := result.LastInsertId()
		note.ID = int(id)
		note.CreatedAt = now
		note.CreatedBy = username
		note.UpdatedAt = now
		note.UpdatedBy = username
	} else {
		query := `
		UPDATE notes SET content = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, note.Content, now, username, note.ID)
		if err != nil {
			return fmt.Errorf("failed to update note: %w", err)
		}
		note.UpdatedAt = now
		note.UpdatedBy = username
	}
	return nil
}

func GetNotes(parentType models.NoteParentType, parentID string) ([]models.Note, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, parent_type, parent_id, content, created_at, created_by, updated_at, updated_by FROM notes WHERE parent_type = ? AND parent_id = ? ORDER BY created_at DESC`
	rows, err := db.Query(query, string(parentType), parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := []models.Note{}
	for rows.Next() {
		var n models.Note
		if err := rows.Scan(&n.ID, &n.ParentType, &n.ParentID, &n.Content, &n.CreatedAt, &n.CreatedBy, &n.UpdatedAt, &n.UpdatedBy); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return notes, nil
}

func DeleteNote(id int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM notes WHERE id = ?", id)
	return err
}

func GetNote(id int) (*models.Note, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, parent_type, parent_id, content, created_at, created_by, updated_at, updated_by FROM notes WHERE id = ?`
	var n models.Note
	err := db.QueryRow(query, id).Scan(
		&n.ID, &n.ParentType, &n.ParentID, &n.Content, &n.CreatedAt, &n.CreatedBy, &n.UpdatedAt, &n.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get note: %w", err)
	}
	return &n, nil
}
