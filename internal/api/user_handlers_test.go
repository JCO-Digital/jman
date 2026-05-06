package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

func setupTestUsersConfig(t *testing.T) (*config.UsersConfig, string) {
	tempDir, err := os.MkdirTemp("", "jman-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	config.RunData.ConfigDir = tempDir

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	cfg := &config.UsersConfig{
		JWTSecret: "test-secret-at-least-32-chars-long-!!!",
		Users: []config.UserEntry{
			{
				Username:     "admin",
				PasswordHash: string(hash),
				DisplayName:  "Admin User",
				Level:        config.LevelExecute,
			},
			{
				Username:     "user",
				PasswordHash: string(hash),
				DisplayName:  "Normal User",
				Level:        config.LevelBasic,
			},
		},
	}

	return cfg, tempDir
}

func TestAdminListUsersHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := AdminListUsersHandler(cfg)
	req := httptest.NewRequest("GET", "/api/users", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var users []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}
}

func TestCreateUserHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := CreateUserHandler(cfg)

	t.Run("Success", func(t *testing.T) {
		reqBody := createUserRequest{
			Username:    "newuser",
			Password:    "newpass",
			DisplayName: "New User",
			Level:       config.LevelEdit,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", w.Code)
		}

		if config.FindUser(cfg, "newuser") == nil {
			t.Error("User was not created in config")
		}
	})

	t.Run("Duplicate User", func(t *testing.T) {
		reqBody := createUserRequest{
			Username:    "admin",
			Password:    "pass",
			DisplayName: "Admin",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("Expected status 409, got %d", w.Code)
		}
	})
}

func TestDeleteUserHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := DeleteUserHandler(cfg)

	t.Run("Delete Others", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/users/user", nil)
		req.SetPathValue("username", "user")

		// Set admin claims in context
		claims := &AuthClaims{Username: "admin", Level: config.LevelExecute}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		if config.FindUser(cfg, "user") != nil {
			t.Error("User was not deleted")
		}
	})

	t.Run("Delete Self Forbidden", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/users/admin", nil)
		req.SetPathValue("username", "admin")

		claims := &AuthClaims{Username: "admin", Level: config.LevelExecute}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", w.Code)
		}
	})

	t.Run("Delete Last Admin Forbidden", func(t *testing.T) {
		// Only "admin" is left as LevelExecute
		req := httptest.NewRequest("DELETE", "/api/users/admin", nil)
		req.SetPathValue("username", "admin")

		// Use a different admin for the claim to bypass "delete self" check if there were multiple
		claims := &AuthClaims{Username: "other-admin", Level: config.LevelExecute}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", w.Code)
		}
	})
}

func TestUpdateProfileHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := UpdateProfileHandler(cfg)

	t.Run("Successful Profile Update", func(t *testing.T) {
		reqBody := updateProfileRequest{
			DisplayName: "Updated Name",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PATCH", "/api/user/profile", bytes.NewBuffer(body))

		claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		user := config.FindUser(cfg, "user")
		if user.DisplayName != "Updated Name" {
			t.Errorf("Expected display name Updated Name, got %s", user.DisplayName)
		}
	})
}

func TestChangePasswordHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := ChangePasswordHandler(cfg)

	t.Run("Successful Change", func(t *testing.T) {
		reqBody := changePasswordRequest{
			CurrentPassword: "password123",
			NewPassword:     "newsecurepass",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/user/password", bytes.NewBuffer(body))

		claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		user := config.FindUser(cfg, "user")
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("newsecurepass"))
		if err != nil {
			t.Error("Password was not updated correctly")
		}
	})

	t.Run("Wrong Current Password", func(t *testing.T) {
		reqBody := changePasswordRequest{
			CurrentPassword: "wrongpassword",
			NewPassword:     "newsecurepass",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/user/password", bytes.NewBuffer(body))

		claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}

func TestTOTPFlow(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
	ctx := contextWithClaims(context.Background(), claims)

	// 1. Setup
	setupReq := httptest.NewRequest("POST", "/api/user/2fa/setup", nil)
	setupReq = setupReq.WithContext(ctx)
	setupW := httptest.NewRecorder()
	Setup2FAHandler(setupW, setupReq)

	if setupW.Code != http.StatusOK {
		t.Errorf("Setup failed with %d", setupW.Code)
	}

	var setupResp map[string]string
	json.Unmarshal(setupW.Body.Bytes(), &setupResp)
	secret := setupResp["secret"]

	if secret == "" {
		t.Fatal("No secret returned from setup")
	}

	// 2. Activate (with a valid code)
	code, _ := totp.GenerateCode(secret, time.Now())
	activateBody, _ := json.Marshal(activate2FARequest{
		Secret: secret,
		Code:   code,
	})
	activateReq := httptest.NewRequest("POST", "/api/user/2fa/activate", bytes.NewBuffer(activateBody))
	activateReq = activateReq.WithContext(ctx)
	activateW := httptest.NewRecorder()
	Activate2FAHandler(cfg)(activateW, activateReq)

	if activateW.Code != http.StatusOK {
		t.Errorf("Activate failed with %d: %s", activateW.Code, activateW.Body.String())
	}

	user := config.FindUser(cfg, "user")
	if user.TOTPSecret != secret {
		t.Error("TOTP secret was not saved to user")
	}

	// 3. Deactivate
	deactivateCode, _ := totp.GenerateCode(secret, time.Now())
	deactivateBody, _ := json.Marshal(deactivate2FARequest{
		Code: deactivateCode,
	})
	deactivateReq := httptest.NewRequest("POST", "/api/user/2fa/deactivate", bytes.NewBuffer(deactivateBody))
	deactivateReq = deactivateReq.WithContext(ctx)
	deactivateW := httptest.NewRecorder()
	Deactivate2FAHandler(cfg)(deactivateW, deactivateReq)

	if deactivateW.Code != http.StatusOK {
		t.Errorf("Deactivate failed with %d", deactivateW.Code)
	}

	if user.TOTPSecret != "" {
		t.Error("TOTP secret was not cleared")
	}
}
