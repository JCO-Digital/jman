package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// --- Payment Method Repository ---

func SavePaymentMethod(pm *models.PaymentMethod, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if pm.ID == 0 {
		query := `
		INSERT INTO payment_methods (name, type, expiry_date, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, pm.Name, pm.Type, pm.ExpiryDate, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert payment method: %w", err)
		}
		id, _ := result.LastInsertId()
		pm.ID = int(id)
		pm.CreatedAt = now
		pm.CreatedBy = username
		pm.UpdatedAt = now
		pm.UpdatedBy = username
	} else {
		query := `
		UPDATE payment_methods SET name = ?, type = ?, expiry_date = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, pm.Name, pm.Type, pm.ExpiryDate, now, username, pm.ID)
		if err != nil {
			return fmt.Errorf("failed to update payment method: %w", err)
		}
		pm.UpdatedAt = now
		pm.UpdatedBy = username
	}
	return nil
}

func GetPaymentMethod(id int) (*models.PaymentMethod, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, name, type, expiry_date, created_at, created_by, updated_at, updated_by FROM payment_methods WHERE id = ?`
	var pm models.PaymentMethod
	err := db.QueryRow(query, id).Scan(
		&pm.ID, &pm.Name, &pm.Type, &pm.ExpiryDate, &pm.CreatedAt, &pm.CreatedBy, &pm.UpdatedAt, &pm.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get payment method: %w", err)
	}
	return &pm, nil
}

func GetAllPaymentMethods(search, pmType string) ([]models.PaymentMethod, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, name, type, expiry_date, created_at, created_by, updated_at, updated_by FROM payment_methods WHERE 1=1`
	var args []interface{}
	if search != "" {
		query += " AND name LIKE ?"
		args = append(args, "%"+search+"%")
	}
	if pmType != "" {
		query += " AND type = ?"
		args = append(args, pmType)
	}
	query += " ORDER BY type ASC, name ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query payment methods: %w", err)
	}
	defer rows.Close()

	methods := []models.PaymentMethod{}
	for rows.Next() {
		var pm models.PaymentMethod
		if err := rows.Scan(&pm.ID, &pm.Name, &pm.Type, &pm.ExpiryDate, &pm.CreatedAt, &pm.CreatedBy, &pm.UpdatedAt, &pm.UpdatedBy); err != nil {
			return nil, err
		}
		methods = append(methods, pm)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return methods, nil
}

func DeletePaymentMethod(id int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM payment_methods WHERE id = ?", id)
	return err
}
