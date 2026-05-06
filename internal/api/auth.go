package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// --- Context helpers ---

type contextKey string

const authClaimsKey contextKey = "authClaims"

// AuthClaims holds the authenticated user's identity extracted from a valid JWT.
type AuthClaims struct {
	Username    string
	DisplayName string
	Level       config.UserLevel
}

// GetAuthClaims retrieves the AuthClaims from the request context.
// Returns nil if the context does not carry claims (i.e. unauthenticated).
func GetAuthClaims(ctx context.Context) *AuthClaims {
	v, _ := ctx.Value(authClaimsKey).(*AuthClaims)
	return v
}

func contextWithClaims(ctx context.Context, claims *AuthClaims) context.Context {
	return context.WithValue(ctx, authClaimsKey, claims)
}

// --- JWT helpers ---

// jwtClaims is the full set of claims embedded in every token we issue.
type jwtClaims struct {
	jwt.RegisteredClaims
	DisplayName string           `json:"name,omitempty"`
	Level       config.UserLevel `json:"level,omitempty"`
}

// signToken creates a new signed JWT for the given user.
func signToken(usersCfg *config.UsersConfig, user *config.UserEntry) (string, time.Time, error) {
	usersCfg.RLock()
	defer usersCfg.RUnlock()
	lifetime := time.Duration(usersCfg.TokenLifetimeHours) * time.Hour
	if lifetime <= 0 {
		lifetime = 24 * time.Hour
	}
	now := time.Now()
	expiresAt := now.Add(lifetime)

	// Fallback logic: Execute level requires TOTP.
	effectiveLevel := user.Level
	if effectiveLevel == config.LevelExecute && user.TOTPSecret == "" {
		effectiveLevel = config.LevelEdit
	}

	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.Username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		DisplayName: user.DisplayName,
		Level:       effectiveLevel,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(usersCfg.JWTSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// parseToken validates a raw JWT string and returns the parsed claims.
func parseToken(usersCfg *config.UsersConfig, raw string) (*jwtClaims, error) {
	usersCfg.RLock()
	defer usersCfg.RUnlock()
	token, err := jwt.ParseWithClaims(raw, &jwtClaims{}, func(t *jwt.Token) (any, error) {
		// Ensure the signing method is what we expect.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(usersCfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// --- Request / Response types ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTP     string `json:"totp"`
}

type loginResponse struct {
	Token     string        `json:"token"`
	ExpiresAt time.Time     `json:"expiresAt"`
	User      loginRespUser `json:"user"`
}

type loginRespUser struct {
	Username    string           `json:"username"`
	DisplayName string           `json:"displayName"`
	Level       config.UserLevel `json:"level"`
}

type refreshResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// --- Handlers ---

// dummyHash is a pre-computed bcrypt hash used when the requested user does
// not exist. Comparing against it keeps the response time constant and
// prevents user-enumeration timing attacks.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-constant-time-comparison"), bcrypt.DefaultCost)

// LoginHandler returns an http.HandlerFunc that authenticates users via
// username/password (and optional TOTP) and issues a JWT on success.
func LoginHandler(usersCfg *config.UsersConfig, limiter *LoginRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.Username == "" || req.Password == "" {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Rate limiting check.
		if !limiter.Allow(req.Username) {
			WriteError(w, http.StatusTooManyRequests, "Too many login attempts, try again later")
			return
		}

		usersCfg.RLock()
		user := config.FindUser(usersCfg, req.Username)

		// Always run bcrypt comparison to prevent timing-based user enumeration.
		hashToCompare := dummyHash
		if user != nil {
			hashToCompare = []byte(user.PasswordHash)
		}
		usersCfg.RUnlock()
		if err := bcrypt.CompareHashAndPassword(hashToCompare, []byte(req.Password)); err != nil || user == nil {
			limiter.RecordFailure(req.Username)
			WriteError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		// TOTP validation (only if the user has a TOTP secret configured).
		if user.TOTPSecret != "" {
			if req.TOTP == "" {
				// Don't record as a failure — the client may retry with the code.
				WriteError(w, http.StatusUnauthorized, "TOTP code required")
				return
			}
			valid := totp.Validate(req.TOTP, user.TOTPSecret)
			if !valid {
				limiter.RecordFailure(req.Username)
				WriteError(w, http.StatusUnauthorized, "Invalid TOTP code")
				return
			}
		}

		// Authentication succeeded — issue a token.
		token, expiresAt, err := signToken(usersCfg, user)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to sign JWT: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		limiter.Reset(req.Username)

		// Re-calculate effective level for response (must match what's in the token).
		effectiveLevel := user.Level
		if effectiveLevel == config.LevelExecute && user.TOTPSecret == "" {
			effectiveLevel = config.LevelEdit
		}

		WriteJSON(w, http.StatusOK, loginResponse{
			Token:     token,
			ExpiresAt: expiresAt,
			User: loginRespUser{
				Username:    user.Username,
				DisplayName: user.DisplayName,
				Level:       effectiveLevel,
			},
		})
	}
}

// RefreshHandler returns an http.HandlerFunc that issues a fresh JWT for the
// currently authenticated user. It must be placed behind AuthMiddleware.
func RefreshHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		usersCfg.RLock()
		user := config.FindUser(usersCfg, claims.Username)
		usersCfg.RUnlock()

		if user == nil {
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		token, expiresAt, err := signToken(usersCfg, user)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to sign refresh JWT: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		WriteJSON(w, http.StatusOK, refreshResponse{
			Token:     token,
			ExpiresAt: expiresAt,
		})
	}
}

// --- Middleware ---

// AuthMiddleware returns middleware that validates the JWT Bearer token in the
// Authorization header and injects AuthClaims into the request context.
func AuthMiddleware(usersCfg *config.UsersConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		// Constant-time prefix check to avoid leaking header format.
		const bearerPrefix = "Bearer "
		if len(authHeader) < len(bearerPrefix) ||
			subtle.ConstantTimeCompare([]byte(authHeader[:len(bearerPrefix)]), []byte(bearerPrefix)) != 1 {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		rawToken := strings.TrimSpace(authHeader[len(bearerPrefix):])
		if rawToken == "" {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		claims, err := parseToken(usersCfg, rawToken)
		if err != nil {
			// Distinguish expired tokens for a friendlier client experience.
			if strings.Contains(err.Error(), "token is expired") {
				WriteError(w, http.StatusUnauthorized, "Token expired")
				return
			}
			verb.LogPrintf(verb.Debug, "JWT validation failed: %v", err)
			WriteError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		authClaims := &AuthClaims{
			Username:    claims.Subject,
			DisplayName: claims.DisplayName,
			Level:       claims.Level,
		}

		ctx := contextWithClaims(r.Context(), authClaims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
