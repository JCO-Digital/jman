package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// ListIgnoreEntriesHandler handles listing all ignore entries.
func ListIgnoreEntriesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	entryType := r.URL.Query().Get("type")
	entries, err := db.GetAllIgnoreEntries(entryType)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListIgnoreEntriesHandler: failed to fetch entries: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, entries)
}

// CreateIgnoreEntryHandler handles adding a new ignore entry.
func CreateIgnoreEntryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var entry models.IgnoreEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if entry.Type == "" || entry.Target == "" {
		WriteError(w, http.StatusBadRequest, "Type and target are required")
		return
	}

	// Force creation of a new entry by ignoring any ID provided in the body.
	entry.ID = 0

	if err := normalizeIgnoreEntry(&entry); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	claims := GetAuthClaims(r.Context())
	username := "api"
	if claims != nil {
		username = claims.Username
	}

	if err := db.SaveIgnoreEntry(&entry, username); err != nil {
		verb.LogPrintf(verb.Normal, "CreateIgnoreEntryHandler: failed to save entry: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusCreated, entry)
}

// UpdateIgnoreEntryHandler handles updating an existing ignore entry.
func UpdateIgnoreEntryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", "PATCH")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	existing, err := db.GetIgnoreEntry(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "UpdateIgnoreEntryHandler: failed to fetch entry: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Entry not found")
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Apply updates
	if val, ok := updates["reason"].(string); ok {
		existing.Reason = val
	}
	if val, ok := updates["use_for_monitor"].(bool); ok {
		existing.UseForMonitor = val
	}
	if val, ok := updates["use_for_vuln"].(bool); ok {
		existing.UseForVuln = val
	}
	if val, ok := updates["negated_site_ids"].([]interface{}); ok {
		ids := make([]string, 0, len(val))
		for _, v := range val {
			switch id := v.(type) {
			case string:
				ids = append(ids, id)
			case float64:
				ids = append(ids, strconv.Itoa(int(id)))
			}
		}
		existing.NegatedSiteIDs = ids
	}
	if err := normalizeIgnoreEntry(existing); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	claims := GetAuthClaims(r.Context())
	username := "api"
	if claims != nil {
		username = claims.Username
	}

	if err := db.SaveIgnoreEntry(existing, username); err != nil {
		verb.LogPrintf(verb.Normal, "UpdateIgnoreEntryHandler: failed to update entry: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, existing)
}

// DeleteIgnoreEntryHandler handles removing an ignore entry.
func DeleteIgnoreEntryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteIgnoreEntry(id); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteIgnoreEntryHandler: failed to delete entry: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// normalizeIgnoreEntry canonicalises site/server targets and negated site IDs
// to UUIDs (accepting legacy SpinupWP integers).
func normalizeIgnoreEntry(entry *models.IgnoreEntry) error {
	var err error
	switch entry.Type {
	case "site":
		entry.Target, err = resolveSiteUUID(entry.Target)
	case "server":
		entry.Target, err = resolveServerUUID(entry.Target)
	}
	if err != nil {
		return err
	}

	negated := make([]string, 0, len(entry.NegatedSiteIDs))
	for _, id := range entry.NegatedSiteIDs {
		siteID, err := resolveSiteUUID(id)
		if err != nil {
			return fmt.Errorf("negated_site_ids: %w", err)
		}
		negated = append(negated, siteID)
	}
	entry.NegatedSiteIDs = negated
	return nil
}
