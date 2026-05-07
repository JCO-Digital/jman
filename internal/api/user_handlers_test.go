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

	oldConfigDir := config.RunData.ConfigDir
	config.RunData.ConfigDir = tempDir
	t.Cleanup(func() {
		config.RunData.ConfigDir = oldConfigDir
	})

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	cfg := &config.UsersConfig{
		JWTSecret: "test-secret-at-least-32-chars-long-!!!",
		Users: []config.UserEntry{
			{
				Username:     "admin",
				PasswordHash: string(hash),
				DisplayName:  "Admin User",
				Level:        config.LevelAdmin,
			},
			{
				Username:     "admin2",
				PasswordHash: string(hash),
				DisplayName:  "Admin User 2",
				Level:        config.LevelAdmin,
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

	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}
}

func TestCreateUserHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := CreateUserHandler(cfg)

	t.Run("Success", func(t *testing.T) {
		reqBody := createUserRequest{
			Username:    "newuser",
			Password:    "newpass-strong-123",
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

	t.Run("Weak Password", func(t *testing.T) {
		reqBody := createUserRequest{
			Username:    "weakuser",
			Password:    "abc",
			DisplayName: "Weak User",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Invalid Username", func(t *testing.T) {
		reqBody := createUserRequest{
			Username:    "user!",
			Password:    "strong-password-12345",
			DisplayName: "Invalid User",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Invalid Level", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"username":    "invalidlevel",
			"password":    "strong-password-12345",
			"displayName": "Invalid Level",
			"level":       "superuser",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/users", bytes.NewBuffer(body))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Duplicate User", func(t *testing.T) {
		reqBody := createUserRequest{
			Username:    "admin",
			Password:    "password-is-strong-enough-now",
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

func TestUpdateUserHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := UpdateUserHandler(cfg)

	t.Run("Success", func(t *testing.T) {
		reqBody := updateUserRequest{
			DisplayName: "New Admin Name",
			Level:       config.LevelEdit,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PATCH", "/api/users/admin", bytes.NewBuffer(body))
		req.SetPathValue("username", "admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		user := config.FindUser(cfg, "admin")
		if user.DisplayName != "New Admin Name" {
			t.Errorf("Expected DisplayName New Admin Name, got %s", user.DisplayName)
		}
		if user.Level != config.LevelEdit {
			t.Errorf("Expected level edit, got %s", user.Level)
		}
	})

	t.Run("Admin Password Update", func(t *testing.T) {
		reqBody := updateUserRequest{
			Password: "new-admin-password-123456",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PATCH", "/api/users/user", bytes.NewBuffer(body))
		req.SetPathValue("username", "user")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		user := config.FindUser(cfg, "user")
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("new-admin-password-123456"))
		if err != nil {
			t.Error("Password was not updated correctly by admin")
		}
	})

	t.Run("User Not Found", func(t *testing.T) {
		reqBody := updateUserRequest{DisplayName: "Nobody"}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PATCH", "/api/users/nonexistent", bytes.NewBuffer(body))
		req.SetPathValue("username", "nonexistent")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
	})

	t.Run("Invalid Level", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"level": "god-mode",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("PATCH", "/api/users/admin", bytes.NewBuffer(body))
		req.SetPathValue("username", "admin")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
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
		claims := &AuthClaims{Username: "admin", Level: config.LevelAdmin}
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

		claims := &AuthClaims{Username: "admin", Level: config.LevelAdmin}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", w.Code)
		}
	})

	t.Run("Delete Last Admin Forbidden", func(t *testing.T) {
		// Remove admin2 first so admin is the last admin
		cfg.Lock()
		for i, u := range cfg.Users {
			if u.Username == "admin2" {
				cfg.Users = append(cfg.Users[:i], cfg.Users[i+1:]...)
				break
			}
		}
		cfg.Unlock()

		// Only "admin" is left as LevelAdmin
		req := httptest.NewRequest("DELETE", "/api/users/admin", nil)
		req.SetPathValue("username", "admin")

		// Use a different admin for the claim to bypass "delete self" check if there were multiple
		claims := &AuthClaims{Username: "other-admin", Level: config.LevelAdmin}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", w.Code)
		}
	})
}

func TestGetProfileHandler(t *testing.T) {
	cfg, tempDir := setupTestUsersConfig(t)
	defer os.RemoveAll(tempDir)

	handler := GetProfileHandler(cfg)

	t.Run("Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/user/profile", nil)

		claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp userProfileResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Username != "user" {
			t.Errorf("Expected username user, got %s", resp.Username)
		}
		if resp.Level != config.LevelBasic {
			t.Errorf("Expected level basic, got %s", resp.Level)
		}
		if resp.Has2FA != false {
			t.Error("Expected has2FA to be false")
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

	limiter := NewLoginRateLimiter(false)
	defer limiter.Stop()
	handler := ChangePasswordHandler(cfg, limiter)

	t.Run("Successful Change", func(t *testing.T) {
		reqBody := changePasswordRequest{
			CurrentPassword: "password123",
			NewPassword:     "newsecurepass-very-strong",
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
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("newsecurepass-very-strong"))
		if err != nil {
			t.Error("Password was not updated correctly")
		}
	})

	t.Run("Weak New Password", func(t *testing.T) {
		reqBody := changePasswordRequest{
			CurrentPassword: "newsecurepass-very-strong",
			NewPassword:     "short",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/user/password", bytes.NewBuffer(body))

		claims := &AuthClaims{Username: "user", Level: config.LevelBasic}
		ctx := contextWithClaims(context.Background(), claims)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Wrong Current Password", func(t *testing.T) {
		reqBody := changePasswordRequest{
			CurrentPassword: "wrongpassword",
			NewPassword:     "newsecurepass-very-strong",
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
	Setup2FAHandler(cfg)(setupW, setupReq)

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
		Code: code,
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
