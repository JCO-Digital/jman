package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// CLITokenPrefix starts every jman CLI token, so AuthMiddleware can tell
// one apart from a JWT.
const CLITokenPrefix = "jman_cli_"

// CLITokenIdleLifetime is how long a CLI token stays valid without being
// used. Each use extends it; the token itself never changes.
const CLITokenIdleLifetime = 90 * 24 * time.Hour

// cliTokenTouchInterval limits how often a token's last_used_at is
// written, so ordinary requests don't each cost a database write.
const cliTokenTouchInterval = time.Hour

// ErrCLITokenInvalid is returned for unknown or expired CLI tokens.
var ErrCLITokenInvalid = errors.New("invalid CLI token")

// CreateCLIToken issues a token for username on deviceName and returns it
// with the plaintext token. Only the token's hash is stored.
func CreateCLIToken(username, deviceName string, tokenVersion int) (models.CLIToken, string, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return models.CLIToken{}, "", fmt.Errorf("database not initialized")
	}
	secret, err := randomToken(32)
	if err != nil {
		return models.CLIToken{}, "", fmt.Errorf("failed to generate CLI token: %w", err)
	}
	raw := CLITokenPrefix + secret
	now := time.Now().UTC()

	t := models.CLIToken{
		Username:     username,
		DeviceName:   deviceName,
		TokenPrefix:  raw[:len(CLITokenPrefix)+6],
		TokenVersion: tokenVersion,
		CreatedAt:    now,
		LastUsedAt:   now,
		ExpiresAt:    now.Add(CLITokenIdleLifetime),
	}
	err = dbConn.QueryRow(
		`INSERT INTO cli_tokens (username, token_hash, token_prefix, device_name, token_version, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		username, hashRefreshToken(raw), t.TokenPrefix, deviceName, tokenVersion, now, now,
	).Scan(&t.ID)
	if err != nil {
		return models.CLIToken{}, "", fmt.Errorf("failed to store CLI token: %w", err)
	}
	return t, raw, nil
}

// GetCLIToken looks up a plaintext CLI token. It returns ErrCLITokenInvalid
// if the token is unknown or has gone unused for CLITokenIdleLifetime.
func GetCLIToken(raw string) (*models.CLIToken, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if !strings.HasPrefix(raw, CLITokenPrefix) {
		return nil, ErrCLITokenInvalid
	}

	row := dbConn.QueryRow(
		`SELECT id, username, device_name, token_prefix, token_version, created_at, last_used_at
		 FROM cli_tokens WHERE token_hash = ?`,
		hashRefreshToken(raw),
	)
	t, err := scanCLIToken(row)
	if err == sql.ErrNoRows {
		return nil, ErrCLITokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up CLI token: %w", err)
	}
	if !time.Now().Before(t.ExpiresAt) {
		return nil, ErrCLITokenInvalid
	}
	return t, nil
}

// TouchCLIToken records a use of t, extending its lifetime. To save
// writes, it does nothing if t was last recorded as used within
// cliTokenTouchInterval.
func TouchCLIToken(t *models.CLIToken) error {
	now := time.Now().UTC()
	if now.Sub(t.LastUsedAt) < cliTokenTouchInterval {
		return nil
	}
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`UPDATE cli_tokens SET last_used_at = ? WHERE id = ?`, now, t.ID); err != nil {
		return fmt.Errorf("failed to update CLI token %d: %w", t.ID, err)
	}
	return nil
}

// ListCLITokens returns the unexpired CLI tokens of username, or of every
// user when username is "", most recently used first.
func ListCLITokens(username string) ([]models.CLIToken, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	query := `SELECT id, username, device_name, token_prefix, token_version, created_at, last_used_at
		 FROM cli_tokens WHERE last_used_at > ?`
	args := []any{time.Now().UTC().Add(-CLITokenIdleLifetime)}
	if username != "" {
		query += ` AND username = ?`
		args = append(args, username)
	}
	query += ` ORDER BY last_used_at DESC, id DESC`

	rows, err := dbConn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list CLI tokens: %w", err)
	}
	defer rows.Close()

	tokens := []models.CLIToken{}
	for rows.Next() {
		t, err := scanCLIToken(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan CLI token: %w", err)
		}
		tokens = append(tokens, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating CLI tokens: %w", err)
	}
	return tokens, nil
}

// DeleteCLIToken revokes token id. If username isn't "", only a token of
// that user is deleted. It reports whether a token was deleted.
func DeleteCLIToken(id int64, username string) (bool, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return false, fmt.Errorf("database not initialized")
	}
	query := `DELETE FROM cli_tokens WHERE id = ?`
	args := []any{id}
	if username != "" {
		query += ` AND username = ?`
		args = append(args, username)
	}
	res, err := dbConn.Exec(query, args...)
	if err != nil {
		return false, fmt.Errorf("failed to delete CLI token %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to delete CLI token %d: %w", id, err)
	}
	return n > 0, nil
}

// DeleteExpiredCLITokens removes tokens unused for CLITokenIdleLifetime.
func DeleteExpiredCLITokens() error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	cutoff := time.Now().UTC().Add(-CLITokenIdleLifetime)
	if _, err := dbConn.Exec(`DELETE FROM cli_tokens WHERE last_used_at <= ?`, cutoff); err != nil {
		return fmt.Errorf("failed to delete expired CLI tokens: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCLIToken(row rowScanner) (*models.CLIToken, error) {
	var t models.CLIToken
	if err := row.Scan(&t.ID, &t.Username, &t.DeviceName, &t.TokenPrefix, &t.TokenVersion, &t.CreatedAt, &t.LastUsedAt); err != nil {
		return nil, err
	}
	t.ExpiresAt = t.LastUsedAt.Add(CLITokenIdleLifetime)
	return &t, nil
}
