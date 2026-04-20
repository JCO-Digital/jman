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

	username := "testuser"
	displayName := "Test User"

	token, expiresAt, err := signToken(usersCfg, username, displayName)
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

	if claims.Subject != username {
		t.Errorf("Expected username %s, got %s", username, claims.Subject)
	}

	if claims.DisplayName != displayName {
		t.Errorf("Expected display name %s, got %s", displayName, claims.DisplayName)
	}
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

	t.Run("Non-existent User", func(t *testing.T) {
		loginReq := loginRequest{
			Username: "nonexistent",
			Password: "password",
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
		token, _, _ := signToken(usersCfg, "admin", "Admin")
		req := httptest.NewRequest("GET", "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}
	})

	t.Run("Missing Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/protected", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Invalid Token Format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/protected", nil)
		req.Header.Set("Authorization", "InvalidFormat token")
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Malformed Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer not.a.token")
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})
}

func TestRefreshHandler(t *testing.T) {
	usersCfg := &config.UsersConfig{
		JWTSecret:          "one-more-long-secret-key-for-testing-refresh",
		TokenLifetimeHours: 1,
	}

	handler := RefreshHandler(usersCfg)

	t.Run("Unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Authenticated", func(t *testing.T) {
		claims := &AuthClaims{Username: "admin", DisplayName: "Admin"}
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
}
