package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// CreateIncident creates a new incident if there is not already an active (open or acknowledged)
// incident for the specified domain. If an active incident already exists, it updates its error message/code
// and returns the existing incident.
func CreateIncident(domain, errorMessage string, errorCode int, downSince time.Time) (*models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	existing, err := GetActiveIncidentByDomain(domain)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		// Update latest error info
		now := time.Now()
		_, err := db.Exec(`
			UPDATE incidents
			SET error_message = ?, error_code = ?, updated_at = ?
			WHERE id = ?
		`, errorMessage, errorCode, now, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to update active incident: %w", err)
		}
		existing.ErrorMessage = errorMessage
		existing.ErrorCode = errorCode
		existing.UpdatedAt = now
		return existing, nil
	}

	now := time.Now()
	if downSince.IsZero() {
		downSince = now
	}

	query := `
		INSERT INTO incidents (domain, status, error_message, error_code, down_since, pd_triggered, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
	`
	res, err := db.Exec(query, domain, models.IncidentStatusOpen, errorMessage, errorCode, downSince, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get incident insert ID: %w", err)
	}

	return &models.Incident{
		ID:           id,
		Domain:       domain,
		Status:       models.IncidentStatusOpen,
		ErrorMessage: errorMessage,
		ErrorCode:    errorCode,
		DownSince:    downSince,
		PDTriggered:  false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// GetActiveIncidentByDomain returns the currently active (open or acknowledged) incident for domain, or nil if none.
func GetActiveIncidentByDomain(domain string) (*models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT id, domain, status, error_message, error_code, down_since,
		       acknowledged_by, acknowledged_at, resolved_by, resolved_at,
		       pd_triggered, created_at, updated_at
		FROM incidents
		WHERE domain = LOWER(?) AND status IN ('open', 'acknowledged')
		ORDER BY id DESC LIMIT 1
	`
	row := db.QueryRow(query, domain)
	inc, err := scanIncident(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query active incident for %s: %w", domain, err)
	}
	return inc, nil
}

// GetIncidentByID fetches a single incident by its ID.
func GetIncidentByID(id int64) (*models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT id, domain, status, error_message, error_code, down_since,
		       acknowledged_by, acknowledged_at, resolved_by, resolved_at,
		       pd_triggered, created_at, updated_at
		FROM incidents
		WHERE id = ?
	`
	row := db.QueryRow(query, id)
	inc, err := scanIncident(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get incident %d: %w", id, err)
	}
	return inc, nil
}

// GetActiveIncidents returns all active (open or acknowledged) incidents, ordered by down_since DESC.
func GetActiveIncidents() ([]models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT id, domain, status, error_message, error_code, down_since,
		       acknowledged_by, acknowledged_at, resolved_by, resolved_at,
		       pd_triggered, created_at, updated_at
		FROM incidents
		WHERE status IN ('open', 'acknowledged')
		ORDER BY down_since DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active incidents: %w", err)
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		inc, err := scanIncidentRows(rows)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, *inc)
	}
	return incidents, nil
}

// GetIncidents retrieves paginated incidents according to filter ("active", "resolved", "closed", or "all").
func GetIncidents(filter string, limit, offset int) ([]models.Incident, int, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	whereClause := ""
	var args []interface{}
	switch filter {
	case "active", "":
		whereClause = "WHERE status IN ('open', 'acknowledged')"
	case "resolved":
		whereClause = "WHERE status = 'resolved'"
	case "closed":
		whereClause = "WHERE status = 'closed'"
	case "history":
		whereClause = "WHERE status IN ('resolved', 'closed')"
	case "all":
		whereClause = ""
	default:
		whereClause = "WHERE status = ?"
		args = append(args, filter)
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM incidents %s", whereClause)
	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count incidents: %w", err)
	}

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, domain, status, error_message, error_code, down_since,
		       acknowledged_by, acknowledged_at, resolved_by, resolved_at,
		       pd_triggered, created_at, updated_at
		FROM incidents
		%s
		ORDER BY id DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	queryArgs := append(args, limit, offset)
	rows, err := db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query incidents: %w", err)
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		inc, err := scanIncidentRows(rows)
		if err != nil {
			return nil, 0, err
		}
		incidents = append(incidents, *inc)
	}

	return incidents, total, nil
}

// GetActiveIncidentCount returns the count of currently open or acknowledged incidents.
func GetActiveIncidentCount() (int, error) {
	db := GetAPIDB()
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM incidents WHERE status IN ('open', 'acknowledged')").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get active incident count: %w", err)
	}
	return count, nil
}

// AcknowledgeIncident transitions an open incident to acknowledged.
func AcknowledgeIncident(id int64, username string) (*models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now()
	query := `
		UPDATE incidents
		SET status = ?, acknowledged_by = ?, acknowledged_at = ?, updated_at = ?
		WHERE id = ? AND status = 'open'
	`
	res, err := db.Exec(query, models.IncidentStatusAcknowledged, username, now, now, id)
	if err != nil {
		return nil, fmt.Errorf("failed to acknowledge incident %d: %w", id, err)
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		// Either already acknowledged or not found/not open
		return GetIncidentByID(id)
	}

	return GetIncidentByID(id)
}

// ResolveActiveIncidentByDomain resolves any active incident for domain.
func ResolveActiveIncidentByDomain(domain string, username string) (*models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	active, err := GetActiveIncidentByDomain(domain)
	if err != nil || active == nil {
		return nil, err
	}

	return ResolveIncident(active.ID, username)
}

// ResolveIncident transitions an active incident to resolved.
func ResolveIncident(id int64, username string) (*models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now()
	var res sql.Result
	var err error
	if username != "" {
		query := `
			UPDATE incidents
			SET status = ?, resolved_by = ?, resolved_at = ?, updated_at = ?
			WHERE id = ? AND status IN ('open', 'acknowledged')
		`
		res, err = db.Exec(query, models.IncidentStatusResolved, username, now, now, id)
	} else {
		query := `
			UPDATE incidents
			SET status = ?, resolved_at = ?, updated_at = ?
			WHERE id = ? AND status IN ('open', 'acknowledged')
		`
		res, err = db.Exec(query, models.IncidentStatusResolved, now, now, id)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to resolve incident %d: %w", id, err)
	}

	_ = res
	return GetIncidentByID(id)
}

// CloseIncident manually closes an incident (marking status = 'closed').
func CloseIncident(id int64, username string) (*models.Incident, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now()
	query := `
		UPDATE incidents
		SET status = ?, resolved_by = ?, resolved_at = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := db.Exec(query, models.IncidentStatusClosed, username, now, now, id)
	if err != nil {
		return nil, fmt.Errorf("failed to close incident %d: %w", id, err)
	}

	return GetIncidentByID(id)
}

// SetIncidentPDTriggered sets the pd_triggered flag on an incident.
func SetIncidentPDTriggered(id int64, triggered bool) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	_, err := db.Exec(`
		UPDATE incidents
		SET pd_triggered = ?, updated_at = ?
		WHERE id = ?
	`, triggered, now, id)
	if err != nil {
		return fmt.Errorf("failed to update pd_triggered on incident %d: %w", id, err)
	}
	return nil
}

func scanIncident(row *sql.Row) (*models.Incident, error) {
	var inc models.Incident
	var ackBy, resBy sql.NullString
	var ackAt, resAt sql.NullTime
	var downSince, createdAt, updatedAt sql.NullTime

	err := row.Scan(
		&inc.ID,
		&inc.Domain,
		&inc.Status,
		&inc.ErrorMessage,
		&inc.ErrorCode,
		&downSince,
		&ackBy,
		&ackAt,
		&resBy,
		&resAt,
		&inc.PDTriggered,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if downSince.Valid {
		inc.DownSince = downSince.Time
	}
	if ackBy.Valid {
		inc.AcknowledgedBy = &ackBy.String
	}
	if ackAt.Valid {
		inc.AcknowledgedAt = &ackAt.Time
	}
	if resBy.Valid {
		inc.ResolvedBy = &resBy.String
	}
	if resAt.Valid {
		inc.ResolvedAt = &resAt.Time
	}
	if createdAt.Valid {
		inc.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		inc.UpdatedAt = updatedAt.Time
	}

	return &inc, nil
}

func scanIncidentRows(rows *sql.Rows) (*models.Incident, error) {
	var inc models.Incident
	var ackBy, resBy sql.NullString
	var ackAt, resAt sql.NullTime
	var downSince, createdAt, updatedAt sql.NullTime

	err := rows.Scan(
		&inc.ID,
		&inc.Domain,
		&inc.Status,
		&inc.ErrorMessage,
		&inc.ErrorCode,
		&downSince,
		&ackBy,
		&ackAt,
		&resBy,
		&resAt,
		&inc.PDTriggered,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if downSince.Valid {
		inc.DownSince = downSince.Time
	}
	if ackBy.Valid {
		inc.AcknowledgedBy = &ackBy.String
	}
	if ackAt.Valid {
		inc.AcknowledgedAt = &ackAt.Time
	}
	if resBy.Valid {
		inc.ResolvedBy = &resBy.String
	}
	if resAt.Valid {
		inc.ResolvedAt = &resAt.Time
	}
	if createdAt.Valid {
		inc.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		inc.UpdatedAt = updatedAt.Time
	}

	return &inc, nil
}
