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
	"github.com/JCO-Digital/jman/internal/db"
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
	DisplayName  string           `json:"name,omitempty"`
	Level        config.UserLevel `json:"level,omitempty"`
	TokenVersion int              `json:"tver"`
}

// effectiveLevel returns the user's authorization level, applying the TOTP
// enforcement policy: Admin and Execute users without TOTP configured are
// downgraded to Edit level.
func effectiveLevel(user *config.UserEntry) config.UserLevel {
	level := user.Level
	if (level == config.LevelAdmin || level == config.LevelExecute) && user.TOTPSecret == "" {
		return config.LevelEdit
	}
	return level
}

// signToken creates a new signed, short-lived access JWT for the given user.
func signToken(usersCfg *config.UsersConfig, user *config.UserEntry) (string, time.Time, error) {
	usersCfg.LockRead()
	defer usersCfg.UnlockRead()
	now := time.Now()
	expiresAt := now.Add(usersCfg.AccessTokenLifetime())

	level := effectiveLevel(user)

	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.Username,
			Issuer:    "jman-api",
			Audience:  jwt.ClaimStrings{"jman-api"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		DisplayName:  user.DisplayName,
		Level:        level,
		TokenVersion: user.TokenVersion,
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
	usersCfg.LockRead()
	defer usersCfg.UnlockRead()
	token, err := jwt.ParseWithClaims(raw, &jwtClaims{}, func(t *jwt.Token) (any, error) {
		// Ensure the signing method is what we expect.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(usersCfg.JWTSecret), nil
	}, jwt.WithIssuer("jman-api"), jwt.WithAudience("jman-api"))
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
	Token     string        `json:"token"`
	ExpiresAt time.Time     `json:"expiresAt"`
	User      loginRespUser `json:"user"`
}

// --- Refresh token cookie ---

const (
	// refreshCookieName holds the long-lived refresh token. It is httpOnly,
	// so page scripts never see it, and scoped to the auth endpoints.
	refreshCookieName = "jman_refresh"
	refreshCookiePath = "/api/auth"

	// refreshReuseGrace is how long a rotated refresh token is still
	// accepted. Two tabs refreshing at once both send the same cookie; the
	// loser gets an access token without a new cookie instead of tripping
	// reuse detection. After the grace period, presenting a rotated token
	// means it was copied, and the whole session is revoked.
	refreshReuseGrace = 30 * time.Second
)

func setRefreshCookie(w http.ResponseWriter, raw string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    raw,
		Path:     refreshCookiePath,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func refreshCookieValue(r *http.Request) string {
	c, err := r.Cookie(refreshCookieName)
	if err != nil {
		return ""
	}
	return c.Value
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

		req.Username = NormalizeUsername(req.Username)
		if req.Username == "" || req.Password == "" {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Rate limiting check (per client IP).
		clientIP := limiter.ClientIP(r)
		if !limiter.Allow(clientIP) {
			WriteError(w, http.StatusTooManyRequests, "Too many login attempts, try again later")
			return
		}

		usersCfg.LockRead()
		user := config.FindUser(usersCfg, req.Username)

		// Always run bcrypt comparison to prevent timing-based user enumeration.
		hashToCompare := dummyHash
		if user != nil {
			hashToCompare = []byte(user.PasswordHash)
		}
		usersCfg.UnlockRead()
		if err := bcrypt.CompareHashAndPassword(hashToCompare, []byte(req.Password)); err != nil || user == nil {
			limiter.RecordFailure(clientIP)
			WriteError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		// TOTP validation (only if the user has a TOTP secret configured).
		if user.TOTPSecret != "" {
			if req.TOTP == "" || !totp.Validate(req.TOTP, user.TOTPSecret) {
				limiter.RecordFailure(clientIP)
				WriteError(w, http.StatusUnauthorized, "Invalid credentials")
				return
			}
		}

		// Authentication succeeded — issue an access token and start a session.
		token, expiresAt, err := signToken(usersCfg, user)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to sign JWT: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		now := time.Now()
		if err := db.DeleteExpiredRefreshTokens(now); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
		usersCfg.LockRead()
		tokenVersion := user.TokenVersion
		refreshExpiresAt := now.Add(usersCfg.RefreshTokenLifetime())
		usersCfg.UnlockRead()
		refreshToken, err := db.CreateRefreshToken(user.Username, tokenVersion, refreshExpiresAt)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to create refresh token: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		setRefreshCookie(w, refreshToken, refreshExpiresAt)

		limiter.Reset(clientIP)

		WriteJSON(w, http.StatusOK, loginResponse{
			Token:     token,
			ExpiresAt: expiresAt,
			User: loginRespUser{
				Username:    user.Username,
				DisplayName: user.DisplayName,
				Level:       effectiveLevel(user),
			},
		})
	}
}

// RefreshHandler returns an http.HandlerFunc that exchanges the refresh
// token cookie for a new access token, rotating the refresh token. It is a
// public route: the cookie is the credential, so it keeps working after the
// access token has expired.
func RefreshHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rt, err := db.GetRefreshToken(refreshCookieValue(r))
		if err != nil {
			if err != db.ErrRefreshTokenInvalid {
				verb.LogPrintf(verb.Normal, "%v", err)
			}
			clearRefreshCookie(w)
			WriteError(w, http.StatusUnauthorized, "Session expired")
			return
		}

		revoke := func(reason string) {
			if err := db.RevokeRefreshTokenFamily(rt.FamilyID); err != nil {
				verb.LogPrintf(verb.Normal, "%v", err)
			}
			clearRefreshCookie(w)
			WriteError(w, http.StatusUnauthorized, reason)
		}

		if rt.RotatedAt.Valid && time.Since(rt.RotatedAt.Time) > refreshReuseGrace {
			verb.LogPrintf(verb.Normal, "Rotated refresh token reused for user %s; revoking session", rt.Username)
			revoke("Session revoked")
			return
		}

		usersCfg.LockRead()
		user := config.FindUser(usersCfg, rt.Username)
		var tokenVersion int
		if user != nil {
			tokenVersion = user.TokenVersion
		}
		refreshExpiresAt := time.Now().Add(usersCfg.RefreshTokenLifetime())
		usersCfg.UnlockRead()

		if user == nil {
			revoke("User no longer exists")
			return
		}
		if rt.TokenVersion != tokenVersion {
			revoke("Session revoked")
			return
		}

		// A token inside its grace period was already rotated by a
		// concurrent request, which set the new cookie; don't rotate again.
		if !rt.RotatedAt.Valid {
			raw, rotated, err := db.RotateRefreshToken(rt, refreshExpiresAt)
			if err != nil {
				verb.LogPrintf(verb.Normal, "Failed to rotate refresh token: %v", err)
				WriteError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			if rotated {
				setRefreshCookie(w, raw, refreshExpiresAt)
			}
		}

		token, expiresAt, err := signToken(usersCfg, user)
		if err != nil {
			verb.LogPrintf(verb.Normal, "Failed to sign refresh JWT: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		usersCfg.LockRead()
		respUser := loginRespUser{
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Level:       effectiveLevel(user),
		}
		usersCfg.UnlockRead()

		WriteJSON(w, http.StatusOK, refreshResponse{
			Token:     token,
			ExpiresAt: expiresAt,
			User:      respUser,
		})
	}
}

// LogoutHandler ends the session named by the refresh token cookie and
// clears the cookie. Outstanding access tokens stay valid until they expire.
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if rt, err := db.GetRefreshToken(refreshCookieValue(r)); err == nil {
		if err := db.RevokeRefreshTokenFamily(rt.FamilyID); err != nil {
			verb.LogPrintf(verb.Normal, "%v", err)
		}
	}
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
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

		// Validate the token version against the current user record.
		// This allows immediate revocation of all tokens when credentials change.
		usersCfg.LockRead()
		user := config.FindUser(usersCfg, claims.Subject)
		if user == nil {
			usersCfg.UnlockRead()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}
		if claims.TokenVersion != user.TokenVersion {
			usersCfg.UnlockRead()
			WriteError(w, http.StatusUnauthorized, "Token revoked")
			return
		}
		usersCfg.UnlockRead()

		authClaims := &AuthClaims{
			Username:    claims.Subject,
			DisplayName: claims.DisplayName,
			Level:       claims.Level,
		}

		ctx := contextWithClaims(r.Context(), authClaims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
