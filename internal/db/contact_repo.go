package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// --- Company Repository ---

func SaveCompany(company *models.Company, username string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if company.ID == 0 {
		query := `
		INSERT INTO companies (name, vat_number, info, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, company.Name, company.VATNumber, company.Info, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert company: %w", err)
		}
		id, _ := result.LastInsertId()
		company.ID = int(id)
		company.CreatedAt = now
		company.CreatedBy = username
		company.UpdatedAt = now
		company.UpdatedBy = username
	} else {
		query := `
		UPDATE companies SET name = ?, vat_number = ?, info = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, company.Name, company.VATNumber, company.Info, now, username, company.ID)
		if err != nil {
			return fmt.Errorf("failed to update company: %w", err)
		}
		company.UpdatedAt = now
		company.UpdatedBy = username
	}
	return nil
}

func GetCompany(id int) (*models.Company, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, name, vat_number, info, created_at, created_by, updated_at, updated_by FROM companies WHERE id = ?`
	var c models.Company
	err := db.QueryRow(query, id).Scan(
		&c.ID, &c.Name, &c.VATNumber, &c.Info, &c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get company: %w", err)
	}
	return &c, nil
}

func GetAllCompanies(search string) ([]models.Company, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, name, vat_number, info, created_at, created_by, updated_at, updated_by FROM companies`
	var args []interface{}
	if search != "" {
		query += " WHERE name LIKE ? OR vat_number LIKE ?"
		term := "%" + search + "%"
		args = append(args, term, term)
	}
	query += " ORDER BY name ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query companies: %w", err)
	}
	defer rows.Close()

	companies := []models.Company{}
	for rows.Next() {
		var c models.Company
		if err := rows.Scan(&c.ID, &c.Name, &c.VATNumber, &c.Info, &c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy); err != nil {
			return nil, err
		}
		companies = append(companies, c)
	}
	return companies, nil
}

func DeleteCompany(id int) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM companies WHERE id = ?", id)
	return err
}

// --- Contact Repository ---

func SaveContact(contact *models.Contact, username string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if contact.ID == 0 {
		query := `
		INSERT INTO contacts (company_id, name, email, phone, type, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, contact.CompanyID, contact.Name, contact.Email, contact.Phone, contact.Type, now, username, now, username)
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
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, company_id, name, email, phone, type, created_at, created_by, updated_at, updated_by FROM contacts WHERE id = ?`
	var c models.Contact
	err := db.QueryRow(query, id).Scan(
		&c.ID, &c.CompanyID, &c.Name, &c.Email, &c.Phone, &c.Type, &c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get contact: %w", err)
	}
	return &c, nil
}

func GetContactsByCompany(companyID int) ([]models.Contact, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, company_id, name, email, phone, type, created_at, created_by, updated_at, updated_by FROM contacts WHERE company_id = ? ORDER BY name ASC`
	rows, err := db.Query(query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	contacts := []models.Contact{}
	for rows.Next() {
		var c models.Contact
		if err := rows.Scan(&c.ID, &c.CompanyID, &c.Name, &c.Email, &c.Phone, &c.Type, &c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func DeleteContact(id int) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM contacts WHERE id = ?", id)
	return err
}

// --- Site-Company Mapping Repository ---

func LinkSiteToCompany(siteID, companyID int, username string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_company_map (site_id, company_id, created_by)
	VALUES (?, ?, ?)
	ON CONFLICT(site_id, company_id) DO NOTHING
	`
	_, err := db.Exec(query, siteID, companyID, username)
	return err
}

func UnlinkSiteFromCompany(siteID, companyID int) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM site_company_map WHERE site_id = ? AND company_id = ?`
	_, err := db.Exec(query, siteID, companyID)
	return err
}

func GetCompanyBySite(siteID int) (*models.Company, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT c.id, c.name, c.vat_number, c.info, c.created_at, c.created_by, c.updated_at, c.updated_by
	FROM companies c
	JOIN site_company_map m ON c.id = m.company_id
	WHERE m.site_id = ?
	`
	var c models.Company
	err := db.QueryRow(query, siteID).Scan(
		&c.ID, &c.Name, &c.VATNumber, &c.Info, &c.CreatedAt, &c.CreatedBy, &c.UpdatedAt, &c.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get company for site: %w", err)
	}
	return &c, nil
}

func GetSitesByCompany(companyID int) ([]int, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT site_id FROM site_company_map WHERE company_id = ?`
	rows, err := db.Query(query, companyID)
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
	return siteIDs, nil
}

// --- Notes Repository ---

func SaveNote(note *models.Note, username string) error {
	db := GetDB()
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

func GetNotes(parentType models.NoteParentType, parentID int) ([]models.Note, error) {
	db := GetDB()
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
	return notes, nil
}

func DeleteNote(id int) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM notes WHERE id = ?", id)
	return err
}

func GetNote(id int) (*models.Note, error) {
	db := GetDB()
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
