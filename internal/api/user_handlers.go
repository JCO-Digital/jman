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

		req.Username = NormalizeUsername(req.Username)
		if req.Username == "" || req.Password == "" || req.DisplayName == "" {
			WriteError(w, http.StatusBadRequest, "Username, password, and display name are required")
			return
		}

		if err := ValidateDisplayName(req.DisplayName); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := ValidateUsername(req.Username); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := ValidatePasswordStrength(req.Password); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		usersCfg.LockRead()
		exists := config.FindUser(usersCfg, req.Username) != nil
		usersCfg.UnlockRead()
		if exists {
			WriteError(w, http.StatusConflict, "User already exists")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), BcryptCost)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		level := req.Level
		if level == "" {
			level = config.LevelBasic
		} else {
			if err := ValidateUserLevel(level); err != nil {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
		}

		usersCfg.LockWrite()
		// Re-check under write lock to prevent TOCTOU race condition.
		if config.FindUser(usersCfg, req.Username) != nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusConflict, "User already exists")
			return
		}
		usersCfg.Users = append(usersCfg.Users, config.UserEntry{
			Username:     req.Username,
			PasswordHash: string(hash),
			DisplayName:  req.DisplayName,
			Level:        level,
		})
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]string{"status": "user created"})
	}
}

type updateUserRequest struct {
	DisplayName string           `json:"displayName"`
	Level       config.UserLevel `json:"level"`
	Password    string           `json:"password"`
}

// UpdateUserHandler allows an admin to modify user details.
func UpdateUserHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := NormalizeUsername(r.PathValue("username"))
		if username == "" {
			WriteError(w, http.StatusBadRequest, "Username is required")
			return
		}

		var req updateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		usersCfg.LockWrite()
		user := config.FindUser(usersCfg, username)
		if user == nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		if req.DisplayName != "" {
			if err := ValidateDisplayName(req.DisplayName); err != nil {
				usersCfg.UnlockWrite()
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			user.DisplayName = req.DisplayName
		}
		if req.Level != "" {
			if err := ValidateUserLevel(req.Level); err != nil {
				usersCfg.UnlockWrite()
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			// Prevent demoting the last admin/execute user.
			if (user.Level == config.LevelAdmin || user.Level == config.LevelExecute) &&
				req.Level != config.LevelAdmin && req.Level != config.LevelExecute {
				adminCount := 0
				for _, u := range usersCfg.Users {
					if u.Level == config.LevelAdmin || u.Level == config.LevelExecute {
						adminCount++
					}
				}
				if adminCount <= 1 {
					usersCfg.UnlockWrite()
					WriteError(w, http.StatusForbidden, "Cannot demote the last administrator")
					return
				}
			}
			user.Level = req.Level
			user.TokenVersion++
		}
		if req.Password != "" {
			if err := ValidatePasswordStrength(req.Password); err != nil {
				usersCfg.UnlockWrite()
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), BcryptCost)
			if err != nil {
				usersCfg.UnlockWrite()
				WriteError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			user.PasswordHash = string(hash)
			user.TokenVersion++
		}
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "user updated"})
	}
}

// DeleteUserHandler allows an admin to remove a user.
func DeleteUserHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := NormalizeUsername(r.PathValue("username"))
		claims := GetAuthClaims(r.Context())

		if claims != nil && claims.Username == username {
			WriteError(w, http.StatusForbidden, "Cannot delete your own account")
			return
		}

		usersCfg.LockWrite()
		userIndex := -1
		adminCount := 0
		for i, u := range usersCfg.Users {
			if u.Username == username {
				userIndex = i
			}
			if u.Level == config.LevelAdmin || u.Level == config.LevelExecute {
				adminCount++
			}
		}

		if userIndex == -1 {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		// Prevent locking out the system
		targetLevel := usersCfg.Users[userIndex].Level
		if (targetLevel == config.LevelAdmin || targetLevel == config.LevelExecute) && adminCount <= 1 {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusForbidden, "Cannot delete the last administrator")
			return
		}

		// Remove user
		usersCfg.Users = append(usersCfg.Users[:userIndex], usersCfg.Users[userIndex+1:]...)
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "user deleted"})
	}
}

// ListUsersHandler returns a user list for basic and admin users, including
// admin-only fields such as level and 2FA status when the requester is an admin.
func ListUsersHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		isAdmin := claims != nil && claims.Level == config.LevelAdmin

		type userListItem struct {
			Username    string            `json:"username"`
			DisplayName string            `json:"displayName"`
			Level       *config.UserLevel `json:"level,omitempty"`
			Has2FA      *bool             `json:"has2FA,omitempty"`
		}

		usersCfg.LockRead()
		resp := make([]userListItem, 0, len(usersCfg.Users))
		for _, u := range usersCfg.Users {
			item := userListItem{
				Username:    u.Username,
				DisplayName: u.DisplayName,
			}
			if isAdmin {
				l := u.Level
				h := u.TOTPSecret != ""
				item.Level = &l
				item.Has2FA = &h
			}
			resp = append(resp, item)
		}
		usersCfg.UnlockRead()
		WriteJSON(w, http.StatusOK, resp)
	}
}

// --- User Self-Service Handlers (Any Level) ---

type userProfileResponse struct {
	Username    string           `json:"username"`
	DisplayName string           `json:"displayName"`
	Level       config.UserLevel `json:"level"`
	Has2FA      bool             `json:"has2FA"`
}

// GetProfileHandler returns the profile information for the logged-in user.
func GetProfileHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		usersCfg.LockRead()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.UnlockRead()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		resp := userProfileResponse{
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Level:       claims.Level, // Use level from claims as it accounts for 2FA fallback
			Has2FA:      user.TOTPSecret != "",
		}
		usersCfg.UnlockRead()
		WriteJSON(w, http.StatusOK, resp)
	}
}

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

		usersCfg.LockWrite()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if req.DisplayName != "" {
			if err := ValidateDisplayName(req.DisplayName); err != nil {
				usersCfg.UnlockWrite()
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			user.DisplayName = req.DisplayName
		}
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
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
func ChangePasswordHandler(usersCfg *config.UsersConfig, limiter *LoginRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := GetAuthClaims(r.Context())
		if claims == nil {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		clientIP := limiter.ClientIP(r)
		if !limiter.Allow(clientIP) {
			WriteError(w, http.StatusTooManyRequests, "Too many attempts, please try again later")
			return
		}

		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		usersCfg.LockWrite()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			usersCfg.UnlockWrite()
			limiter.RecordFailure(clientIP)
			WriteError(w, http.StatusUnauthorized, "Invalid current password")
			return
		}

		if err := ValidatePasswordStrength(req.NewPassword); err != nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), BcryptCost)
		if err != nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		user.PasswordHash = string(hash)
		user.TokenVersion++
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		limiter.Reset(clientIP)
		WriteJSON(w, http.StatusOK, map[string]string{"status": "password updated"})
	}
}

// Setup2FAHandler generates a temporary TOTP secret for the user to set up their app.
// The secret is stored as a pending secret on the user record until activation.
func Setup2FAHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		usersCfg.LockWrite()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}
		user.PendingTOTPSecret = key.Secret()
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{
			"secret": key.Secret(),
			"uri":    key.URL(),
		})
	}
}

type activate2FARequest struct {
	Code string `json:"code"`
}

// Activate2FAHandler verifies a setup code and persists the TOTP secret.
// It uses the pending secret stored during setup rather than accepting a client-supplied secret.
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

		if req.Code == "" {
			WriteError(w, http.StatusBadRequest, "Code is required")
			return
		}

		usersCfg.LockWrite()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if user.PendingTOTPSecret == "" {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusBadRequest, "No pending 2FA setup. Call setup first.")
			return
		}

		if !totp.Validate(req.Code, user.PendingTOTPSecret) {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusBadRequest, "Invalid verification code")
			return
		}

		user.TOTPSecret = user.PendingTOTPSecret
		user.PendingTOTPSecret = ""
		user.TokenVersion++
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
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

		// Decode body before acquiring lock to avoid holding the lock during I/O.
		var req deactivate2FARequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		usersCfg.LockWrite()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.UnlockWrite()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		// If 2FA is enabled, require a valid code to disable it.
		if user.TOTPSecret != "" {
			if !totp.Validate(req.Code, user.TOTPSecret) {
				usersCfg.UnlockWrite()
				WriteError(w, http.StatusBadRequest, "Invalid verification code")
				return
			}
		}

		user.TOTPSecret = ""
		user.TokenVersion++
		cfgSnapshot := *usersCfg
		usersCfg.UnlockWrite()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "2FA disabled"})
	}
}
