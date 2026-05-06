package api

import (
	"encoding/json"
	"net/http"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// --- Admin Handlers (LevelExecute) ---

type createUserRequest struct {
	Username    string           `json:"username"`
	Password    string           `json:"password"`
	DisplayName string           `json:"displayName"`
	Level       config.UserLevel `json:"level"`
}

// CreateUserHandler allows an admin to add a new user.
func CreateUserHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.Username == "" || req.Password == "" || req.DisplayName == "" {
			WriteError(w, http.StatusBadRequest, "Username, password, and display name are required")
			return
		}

		if err := ValidatePasswordStrength(req.Password); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		if config.FindUser(usersCfg, req.Username) != nil {
			WriteError(w, http.StatusConflict, "User already exists")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		level := req.Level
		if level == "" {
			level = config.LevelBasic
		}

		usersCfg.Users = append(usersCfg.Users, config.UserEntry{
			Username:     req.Username,
			PasswordHash: string(hash),
			DisplayName:  req.DisplayName,
			Level:        level,
		})

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, *usersCfg); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]string{"status": "user created"})
	}
}

type updateUserRequest struct {
	DisplayName string           `json:"displayName"`
	Level       config.UserLevel `json:"level"`
}

// UpdateUserHandler allows an admin to modify user details.
func UpdateUserHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")
		if username == "" {
			WriteError(w, http.StatusBadRequest, "Username is required")
			return
		}

		var req updateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		user := config.FindUser(usersCfg, username)
		if user == nil {
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		if req.DisplayName != "" {
			user.DisplayName = req.DisplayName
		}
		if req.Level != "" {
			user.Level = req.Level
		}

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, *usersCfg); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "user updated"})
	}
}

// DeleteUserHandler allows an admin to remove a user.
func DeleteUserHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.PathValue("username")
		claims := GetAuthClaims(r.Context())

		if claims != nil && claims.Username == username {
			WriteError(w, http.StatusForbidden, "Cannot delete your own account")
			return
		}

		userIndex := -1
		execCount := 0
		for i, u := range usersCfg.Users {
			if u.Username == username {
				userIndex = i
			}
			if u.Level == config.LevelExecute {
				execCount++
			}
		}

		if userIndex == -1 {
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		// Prevent locking out the system
		if usersCfg.Users[userIndex].Level == config.LevelExecute && execCount <= 1 {
			WriteError(w, http.StatusForbidden, "Cannot delete the last administrator")
			return
		}

		// Remove user
		usersCfg.Users = append(usersCfg.Users[:userIndex], usersCfg.Users[userIndex+1:]...)

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, *usersCfg); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "user deleted"})
	}
}

// AdminListUsersHandler provides an enhanced user list for admin management.
func AdminListUsersHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type userListItem struct {
			Username    string           `json:"username"`
			DisplayName string           `json:"displayName"`
			Level       config.UserLevel `json:"level"`
			Has2FA      bool             `json:"has2FA"`
		}

		var resp []userListItem
		for _, u := range usersCfg.Users {
			resp = append(resp, userListItem{
				Username:    u.Username,
				DisplayName: u.DisplayName,
				Level:       u.Level,
				Has2FA:      u.TOTPSecret != "",
			})
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// --- User Self-Service Handlers (Any Level) ---

type updateProfileRequest struct {
	DisplayName string `json:"displayName"`
}

// UpdateProfileHandler allows the logged-in user to update their own profile details.
func UpdateProfileHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		var req updateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if req.DisplayName != "" {
			user.DisplayName = req.DisplayName
		}

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, *usersCfg); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "profile updated"})
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// ChangePasswordHandler allows the logged-in user to change their own password.
func ChangePasswordHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			WriteError(w, http.StatusUnauthorized, "Invalid current password")
			return
		}

		if err := ValidatePasswordStrength(req.NewPassword); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		user.PasswordHash = string(hash)

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, *usersCfg); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "password updated"})
	}
}

// Setup2FAHandler generates a temporary TOTP secret for the user to set up their app.
func Setup2FAHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "jman-api",
		AccountName: claims.Username,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"secret": key.Secret(),
		"uri":    key.URL(),
	})
}

type activate2FARequest struct {
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

// Activate2FAHandler verifies a setup code and persists the TOTP secret.
func Activate2FAHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		var req activate2FARequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.Secret == "" || req.Code == "" {
			WriteError(w, http.StatusBadRequest, "Secret and code are required")
			return
		}

		if !totp.Validate(req.Code, req.Secret) {
			WriteError(w, http.StatusBadRequest, "Invalid verification code")
			return
		}

		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		user.TOTPSecret = req.Secret

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, *usersCfg); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "2FA enabled"})
	}
}

type deactivate2FARequest struct {
	Code string `json:"code"`
}

// Deactivate2FAHandler removes the TOTP secret for the logged-in user.
// It requires a valid TOTP code to confirm the deactivation.
func Deactivate2FAHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		// If 2FA is enabled, require a valid code to disable it.
		if user.TOTPSecret != "" {
			var req deactivate2FARequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				WriteError(w, http.StatusBadRequest, "Invalid request body")
				return
			}
			if !totp.Validate(req.Code, user.TOTPSecret) {
				WriteError(w, http.StatusBadRequest, "Invalid verification code")
				return
			}
		}

		user.TOTPSecret = ""

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, *usersCfg); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "2FA disabled"})
	}
}
