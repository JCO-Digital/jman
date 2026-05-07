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

		usersCfg.RLock()
		exists := config.FindUser(usersCfg, req.Username) != nil
		usersCfg.RUnlock()
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

		usersCfg.Lock()
		// Re-check under write lock to prevent TOCTOU race condition.
		if config.FindUser(usersCfg, req.Username) != nil {
			usersCfg.Unlock()
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
		usersCfg.Unlock()

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

		usersCfg.Lock()
		user := config.FindUser(usersCfg, username)
		if user == nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		if req.DisplayName != "" {
			if err := ValidateDisplayName(req.DisplayName); err != nil {
				usersCfg.Unlock()
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			user.DisplayName = req.DisplayName
		}
		if req.Level != "" {
			if err := ValidateUserLevel(req.Level); err != nil {
				usersCfg.Unlock()
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
					usersCfg.Unlock()
					WriteError(w, http.StatusForbidden, "Cannot demote the last administrator")
					return
				}
			}
			user.Level = req.Level
			user.TokenVersion++
		}
		if req.Password != "" {
			if err := ValidatePasswordStrength(req.Password); err != nil {
				usersCfg.Unlock()
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), BcryptCost)
			if err != nil {
				usersCfg.Unlock()
				WriteError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			user.PasswordHash = string(hash)
			user.TokenVersion++
		}
		cfgSnapshot := *usersCfg
		usersCfg.Unlock()

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

		usersCfg.Lock()
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
			usersCfg.Unlock()
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		// Prevent locking out the system
		targetLevel := usersCfg.Users[userIndex].Level
		if (targetLevel == config.LevelAdmin || targetLevel == config.LevelExecute) && adminCount <= 1 {
			usersCfg.Unlock()
			WriteError(w, http.StatusForbidden, "Cannot delete the last administrator")
			return
		}

		// Remove user
		usersCfg.Users = append(usersCfg.Users[:userIndex], usersCfg.Users[userIndex+1:]...)
		cfgSnapshot := *usersCfg
		usersCfg.Unlock()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
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

		usersCfg.RLock()
		var resp []userListItem
		for _, u := range usersCfg.Users {
			resp = append(resp, userListItem{
				Username:    u.Username,
				DisplayName: u.DisplayName,
				Level:       u.Level,
				Has2FA:      u.TOTPSecret != "",
			})
		}
		usersCfg.RUnlock()
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

		usersCfg.RLock()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.RUnlock()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		resp := userProfileResponse{
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Level:       claims.Level, // Use level from claims as it accounts for 2FA fallback
			Has2FA:      user.TOTPSecret != "",
		}
		usersCfg.RUnlock()
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

		usersCfg.Lock()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if req.DisplayName != "" {
			if err := ValidateDisplayName(req.DisplayName); err != nil {
				usersCfg.Unlock()
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			user.DisplayName = req.DisplayName
		}
		cfgSnapshot := *usersCfg
		usersCfg.Unlock()

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

		usersCfg.Lock()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusUnauthorized, "Invalid current password")
			return
		}

		if err := ValidatePasswordStrength(req.NewPassword); err != nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), BcryptCost)
		if err != nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		user.PasswordHash = string(hash)
		user.TokenVersion++
		cfgSnapshot := *usersCfg
		usersCfg.Unlock()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
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

		usersCfg.Lock()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		user.TOTPSecret = req.Secret
		user.TokenVersion++
		cfgSnapshot := *usersCfg
		usersCfg.Unlock()

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

		usersCfg.Lock()
		user := config.FindUser(usersCfg, claims.Username)
		if user == nil {
			usersCfg.Unlock()
			WriteError(w, http.StatusUnauthorized, "User no longer exists")
			return
		}

		// If 2FA is enabled, require a valid code to disable it.
		if user.TOTPSecret != "" {
			if !totp.Validate(req.Code, user.TOTPSecret) {
				usersCfg.Unlock()
				WriteError(w, http.StatusBadRequest, "Invalid verification code")
				return
			}
		}

		user.TOTPSecret = ""
		user.TokenVersion++
		cfgSnapshot := *usersCfg
		usersCfg.Unlock()

		if err := config.SaveUsersConfig(config.RunData.ConfigDir, cfgSnapshot); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"status": "2FA disabled"})
	}
}
