package api

import (
	"encoding/json"
	"net/http"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// ListSettingsHandler returns all settings for the authenticated user.
func ListSettingsHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	settings, err := db.GetAllSettings(claims.Username)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to get settings for user %s: %v", claims.Username, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Ensure we return an empty array instead of null if no settings exist
	if settings == nil {
		settings = []models.Setting{}
	}

	WriteJSON(w, http.StatusOK, settings)
}

// GetSettingHandler returns a specific setting for the authenticated user by key.
func GetSettingHandler(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		WriteError(w, http.StatusBadRequest, "Key is required")
		return
	}

	claims := GetAuthClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	setting, err := db.GetSetting(claims.Username, key)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to get setting %s for user %s: %v", key, claims.Username, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if setting == nil {
		WriteError(w, http.StatusNotFound, "Setting not found")
		return
	}

	WriteJSON(w, http.StatusOK, setting)
}

// SaveSettingHandler creates or replaces a setting for the authenticated user.
func SaveSettingHandler(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		WriteError(w, http.StatusBadRequest, "Key is required")
		return
	}

	claims := GetAuthClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var value any
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	setting, err := db.SaveSetting(claims.Username, key, value)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to save setting %s for user %s: %v", key, claims.Username, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, setting)
}

// PatchSettingHandler creates or updates a setting by merging the new value with the existing one.
// This merge only applies if both the existing value and the new value are JSON objects (maps).
func PatchSettingHandler(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		WriteError(w, http.StatusBadRequest, "Key is required")
		return
	}

	claims := GetAuthClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var patchValue any
	if err := json.NewDecoder(r.Body).Decode(&patchValue); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	existing, err := db.GetSetting(claims.Username, key)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to get existing setting %s for user %s: %v", key, claims.Username, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var finalValue any
	if existing != nil {
		// If both are maps, merge them
		existingMap, ok1 := existing.Value.(map[string]any)
		patchMap, ok2 := patchValue.(map[string]any)
		if ok1 && ok2 {
			for k, v := range patchMap {
				existingMap[k] = v
			}
			finalValue = existingMap
		} else {
			// If either is not a map, PATCH behaves like POST (replace)
			finalValue = patchValue
		}
	} else {
		// No existing setting, just use the new value
		finalValue = patchValue
	}

	setting, err := db.SaveSetting(claims.Username, key, finalValue)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to patch setting %s for user %s: %v", key, claims.Username, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, setting)
}

// DeleteSettingHandler removes a setting for the authenticated user.
func DeleteSettingHandler(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		WriteError(w, http.StatusBadRequest, "Key is required")
		return
	}

	claims := GetAuthClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	if err := db.DeleteSetting(claims.Username, key); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to delete setting %s for user %s: %v", key, claims.Username, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
