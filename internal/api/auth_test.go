package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func TestSignAndParseToken(t *testing.T) {
	secret := "this-is-a-long-enough-secret-for-jwt-signing"
	usersCfg := &config.UsersConfig{
		JWTSecret:                  secret,
		AccessTokenLifetimeMinutes: 60,
	}

	user := &config.UserEntry{
		Username:    "testuser",
		DisplayName: "Test User",
		Level:       config.LevelEdit,
	}

	token, expiresAt, err := signToken(usersCfg, user)
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	if token == "" {
		t.Error("Token should not be empty")
	}

	if expiresAt.Before(time.Now()) {
		t.Error("Token should expire in the future")
	}

	claims, err := parseToken(usersCfg, token)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if claims.Subject != user.Username {
		t.Errorf("Expected username %s, got %s", user.Username, claims.Subject)
	}

	if claims.DisplayName != user.DisplayName {
		t.Errorf("Expected display name %s, got %s", user.DisplayName, claims.DisplayName)
	}

	if claims.Level != user.Level {
		t.Errorf("Expected level %s, got %s", user.Level, claims.Level)
	}
}

func TestSignTokenHighPrivilegeFallback(t *testing.T) {
	secret := "this-is-a-long-enough-secret-for-jwt-signing"
	usersCfg := &config.UsersConfig{
		JWTSecret:                  secret,
		AccessTokenLifetimeMinutes: 60,
	}

	t.Run("Admin with TOTP", func(t *testing.T) {
		user := &config.UserEntry{
			Username:    "admin",
			DisplayName: "Admin User",
			Level:       config.LevelAdmin,
			TOTPSecret:  "MFRGGZDFMZTWQ2LK",
		}
		token, _, _ := signToken(usersCfg, user)
		claims, _ := parseToken(usersCfg, token)
		if claims.Level != config.LevelAdmin {
			t.Errorf("Expected level admin, got %s", claims.Level)
		}
	})

	t.Run("Admin without TOTP falls back to Edit", func(t *testing.T) {
		user := &config.UserEntry{
			Username:    "admin",
			DisplayName: "Admin User",
			Level:       config.LevelAdmin,
			TOTPSecret:  "",
		}
		token, _, _ := signToken(usersCfg, user)
		claims, _ := parseToken(usersCfg, token)
		if claims.Level != config.LevelEdit {
			t.Errorf("Expected level edit (fallback), got %s", claims.Level)
		}
	})

	t.Run("Execute with TOTP", func(t *testing.T) {
		user := &config.UserEntry{
			Username:    "exec",
			DisplayName: "Exec User",
			Level:       config.LevelExecute,
			TOTPSecret:  "MFRGGZDFMZTWQ2LK",
		}
		token, _, _ := signToken(usersCfg, user)
		claims, _ := parseToken(usersCfg, token)
		if claims.Level != config.LevelExecute {
			t.Errorf("Expected level execute, got %s", claims.Level)
		}
	})

	t.Run("Execute without TOTP falls back to Edit", func(t *testing.T) {
		user := &config.UserEntry{
			Username:    "exec",
			DisplayName: "Exec User",
			Level:       config.LevelExecute,
			TOTPSecret:  "",
		}
		token, _, _ := signToken(usersCfg, user)
		claims, _ := parseToken(usersCfg, token)
		if claims.Level != config.LevelEdit {
			t.Errorf("Expected level edit (fallback), got %s", claims.Level)
		}
	})
}

func TestLoginHandler(t *testing.T) {
	password := "password123"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	usersCfg := &config.UsersConfig{
		JWTSecret:                  "another-long-enough-secret-key-for-testing",
		AccessTokenLifetimeMinutes: 60,
		Users: []config.UserEntry{
			{
				Username:     "admin",
				PasswordHash: string(hash),
				DisplayName:  "Admin User",
				Level:        config.LevelAdmin,
			},
		},
	}

	setupSettingsTest(t)
	limiter := NewLoginRateLimiter(false)

	t.Run("Successful Login", func(t *testing.T) {
		loginReq := loginRequest{
			Username: "admin",
			Password: password,
		}
		body, _ := json.Marshal(loginReq)
		req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler := LoginHandler(usersCfg, limiter)
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}

		var resp loginResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Token == "" {
			t.Error("Token should not be empty")
		}
		if resp.User.Username != "admin" {
			t.Errorf("Expected user admin, got %s", resp.User.Username)
		}
		if resp.User.Level != config.LevelEdit {
			t.Errorf("Expected level edit, got %s", resp.User.Level)
		}

		cookie := findRefreshCookie(w.Result().Cookies())
		if cookie == nil || cookie.Value == "" {
			t.Fatal("Expected a refresh token cookie")
		}
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
			t.Errorf("Refresh cookie must be HttpOnly, Secure and SameSite=Strict: %+v", cookie)
		}
		if cookie.Path != refreshCookiePath {
			t.Errorf("Expected cookie path %s, got %s", refreshCookiePath, cookie.Path)
		}
	})

	t.Run("Invalid Password", func(t *testing.T) {
		loginReq := loginRequest{
			Username: "admin",
			Password: "wrongpassword",
		}
		body, _ := json.Marshal(loginReq)
		req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler := LoginHandler(usersCfg, limiter)
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})
}

func TestAuthMiddleware(t *testing.T) {
	usersCfg := &config.UsersConfig{
		JWTSecret:                  "yet-another-long-secret-key-for-testing",
		AccessTokenLifetimeMinutes: 60,
		Users: []config.UserEntry{
			{
				Username:    "admin",
				DisplayName: "Admin",
				Level:       config.LevelBasic,
			},
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			t.Error("Claims should not be nil in protected handler")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	middleware := AuthMiddleware(usersCfg, nextHandler)

	t.Run("Valid Token", func(t *testing.T) {
		user := &config.UserEntry{Username: "admin", DisplayName: "Admin", Level: config.LevelBasic}
		token, _, _ := signToken(usersCfg, user)
		req := httptest.NewRequest("GET", "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
	})

	t.Run("Revoked Token (version mismatch)", func(t *testing.T) {
		// Sign a token with current TokenVersion (0)
		user := &config.UserEntry{Username: "admin", DisplayName: "Admin", Level: config.LevelBasic}
		token, _, _ := signToken(usersCfg, user)

		// Simulate a credential change by incrementing the stored version
		usersCfg.Users[0].TokenVersion++

		req := httptest.NewRequest("GET", "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}

		// Reset for other tests
		usersCfg.Users[0].TokenVersion--
	})
}

func findRefreshCookie(cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == refreshCookieName {
			return c
		}
	}
	return nil
}

func TestRefreshHandler(t *testing.T) {
	setupSettingsTest(t)

	usersCfg := &config.UsersConfig{
		JWTSecret:                  "one-more-long-secret-key-for-testing-refresh",
		AccessTokenLifetimeMinutes: 15,
		RefreshTokenLifetimeDays:   30,
		Users: []config.UserEntry{
			{
				Username:    "admin",
				DisplayName: "Admin",
				Level:       config.LevelEdit,
			},
		},
	}
	handler := RefreshHandler(usersCfg)

	newSession := func(t *testing.T) string {
		t.Helper()
		raw, err := db.CreateRefreshToken("admin", usersCfg.Users[0].TokenVersion, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
		return raw
	}

	refresh := func(raw string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
		if raw != "" {
			req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: raw})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	t.Run("Rotates", func(t *testing.T) {
		raw := newSession(t)
		w := refresh(raw)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected status OK, got %d: %s", w.Code, w.Body.String())
		}

		var resp refreshResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if resp.Token == "" {
			t.Error("Token should not be empty")
		}
		if resp.User.Username != "admin" || resp.User.Level != config.LevelEdit {
			t.Errorf("Unexpected user in response: %+v", resp.User)
		}
		if until := time.Until(resp.ExpiresAt); until > 16*time.Minute {
			t.Errorf("Access token should be short-lived, expires in %s", until)
		}

		next := findRefreshCookie(w.Result().Cookies())
		if next == nil || next.Value == "" || next.Value == raw {
			t.Fatalf("Expected a new refresh cookie, got %+v", next)
		}
		if w := refresh(next.Value); w.Code != http.StatusOK {
			t.Errorf("Successor token should refresh, got %d", w.Code)
		}
	})

	t.Run("Concurrent Reuse Within Grace", func(t *testing.T) {
		raw := newSession(t)
		if w := refresh(raw); w.Code != http.StatusOK {
			t.Fatalf("First refresh failed: %d", w.Code)
		}
		w := refresh(raw)
		if w.Code != http.StatusOK {
			t.Fatalf("Reuse within grace should succeed, got %d", w.Code)
		}
		if c := findRefreshCookie(w.Result().Cookies()); c != nil {
			t.Errorf("Reuse within grace must not set a cookie, got %+v", c)
		}
	})

	t.Run("Reuse After Grace Revokes Session", func(t *testing.T) {
		raw := newSession(t)
		w := refresh(raw)
		next := findRefreshCookie(w.Result().Cookies())
		if next == nil {
			t.Fatal("Expected a new refresh cookie")
		}
		if _, err := db.GetAPIDB().Exec(`UPDATE refresh_tokens SET rotated_at = ? WHERE rotated_at IS NOT NULL`,
			time.Now().Add(-time.Minute)); err != nil {
			t.Fatalf("backdate rotation: %v", err)
		}

		if w := refresh(raw); w.Code != http.StatusUnauthorized {
			t.Fatalf("Reused token should be rejected, got %d", w.Code)
		}
		if w := refresh(next.Value); w.Code != http.StatusUnauthorized {
			t.Errorf("Whole session should be revoked after reuse, got %d", w.Code)
		}
	})

	t.Run("Token Version Changed", func(t *testing.T) {
		raw := newSession(t)
		usersCfg.Users[0].TokenVersion++
		defer func() { usersCfg.Users[0].TokenVersion-- }()
		if w := refresh(raw); w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Missing Or Unknown Cookie", func(t *testing.T) {
		if w := refresh(""); w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized without cookie, got %d", w.Code)
		}
		if w := refresh("not-a-real-token"); w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized for unknown token, got %d", w.Code)
		}
	})

	t.Run("Expired", func(t *testing.T) {
		raw, err := db.CreateRefreshToken("admin", usersCfg.Users[0].TokenVersion, time.Now().Add(-time.Second))
		if err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
		if w := refresh(raw); w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})

	t.Run("User Removed", func(t *testing.T) {
		raw, err := db.CreateRefreshToken("nonexistent", 0, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
		if w := refresh(raw); w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Logout", func(t *testing.T) {
		raw := newSession(t)
		req := httptest.NewRequest("POST", "/api/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: raw})
		w := httptest.NewRecorder()
		LogoutHandler(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("Expected status No Content, got %d", w.Code)
		}
		if c := findRefreshCookie(w.Result().Cookies()); c == nil || c.MaxAge >= 0 {
			t.Errorf("Logout should clear the cookie, got %+v", c)
		}
		if w := refresh(raw); w.Code != http.StatusUnauthorized {
			t.Errorf("Refresh after logout should fail, got %d", w.Code)
		}
	})
}

func TestRequireLevelMiddleware(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("Insufficient Level", func(t *testing.T) {
		claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
		ctx := contextWithClaims(context.Background(), claims)
		req := httptest.NewRequest("POST", "/api/test", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		RequireLevel(config.LevelEdit)(nextHandler).ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status Forbidden, got %d", w.Code)
		}
	})

	t.Run("Sufficient Level", func(t *testing.T) {
		claims := &AuthClaims{Username: "user", Level: config.LevelEdit}
		ctx := contextWithClaims(context.Background(), claims)
		req := httptest.NewRequest("POST", "/api/test", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		RequireLevel(config.LevelEdit)(nextHandler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
	})

	t.Run("Admin Level", func(t *testing.T) {
		claims := &AuthClaims{Username: "user", Level: config.LevelAdmin}
		ctx := contextWithClaims(context.Background(), claims)
		req := httptest.NewRequest("POST", "/api/test", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		RequireLevel(config.LevelEdit)(nextHandler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
	})

	t.Run("Higher Level", func(t *testing.T) {
		claims := &AuthClaims{Username: "user", Level: config.LevelExecute}
		ctx := contextWithClaims(context.Background(), claims)
		req := httptest.NewRequest("POST", "/api/test", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		RequireLevel(config.LevelEdit)(nextHandler).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
	})
}
