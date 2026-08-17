package api

import (
	"context"
	"net/http"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/verb"
)

type agentContextKey string

const agentClaimsKey agentContextKey = "agentClaims"

// AgentClaims identifies the server a validated agent token belongs to.
type AgentClaims struct {
	TokenID  int
	ServerID int
}

// GetAgentClaims retrieves the AgentClaims from the request context.
// Returns nil if the context does not carry claims (i.e. unauthenticated).
func GetAgentClaims(ctx context.Context) *AgentClaims {
	v, _ := ctx.Value(agentClaimsKey).(*AgentClaims)
	return v
}

func contextWithAgentClaims(ctx context.Context, claims *AgentClaims) context.Context {
	return context.WithValue(ctx, agentClaimsKey, claims)
}

// AgentAuthMiddleware validates the X-Agent-Token header against the
// agent_tokens table and injects AgentClaims into the request context.
// This is deliberately separate from AuthMiddleware (human JWT/TOTP login) —
// agent tokens are long-lived per-server machine credentials with no
// relationship to human user accounts or token versioning.
func AgentAuthMiddleware(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("X-Agent-Token")
		if raw == "" {
			WriteError(w, http.StatusUnauthorized, "Agent token required")
			return
		}

		claims, err := db.VerifyAgentToken(raw)
		if err != nil {
			verb.LogPrintf(verb.Debug, "Agent token validation failed: %v", err)
			WriteError(w, http.StatusUnauthorized, "Invalid or revoked agent token")
			return
		}

		if touchErr := db.TouchAgentTokenLastSeen(claims.TokenID); touchErr != nil {
			verb.LogPrintf(verb.Debug, "Failed to update agent token last_seen_at: %v", touchErr)
		}

		ctx := contextWithAgentClaims(r.Context(), &AgentClaims{TokenID: claims.TokenID, ServerID: claims.ServerID})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
