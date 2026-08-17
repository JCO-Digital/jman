package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// agentTokenBcryptCost mirrors the cost used for human user passwords
// (internal/api.BcryptCost) without introducing an api->db dependency.
const agentTokenBcryptCost = 12

// AgentClaims identifies the server an agent token belongs to, returned by
// VerifyAgentToken on success.
type AgentClaims struct {
	TokenID  int
	ServerID int
}

// CreateAgentToken generates a new random secret for the given server,
// stores its bcrypt hash, and returns the one-time plaintext token in the
// form "<id>.<secret>". The plaintext value cannot be recovered later — only
// TokenPrefix (its first 8 characters) is retained for display purposes.
func CreateAgentToken(serverID int, serverName, description, createdBy string) (models.AgentToken, string, error) {
	dbConn := GetDB()
	if dbConn == nil {
		return models.AgentToken{}, "", fmt.Errorf("database not initialized")
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return models.AgentToken{}, "", fmt.Errorf("failed to generate token secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), agentTokenBcryptCost)
	if err != nil {
		return models.AgentToken{}, "", fmt.Errorf("failed to hash token secret: %w", err)
	}

	prefix := secret[:8]

	var descPtr *string
	if description != "" {
		descPtr = &description
	}

	result, err := dbConn.Exec(
		`INSERT INTO agent_tokens (server_id, server_name, token_hash, token_prefix, description, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		serverID, serverName, string(hash), prefix, descPtr, createdBy,
	)
	if err != nil {
		return models.AgentToken{}, "", fmt.Errorf("failed to create agent token: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return models.AgentToken{}, "", fmt.Errorf("failed to determine new token id: %w", err)
	}

	token := models.AgentToken{
		ID:          int(id),
		ServerID:    serverID,
		ServerName:  serverName,
		TokenPrefix: prefix,
		Description: descPtr,
		Revoked:     false,
		CreatedBy:   createdBy,
	}

	plaintext := fmt.Sprintf("%d.%s", id, secret)
	return token, plaintext, nil
}

// VerifyAgentToken parses a raw "<id>.<secret>" token, looks up the
// corresponding row by id (avoiding a full-table bcrypt-compare scan), and
// verifies the secret against its stored hash. It fails for unknown,
// malformed, revoked, or mismatched tokens.
func VerifyAgentToken(raw string) (*AgentClaims, error) {
	dbConn := GetDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	idPart, secret, found := strings.Cut(raw, ".")
	if !found || idPart == "" || secret == "" {
		return nil, fmt.Errorf("malformed token")
	}
	id, err := strconv.Atoi(idPart)
	if err != nil {
		return nil, fmt.Errorf("malformed token")
	}

	var serverID int
	var tokenHash string
	var revoked bool
	row := dbConn.QueryRow(`SELECT server_id, token_hash, revoked FROM agent_tokens WHERE id = ?`, id)
	if err := row.Scan(&serverID, &tokenHash, &revoked); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid token")
		}
		return nil, fmt.Errorf("failed to look up token: %w", err)
	}
	if revoked {
		return nil, fmt.Errorf("token revoked")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(tokenHash), []byte(secret)); err != nil {
		return nil, fmt.Errorf("invalid token")
	}

	return &AgentClaims{TokenID: id, ServerID: serverID}, nil
}

// TouchAgentTokenLastSeen updates the last_seen_at timestamp for a token.
// Failures are non-fatal to callers — this is best-effort bookkeeping.
func TouchAgentTokenLastSeen(tokenID int) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`UPDATE agent_tokens SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`, tokenID)
	return err
}

// ListAgentTokens returns every agent token (including revoked ones), most
// recently created first.
func ListAgentTokens() ([]models.AgentToken, error) {
	dbConn := GetDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := dbConn.Query(
		`SELECT id, server_id, server_name, token_prefix, description, revoked, last_seen_at, agent_version, created_at, created_by
		 FROM agent_tokens ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent tokens: %w", err)
	}
	defer rows.Close()

	tokens := []models.AgentToken{}
	for rows.Next() {
		var t models.AgentToken
		var serverName sql.NullString
		var lastSeen sql.NullString
		var agentVersion sql.NullString
		var createdBy sql.NullString
		if err := rows.Scan(&t.ID, &t.ServerID, &serverName, &t.TokenPrefix, &t.Description, &t.Revoked, &lastSeen, &agentVersion, &t.CreatedAt, &createdBy); err != nil {
			return nil, fmt.Errorf("failed to scan agent token: %w", err)
		}
		t.ServerName = serverName.String
		t.CreatedBy = createdBy.String
		if lastSeen.Valid {
			t.LastSeenAt = &lastSeen.String
		}
		if agentVersion.Valid {
			t.AgentVersion = &agentVersion.String
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating agent tokens: %w", err)
	}

	return tokens, nil
}

// SetAgentTokenVersion records the jman-agent version reported in a
// successfully parsed report. This is deliberately separate from
// TouchAgentTokenLastSeen (called by AgentAuthMiddleware for every
// authenticated request, including manifest GETs which carry no version) —
// only a successful report updates it, so it doubles as a signal that the
// report path (not just authentication) is actually working.
func SetAgentTokenVersion(tokenID int, version string) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`UPDATE agent_tokens SET agent_version = ? WHERE id = ?`, version, tokenID)
	return err
}

// RevokeAgentToken marks a token as revoked, immediately blocking its use.
func RevokeAgentToken(id int) error {
	dbConn := GetDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	result, err := dbConn.Exec(`UPDATE agent_tokens SET revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to revoke agent token %d: %w", id, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm revocation of token %d: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("agent token %d not found", id)
	}
	return nil
}
