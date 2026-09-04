package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/verb"
)

// vulnSettingsResponse is the request/response shape for the global
// vulnerability-task settings.
type vulnSettingsResponse struct {
	DefaultAssignee string `json:"defaultAssignee"`
}

// GetVulnSettingsHandler returns the global vulnerability-task settings.
func GetVulnSettingsHandler(w http.ResponseWriter, r *http.Request) {
	setting, err := db.GetSetting(db.SystemSettingsUserID, db.DefaultVulnAssigneeSettingKey)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to get default vuln assignee setting: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	resp := vulnSettingsResponse{}
	if setting != nil {
		if v, ok := setting.Value.(string); ok {
			resp.DefaultAssignee = v
		}
	}
	WriteJSON(w, http.StatusOK, resp)
}

// SaveVulnSettingsHandler updates the global vulnerability-task settings. An
// empty/omitted defaultAssignee clears the setting, leaving newly-created
// vulnerability Tasks unassigned.
func SaveVulnSettingsHandler(usersCfg *config.UsersConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req vulnSettingsResponse
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}

		assignee := strings.TrimSpace(req.DefaultAssignee)
		if assignee != "" && config.FindUser(usersCfg, assignee) == nil {
			WriteError(w, http.StatusBadRequest, "Unknown username")
			return
		}

		if _, err := db.SaveSetting(db.SystemSettingsUserID, db.DefaultVulnAssigneeSettingKey, assignee); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to save default vuln assignee setting: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		WriteJSON(w, http.StatusOK, vulnSettingsResponse{DefaultAssignee: assignee})
	}
}
