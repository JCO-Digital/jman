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

// --- Company Handlers ---

// ListCompaniesHandler returns a list of all companies, with optional search.
func ListCompaniesHandler(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	companies, err := db.GetAllCompanies(search)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, companies)
}

// CreateCompanyHandler creates a new company record.
func CreateCompanyHandler(w http.ResponseWriter, r *http.Request) {
	var company models.Company
	if err := json.NewDecoder(r.Body).Decode(&company); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if company.Name == "" {
		WriteError(w, http.StatusBadRequest, "Company name is required")
		return
	}

	username := getUsername(r)
	if err := db.SaveCompany(&company, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, company)
}

// GetCompanyHandler returns details for a specific company.
func GetCompanyHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	company, err := db.GetCompany(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if company == nil {
		WriteError(w, http.StatusNotFound, "Company not found")
		return
	}
	WriteJSON(w, http.StatusOK, company)
}

// UpdateCompanyHandler updates an existing company record.
func UpdateCompanyHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var updates models.Company
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	company, err := db.GetCompany(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if company == nil {
		WriteError(w, http.StatusNotFound, "Company not found")
		return
	}

	if updates.Name != "" {
		company.Name = updates.Name
	}
	company.VATNumber = updates.VATNumber
	company.Info = updates.Info

	username := getUsername(r)
	if err := db.SaveCompany(company, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, company)
}

// DeleteCompanyHandler deletes a company record and all its dependencies.
func DeleteCompanyHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteCompany(id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Contact Handlers ---

// ListContactsHandler returns all contacts for a specific company.
func ListContactsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid company ID")
		return
	}

	contacts, err := db.GetContactsByCompany(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, contacts)
}

// ListCompanySitesHandler returns all sites linked to a specific company.
func ListCompanySitesHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid company ID")
		return
	}

	siteIDs, err := db.GetSitesByCompany(id)
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
	companySites := []models.Site{}
	idMap := make(map[int]bool)
	for _, sid := range siteIDs {
		idMap[sid] = true
	}

	for _, s := range allSites {
		if idMap[s.ID] {
			companySites = append(companySites, s)
		}
	}

	WriteJSON(w, http.StatusOK, companySites)
}

// CreateContactHandler creates a new contact person for a company.
func CreateContactHandler(w http.ResponseWriter, r *http.Request) {
	var contact models.Contact
	if err := json.NewDecoder(r.Body).Decode(&contact); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if contact.Name == "" || contact.CompanyID == 0 {
		WriteError(w, http.StatusBadRequest, "Name and CompanyID are required")
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

// GetSiteCompanyHandler returns the company linked to a site.
func GetSiteCompanyHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	company, err := db.GetCompanyBySite(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if company == nil {
		WriteError(w, http.StatusNotFound, "No company linked to this site")
		return
	}
	WriteJSON(w, http.StatusOK, company)
}

// LinkSiteHandler links a site to a company.
func LinkSiteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	siteID, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	var body struct {
		CompanyID int `json:"company_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	username := getUsername(r)
	if err := db.LinkSiteToCompany(siteID, body.CompanyID, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// UnlinkSiteHandler removes the link between a site and its company.
func UnlinkSiteHandler(w http.ResponseWriter, r *http.Request) {
	siteIDStr := r.PathValue("id")
	siteID, err := strconv.Atoi(siteIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	company, err := db.GetCompanyBySite(siteID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if company == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := db.UnlinkSiteFromCompany(siteID, company.ID); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Note Handlers ---

// ListNotesHandler returns notes for a specific company or site.
func ListNotesHandler(w http.ResponseWriter, r *http.Request) {
	parentType := models.NoteParentType(r.URL.Query().Get("type"))
	parentIDStr := r.URL.Query().Get("id")
	parentID, err := strconv.Atoi(parentIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid parent ID")
		return
	}

	if parentType != models.NoteParentTypeCompany && parentType != models.NoteParentTypeSite {
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
