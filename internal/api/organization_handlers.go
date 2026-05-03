package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// getUsername extracts the username from the JWT context.
func getUsername(r *http.Request) string {
	claims := GetAuthClaims(r.Context())
	if claims != nil {
		return claims.Username
	}
	return "system"
}

// --- Organization Handlers ---

// ListOrganizationsHandler returns a list of all organizations, with optional search.
func ListOrganizationsHandler(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	organizations, err := db.GetAllOrganizations(search)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, organizations)
}

// CreateOrganizationHandler creates a new organization record.
func CreateOrganizationHandler(w http.ResponseWriter, r *http.Request) {
	var org models.Organization
	if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if org.Name == "" {
		WriteError(w, http.StatusBadRequest, "Organization name is required")
		return
	}

	username := getUsername(r)
	if err := db.SaveOrganization(&org, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, org)
}

// GetOrganizationHandler returns details for a specific organization.
func GetOrganizationHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	org, err := db.GetOrganization(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if org == nil {
		WriteError(w, http.StatusNotFound, "Organization not found")
		return
	}
	WriteJSON(w, http.StatusOK, org)
}

// UpdateOrganizationHandler updates an existing organization record.
func UpdateOrganizationHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var updates models.Organization
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	org, err := db.GetOrganization(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if org == nil {
		WriteError(w, http.StatusNotFound, "Organization not found")
		return
	}

	if updates.Name != "" {
		org.Name = updates.Name
	}
	org.VATNumber = updates.VATNumber
	org.Info = updates.Info

	username := getUsername(r)
	if err := db.SaveOrganization(org, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, org)
}

// DeleteOrganizationHandler deletes an organization record and all its dependencies.
func DeleteOrganizationHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteOrganization(id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Contact Handlers ---

// ListContactsHandler returns all contacts for a specific organization.
func ListContactsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	contacts, err := db.GetContactsByOrganization(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, contacts)
}

// ListOrganizationSitesHandler returns all sites linked to a specific organization.
func ListOrganizationSitesHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	siteIDs, err := db.GetSitesByOrganization(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Load all sites from cache to return full site objects
	allSites := []models.Site{}
	if err := cache.ReadJSONCache("sites", &allSites, cache.DefaultTTL); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Cache missing or expired: %v", err))
		return
	}

	// Filter sites by the IDs we found in DB
	orgSites := []models.Site{}
	idMap := make(map[int]bool)
	for _, sid := range siteIDs {
		idMap[sid] = true
	}

	for _, s := range allSites {
		if idMap[s.ID] {
			orgSites = append(orgSites, s)
		}
	}

	WriteJSON(w, http.StatusOK, orgSites)
}

// CreateContactHandler creates a new contact person for an organization.
func CreateContactHandler(w http.ResponseWriter, r *http.Request) {
	var contact models.Contact
	if err := json.NewDecoder(r.Body).Decode(&contact); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if contact.Name == "" || contact.OrganizationID == 0 {
		WriteError(w, http.StatusBadRequest, "Name and OrganizationID are required")
		return
	}

	username := getUsername(r)
	if err := db.SaveContact(&contact, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, contact)
}

// UpdateContactHandler updates an existing contact person.
func UpdateContactHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var updates models.Contact
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	contact, err := db.GetContact(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if contact == nil {
		WriteError(w, http.StatusNotFound, "Contact not found")
		return
	}

	if updates.Name != "" {
		contact.Name = updates.Name
	}
	contact.Email = updates.Email
	contact.Phone = updates.Phone
	if updates.Type != "" {
		contact.Type = updates.Type
	}

	username := getUsername(r)
	if err := db.SaveContact(contact, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, contact)
}

// DeleteContactHandler removes a contact person.
func DeleteContactHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteContact(id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Site Linking Handlers ---

// GetSiteOrganizationHandler returns the organization linked to a site.
func GetSiteOrganizationHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	org, err := db.GetOrganizationBySite(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if org == nil {
		WriteError(w, http.StatusNotFound, "No organization linked to this site")
		return
	}
	WriteJSON(w, http.StatusOK, org)
}

// LinkSiteHandler links a site to an organization.
func LinkSiteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	siteID, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	var body struct {
		OrganizationID int `json:"organization_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	username := getUsername(r)
	if err := db.LinkSiteToOrganization(siteID, body.OrganizationID, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// UnlinkSiteHandler removes the link between a site and its organization.
func UnlinkSiteHandler(w http.ResponseWriter, r *http.Request) {
	siteIDStr := r.PathValue("id")
	siteID, err := strconv.Atoi(siteIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	org, err := db.GetOrganizationBySite(siteID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if org == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := db.UnlinkSiteFromOrganization(siteID, org.ID); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Note Handlers ---

// ListNotesHandler returns notes for a specific organization or site.
func ListNotesHandler(w http.ResponseWriter, r *http.Request) {
	parentType := models.NoteParentType(r.URL.Query().Get("type"))
	parentIDStr := r.URL.Query().Get("id")
	parentID, err := strconv.Atoi(parentIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid parent ID")
		return
	}

	if parentType != models.NoteParentTypeOrganization && parentType != models.NoteParentTypeSite {
		WriteError(w, http.StatusBadRequest, "Invalid parent type")
		return
	}

	notes, err := db.GetNotes(parentType, parentID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, notes)
}

// CreateNoteHandler creates a new note.
func CreateNoteHandler(w http.ResponseWriter, r *http.Request) {
	var note models.Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if note.Content == "" || note.ParentID == 0 || note.ParentType == "" {
		WriteError(w, http.StatusBadRequest, "Content, ParentID and ParentType are required")
		return
	}

	username := getUsername(r)
	if err := db.SaveNote(&note, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, note)
}

// UpdateNoteHandler updates an existing note's content.
func UpdateNoteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var updates struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	note, err := db.GetNote(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if note == nil {
		WriteError(w, http.StatusNotFound, "Note not found")
		return
	}

	note.Content = updates.Content
	username := getUsername(r)
	if err := db.SaveNote(note, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, note)
}

// DeleteNoteHandler removes a note.
func DeleteNoteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteNote(id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
