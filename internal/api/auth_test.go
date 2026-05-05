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
	"golang.org/x/crypto/bcrypt"
)

func TestSignAndParseToken(t *testing.T) {
	secret := "this-is-a-long-enough-secret-for-jwt-signing"
	usersCfg := &config.UsersConfig{
		JWTSecret:          secret,
		TokenLifetimeHours: 1,
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

func TestSignTokenExecuteFallback(t *testing.T) {
	secret := "this-is-a-long-enough-secret-for-jwt-signing"
	usersCfg := &config.UsersConfig{
		JWTSecret:          secret,
		TokenLifetimeHours: 1,
	}

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
		JWTSecret:          "another-long-enough-secret-key-for-testing",
		TokenLifetimeHours: 1,
		Users: []config.UserEntry{
			{
				Username:     "admin",
				PasswordHash: string(hash),
				DisplayName:  "Admin User",
				Level:        config.LevelEdit,
			},
		},
	}

	limiter := NewLoginRateLimiter()

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
		JWTSecret:          "yet-another-long-secret-key-for-testing",
		TokenLifetimeHours: 1,
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
}

func TestRefreshHandler(t *testing.T) {
	usersCfg := &config.UsersConfig{
		JWTSecret:          "one-more-long-secret-key-for-testing-refresh",
		TokenLifetimeHours: 1,
		Users: []config.UserEntry{
			{
				Username:    "admin",
				DisplayName: "Admin",
				Level:       config.LevelEdit,
			},
		},
	}

	handler := RefreshHandler(usersCfg)

	t.Run("Authenticated", func(t *testing.T) {
		claims := &AuthClaims{Username: "admin", DisplayName: "Admin", Level: config.LevelEdit}
		ctx := contextWithClaims(context.Background(), claims)
		req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}

		var resp refreshResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Token == "" {
			t.Error("Token should not be empty")
		}
	})

	t.Run("User Removed", func(t *testing.T) {
		claims := &AuthClaims{Username: "nonexistent", DisplayName: "Deleted", Level: config.LevelEdit}
		ctx := contextWithClaims(context.Background(), claims)
		req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
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
