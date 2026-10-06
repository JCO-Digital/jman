package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ErrRefreshTokenInvalid is returned for unknown or expired refresh tokens.
var ErrRefreshTokenInvalid = errors.New("invalid refresh token")

// RefreshToken is a stored login session token. Tokens sharing a FamilyID
// descend from the same login: each refresh marks the presented token
// rotated and issues a successor in the same family.
type RefreshToken struct {
	ID           int64
	FamilyID     string
	Username     string
	TokenVersion int
	ExpiresAt    time.Time
	RotatedAt    sql.NullTime
}

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insertRefreshToken(conn execer, familyID, username string, tokenVersion int, expiresAt time.Time) (string, error) {
	raw, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}
	_, err = conn.Exec(
		`INSERT INTO refresh_tokens (family_id, username, token_hash, token_version, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		familyID, username, hashRefreshToken(raw), tokenVersion, expiresAt.UTC(), time.Now().UTC(),
	)
	if err != nil {
		return "", fmt.Errorf("failed to store refresh token: %w", err)
	}
	return raw, nil
}

// CreateRefreshToken starts a new session family for username and returns
// the plaintext token. Only its hash is stored.
func CreateRefreshToken(username string, tokenVersion int, expiresAt time.Time) (string, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return "", fmt.Errorf("database not initialized")
	}
	familyID, err := randomToken(16)
	if err != nil {
		return "", fmt.Errorf("failed to generate session id: %w", err)
	}
	return insertRefreshToken(dbConn, familyID, username, tokenVersion, expiresAt)
}

// GetRefreshToken looks up a plaintext refresh token. It returns
// ErrRefreshTokenInvalid if the token is unknown or expired. A rotated
// token is still returned; the caller decides how to treat reuse.
func GetRefreshToken(raw string) (*RefreshToken, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if raw == "" {
		return nil, ErrRefreshTokenInvalid
	}

	var t RefreshToken
	err := dbConn.QueryRow(
		`SELECT id, family_id, username, token_version, expires_at, rotated_at
		 FROM refresh_tokens WHERE token_hash = ?`,
		hashRefreshToken(raw),
	).Scan(&t.ID, &t.FamilyID, &t.Username, &t.TokenVersion, &t.ExpiresAt, &t.RotatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrRefreshTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up refresh token: %w", err)
	}
	if !time.Now().Before(t.ExpiresAt) {
		return nil, ErrRefreshTokenInvalid
	}
	return &t, nil
}

// RotateRefreshToken marks old as rotated and issues its successor in the
// same family, expiring at expiresAt. If another request rotated old first,
// it returns ("", false, nil) and issues nothing.
func RotateRefreshToken(old *RefreshToken, expiresAt time.Time) (string, bool, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return "", false, fmt.Errorf("database not initialized")
	}

	tx, err := dbConn.Begin()
	if err != nil {
		return "", false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`UPDATE refresh_tokens SET rotated_at = ? WHERE id = ? AND rotated_at IS NULL`,
		time.Now().UTC(), old.ID,
	)
	if err != nil {
		return "", false, fmt.Errorf("failed to rotate refresh token: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return "", false, fmt.Errorf("failed to rotate refresh token: %w", err)
	} else if n == 0 {
		return "", false, nil
	}

	raw, err := insertRefreshToken(tx, old.FamilyID, old.Username, old.TokenVersion, expiresAt)
	if err != nil {
		return "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("failed to commit refresh token rotation: %w", err)
	}
	return raw, true, nil
}

// RevokeRefreshTokenFamily deletes every token of a session, logging it out.
func RevokeRefreshTokenFamily(familyID string) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`DELETE FROM refresh_tokens WHERE family_id = ?`, familyID); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}
	return nil
}

// DeleteExpiredRefreshTokens removes tokens that expired before now.
func DeleteExpiredRefreshTokens(now time.Time) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`DELETE FROM refresh_tokens WHERE expires_at < ?`, now); err != nil {
		return fmt.Errorf("failed to delete expired refresh tokens: %w", err)
	}
	return nil
}
