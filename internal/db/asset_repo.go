package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// --- Asset Repository (Templates) ---

func SaveAsset(asset *models.Asset, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if asset.ID == 0 {
		query := `
		INSERT INTO assets (type, identifier, name, description, default_price, default_freq, active, payment_method_id, purchase_price, quantity, next_payment, management_url, management_account, license_key, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, asset.Type, asset.Identifier, asset.Name, asset.Description, asset.DefaultPrice, asset.DefaultFreq, asset.Active,
			asset.PaymentMethodID, asset.PurchasePrice, asset.Quantity, asset.NextPayment, asset.ManagementURL, asset.ManagementAccount, asset.LicenseKey, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert asset: %w", err)
		}
		id, _ := result.LastInsertId()
		asset.ID = int(id)
		asset.CreatedAt = now
		asset.CreatedBy = username
		asset.UpdatedAt = now
		asset.UpdatedBy = username
	} else {
		query := `
		UPDATE assets SET type = ?, identifier = ?, name = ?, description = ?, default_price = ?, default_freq = ?, active = ?,
		       payment_method_id = ?, purchase_price = ?, quantity = ?, next_payment = ?, management_url = ?, management_account = ?, license_key = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, asset.Type, asset.Identifier, asset.Name, asset.Description, asset.DefaultPrice, asset.DefaultFreq, asset.Active,
			asset.PaymentMethodID, asset.PurchasePrice, asset.Quantity, asset.NextPayment, asset.ManagementURL, asset.ManagementAccount, asset.LicenseKey, now, username, asset.ID)
		if err != nil {
			return fmt.Errorf("failed to update asset: %w", err)
		}
		asset.UpdatedAt = now
		asset.UpdatedBy = username
	}
	return nil
}

func GetAsset(id int) (*models.Asset, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT a.id, a.type, a.identifier, a.name, a.description, a.default_price, a.default_freq, a.active,
	       a.payment_method_id, a.purchase_price, a.quantity, a.next_payment, a.management_url, a.management_account, a.license_key,
	       a.created_at, a.created_by, a.updated_at, a.updated_by, pm.name
	FROM assets a
	LEFT JOIN payment_methods pm ON a.payment_method_id = pm.id
	WHERE a.id = ?
	`
	var a models.Asset
	var pmName sql.NullString
	err := db.QueryRow(query, id).Scan(
		&a.ID, &a.Type, &a.Identifier, &a.Name, &a.Description, &a.DefaultPrice, &a.DefaultFreq, &a.Active,
		&a.PaymentMethodID, &a.PurchasePrice, &a.Quantity, &a.NextPayment, &a.ManagementURL, &a.ManagementAccount, &a.LicenseKey,
		&a.CreatedAt, &a.CreatedBy, &a.UpdatedAt, &a.UpdatedBy, &pmName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get asset: %w", err)
	}
	if pmName.Valid {
		a.PaymentMethodName = pmName.String
	}
	return &a, nil
}

func GetAllAssets(search string) ([]models.Asset, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT a.id, a.type, a.identifier, a.name, a.description, a.default_price, a.default_freq, a.active,
	       a.payment_method_id, a.purchase_price, a.quantity, a.next_payment, a.management_url, a.management_account, a.license_key,
	       a.created_at, a.created_by, a.updated_at, a.updated_by,
	       pm.name, COUNT(oa.id) AS usage_count
	FROM assets a
	LEFT JOIN organization_assets oa ON oa.asset_id = a.id
	LEFT JOIN payment_methods pm ON a.payment_method_id = pm.id
	WHERE 1=1
	`
	var args []interface{}
	if search != "" {
		query += " AND (a.name LIKE ? OR a.identifier LIKE ? OR a.type LIKE ?)"
		term := "%" + search + "%"
		args = append(args, term, term, term)
	}
	query += " GROUP BY a.id ORDER BY a.type ASC, a.name ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query assets: %w", err)
	}
	defer rows.Close()

	assets := []models.Asset{}
	for rows.Next() {
		var a models.Asset
		var pmName sql.NullString
		if err := rows.Scan(
			&a.ID, &a.Type, &a.Identifier, &a.Name, &a.Description, &a.DefaultPrice, &a.DefaultFreq, &a.Active,
			&a.PaymentMethodID, &a.PurchasePrice, &a.Quantity, &a.NextPayment, &a.ManagementURL, &a.ManagementAccount, &a.LicenseKey,
			&a.CreatedAt, &a.CreatedBy, &a.UpdatedAt, &a.UpdatedBy, &pmName, &a.UsageCount,
		); err != nil {
			return nil, err
		}
		if pmName.Valid {
			a.PaymentMethodName = pmName.String
		}
		assets = append(assets, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assets, nil
}

func DeleteAsset(id int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM assets WHERE id = ?", id)
	return err
}

// --- Organization Asset Repository (Links) ---

func SaveOrganizationAsset(oa *models.OrganizationAsset, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if oa.ID == 0 {
		query := `
		INSERT INTO organization_assets (organization_id, site_id, asset_id, identifier, price, billing_freq, next_billing, status, description, payment_method_id, license_key, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, oa.OrganizationID, oa.SiteID, oa.AssetID, oa.Identifier, oa.Price, oa.BillingFreq, oa.NextBilling, oa.Status, oa.Description, oa.PaymentMethodID, oa.LicenseKey, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert organization asset: %w", err)
		}
		id, _ := result.LastInsertId()
		oa.ID = int(id)
		oa.CreatedAt = now
		oa.CreatedBy = username
		oa.UpdatedAt = now
		oa.UpdatedBy = username
	} else {
		query := `
		UPDATE organization_assets SET site_id = ?, asset_id = ?, identifier = ?, price = ?, billing_freq = ?, next_billing = ?, status = ?, description = ?, payment_method_id = ?, license_key = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, oa.SiteID, oa.AssetID, oa.Identifier, oa.Price, oa.BillingFreq, oa.NextBilling, oa.Status, oa.Description, oa.PaymentMethodID, oa.LicenseKey, now, username, oa.ID)
		if err != nil {
			return fmt.Errorf("failed to update organization asset: %w", err)
		}
		oa.UpdatedAt = now
		oa.UpdatedBy = username
	}
	return nil
}

func GetOrganizationAsset(id int) (*models.OrganizationAsset, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT oa.id, oa.organization_id, oa.site_id, oa.asset_id, oa.identifier, oa.price, oa.billing_freq,
	       oa.next_billing, oa.status, oa.description, oa.payment_method_id, oa.license_key, oa.created_at, oa.created_by, oa.updated_at, oa.updated_by,
	       o.name, a.name, a.type, pm.name,
	       a.purchase_price, a.quantity, a.next_payment, a.management_url, a.management_account, a.license_key
	FROM organization_assets oa
	LEFT JOIN organizations o ON oa.organization_id = o.id
	LEFT JOIN assets a ON oa.asset_id = a.id
	LEFT JOIN payment_methods pm ON oa.payment_method_id = pm.id
	WHERE oa.id = ?
	`
	var oa models.OrganizationAsset
	var orgName, assetName, assetType, pmName sql.NullString
	var assetPurchasePrice, assetQuantity sql.NullInt64
	var assetNextPayment sql.NullTime
	var assetManagementURL, assetManagementAccount, assetLicenseKey sql.NullString
	err := db.QueryRow(query, id).Scan(
		&oa.ID, &oa.OrganizationID, &oa.SiteID, &oa.AssetID, &oa.Identifier, &oa.Price, &oa.BillingFreq,
		&oa.NextBilling, &oa.Status, &oa.Description, &oa.PaymentMethodID, &oa.LicenseKey, &oa.CreatedAt, &oa.CreatedBy, &oa.UpdatedAt, &oa.UpdatedBy,
		&orgName, &assetName, &assetType, &pmName,
		&assetPurchasePrice, &assetQuantity, &assetNextPayment, &assetManagementURL, &assetManagementAccount, &assetLicenseKey,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get organization asset: %w", err)
	}
	if orgName.Valid {
		oa.OrganizationName = orgName.String
	}
	if assetName.Valid {
		oa.AssetName = assetName.String
	}
	if assetType.Valid {
		oa.AssetType = assetType.String
	}
	if pmName.Valid {
		oa.PaymentMethodName = pmName.String
	}
	if assetPurchasePrice.Valid {
		oa.AssetPurchasePrice = int(assetPurchasePrice.Int64)
	}
	if assetQuantity.Valid {
		oa.AssetQuantity = int(assetQuantity.Int64)
	}
	if assetNextPayment.Valid {
		oa.AssetNextPayment = &assetNextPayment.Time
	}
	if assetManagementURL.Valid {
		oa.AssetManagementURL = assetManagementURL.String
	}
	if assetManagementAccount.Valid {
		oa.AssetManagementAccount = assetManagementAccount.String
	}
	if assetLicenseKey.Valid {
		oa.AssetLicenseKey = assetLicenseKey.String
	}
	return &oa, nil
}

func GetAllOrganizationAssets(search, status, before string) ([]models.OrganizationAsset, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT oa.id, oa.organization_id, oa.site_id, oa.asset_id, oa.identifier, oa.price, oa.billing_freq,
	       oa.next_billing, oa.status, oa.description, oa.payment_method_id, oa.license_key, oa.created_at, oa.created_by, oa.updated_at, oa.updated_by,
	       o.name as organization_name, a.name as asset_name, a.type as asset_type, pm.name as payment_method_name,
	       a.purchase_price, a.quantity, a.next_payment, a.management_url, a.management_account, a.license_key as asset_license_key
	FROM organization_assets oa
	LEFT JOIN organizations o ON oa.organization_id = o.id
	LEFT JOIN assets a ON oa.asset_id = a.id
	LEFT JOIN payment_methods pm ON oa.payment_method_id = pm.id
	WHERE 1=1
	`
	var args []interface{}

	if search != "" {
		query += " AND (oa.identifier LIKE ? OR o.name LIKE ? OR a.name LIKE ?)"
		term := "%" + search + "%"
		args = append(args, term, term, term)
	}

	if status != "" {
		query += " AND oa.status = ?"
		args = append(args, status)
	}

	if before != "" {
		query += " AND oa.next_billing <= ?"
		args = append(args, before)
	}

	query += " ORDER BY oa.next_billing ASC, oa.created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query organization assets: %w", err)
	}
	defer rows.Close()

	oas := []models.OrganizationAsset{}
	for rows.Next() {
		oa, err := scanOrganizationAssetRow(rows)
		if err != nil {
			return nil, err
		}
		oas = append(oas, oa)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return oas, nil
}

func GetOrganizationAssetsByOrganization(organizationID int) ([]models.OrganizationAsset, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT oa.id, oa.organization_id, oa.site_id, oa.asset_id, oa.identifier, oa.price, oa.billing_freq,
	       oa.next_billing, oa.status, oa.description, oa.payment_method_id, oa.license_key, oa.created_at, oa.created_by, oa.updated_at, oa.updated_by,
	       o.name as organization_name, a.name as asset_name, a.type as asset_type, pm.name as payment_method_name,
	       a.purchase_price, a.quantity, a.next_payment, a.management_url, a.management_account, a.license_key as asset_license_key
	FROM organization_assets oa
	LEFT JOIN organizations o ON oa.organization_id = o.id
	LEFT JOIN assets a ON oa.asset_id = a.id
	LEFT JOIN payment_methods pm ON oa.payment_method_id = pm.id
	WHERE oa.organization_id = ?
	ORDER BY oa.next_billing ASC, oa.created_at DESC
	`
	rows, err := db.Query(query, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	oas := []models.OrganizationAsset{}
	for rows.Next() {
		oa, err := scanOrganizationAssetRow(rows)
		if err != nil {
			return nil, err
		}
		oas = append(oas, oa)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return oas, nil
}

// scanOrganizationAssetRow scans a row produced by the shared "organization_assets LEFT JOIN
// organizations/assets/payment_methods" column layout used by GetAllOrganizationAssets and
// GetOrganizationAssetsByOrganization.
func scanOrganizationAssetRow(rows *sql.Rows) (models.OrganizationAsset, error) {
	var oa models.OrganizationAsset
	var orgName, assetName, assetType, pmName sql.NullString
	var assetPurchasePrice, assetQuantity sql.NullInt64
	var assetNextPayment sql.NullTime
	var assetManagementURL, assetManagementAccount, assetLicenseKey sql.NullString
	err := rows.Scan(
		&oa.ID, &oa.OrganizationID, &oa.SiteID, &oa.AssetID, &oa.Identifier, &oa.Price, &oa.BillingFreq,
		&oa.NextBilling, &oa.Status, &oa.Description, &oa.PaymentMethodID, &oa.LicenseKey, &oa.CreatedAt, &oa.CreatedBy, &oa.UpdatedAt, &oa.UpdatedBy,
		&orgName, &assetName, &assetType, &pmName,
		&assetPurchasePrice, &assetQuantity, &assetNextPayment, &assetManagementURL, &assetManagementAccount, &assetLicenseKey,
	)
	if err != nil {
		return oa, err
	}
	if orgName.Valid {
		oa.OrganizationName = orgName.String
	}
	if assetName.Valid {
		oa.AssetName = assetName.String
	}
	if assetType.Valid {
		oa.AssetType = assetType.String
	}
	if pmName.Valid {
		oa.PaymentMethodName = pmName.String
	}
	if assetPurchasePrice.Valid {
		oa.AssetPurchasePrice = int(assetPurchasePrice.Int64)
	}
	if assetQuantity.Valid {
		oa.AssetQuantity = int(assetQuantity.Int64)
	}
	if assetNextPayment.Valid {
		oa.AssetNextPayment = &assetNextPayment.Time
	}
	if assetManagementURL.Valid {
		oa.AssetManagementURL = assetManagementURL.String
	}
	if assetManagementAccount.Valid {
		oa.AssetManagementAccount = assetManagementAccount.String
	}
	if assetLicenseKey.Valid {
		oa.AssetLicenseKey = assetLicenseKey.String
	}
	return oa, nil
}

func DeleteOrganizationAsset(id int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM organization_assets WHERE id = ?", id)
	return err
}

// --- Asset Payment Repository ---

func SaveAssetPayment(payment *models.AssetPayment, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	if payment.ID == 0 {
		query := `
		INSERT INTO asset_payments (org_asset_id, amount, payment_date, info, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?)
		`
		now := time.Now()
		result, err := db.Exec(query, payment.OrgAssetID, payment.Amount, payment.PaymentDate, payment.Info, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert asset payment: %w", err)
		}
		id, _ := result.LastInsertId()
		payment.ID = int(id)
		payment.CreatedAt = now
		payment.CreatedBy = username
	} else {
		query := `
		UPDATE asset_payments SET amount = ?, payment_date = ?, info = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, payment.Amount, payment.PaymentDate, payment.Info, payment.ID)
		if err != nil {
			return fmt.Errorf("failed to update asset payment: %w", err)
		}
	}
	return nil
}

func GetAssetPaymentsByAsset(orgAssetID int) ([]models.AssetPayment, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, org_asset_id, amount, payment_date, info, created_at, created_by FROM asset_payments WHERE org_asset_id = ? ORDER BY payment_date DESC`
	rows, err := db.Query(query, orgAssetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := []models.AssetPayment{}
	for rows.Next() {
		var p models.AssetPayment
		if err := rows.Scan(&p.ID, &p.OrgAssetID, &p.Amount, &p.PaymentDate, &p.Info, &p.CreatedAt, &p.CreatedBy); err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return payments, nil
}

// GetAssetPaymentsInRange returns all asset_payments with payment_date in
// [start, end] (inclusive, "YYYY-MM-DD"), enriched with organization/asset
// context, ordered by organization then payment date, for use by the
// asset/billing report.
func GetAssetPaymentsInRange(start, end string) ([]models.AssetPaymentReportRow, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT o.id, o.name, oa.id, oa.identifier, a.name, a.type, oa.billing_freq, oa.status,
	       ap.id, ap.amount, ap.payment_date, ap.info
	FROM asset_payments ap
	JOIN organization_assets oa ON ap.org_asset_id = oa.id
	JOIN organizations o ON oa.organization_id = o.id
	LEFT JOIN assets a ON oa.asset_id = a.id
	WHERE ap.payment_date >= ? AND ap.payment_date <= ?
	ORDER BY o.name ASC, ap.payment_date ASC
	`
	// Compared as plain ISO8601 text against the stored column (matching
	// GetSiteTraffic's cutoff-expression convention) rather than wrapping
	// the column in date()/strftime() — the modernc.org/sqlite driver's
	// DATE/DATETIME column handling doesn't play well with SQL date
	// functions applied to the column itself. end is extended to the end
	// of its day so a payment_date with a non-midnight time-of-day on the
	// last day is still included.
	rows, err := db.Query(query, start, end+"T23:59:59Z")
	if err != nil {
		return nil, fmt.Errorf("failed to query asset payments for report: %w", err)
	}
	defer rows.Close()

	result := []models.AssetPaymentReportRow{}
	for rows.Next() {
		var row models.AssetPaymentReportRow
		var assetName, assetType sql.NullString
		if err := rows.Scan(
			&row.OrganizationID, &row.OrganizationName, &row.OrgAssetID, &row.Identifier,
			&assetName, &assetType, &row.BillingFreq, &row.Status,
			&row.PaymentID, &row.Amount, &row.PaymentDate, &row.Info,
		); err != nil {
			return nil, fmt.Errorf("failed to scan asset payment report row: %w", err)
		}
		if assetName.Valid {
			row.AssetName = assetName.String
		}
		if assetType.Valid {
			row.AssetType = assetType.String
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating asset payment report rows: %w", err)
	}
	return result, nil
}

func DeleteAssetPayment(id int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM asset_payments WHERE id = ?", id)
	return err
}
