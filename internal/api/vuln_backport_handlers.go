package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/vuln"
)

// CreateVulnBackportRequest is the body for creating a backport rule.
type CreateVulnBackportRequest struct {
	Type       string `json:"type"`
	Slug       string `json:"slug"`
	VulnUUID   string `json:"vuln_uuid"`
	Branch     string `json:"branch"`
	MinVersion string `json:"min_version"`
	Reason     string `json:"reason"`
}

// CreateVulnBackportResponse returns the saved rule, the number of sites it
// currently matches, and any non-blocking validation warnings.
type CreateVulnBackportResponse struct {
	Rule     models.VulnBackportListing `json:"rule"`
	Warnings []string                   `json:"warnings"`
}

// ListVulnBackportsHandler lists all backport rules with their matched-site counts.
func ListVulnBackportsHandler(w http.ResponseWriter, r *http.Request) {
	listings, err := vuln.ListVulnBackports()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListVulnBackportsHandler: failed to list rules: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, listings)
}

// CreateVulnBackportHandler validates and saves a new backport rule.
func CreateVulnBackportHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateVulnBackportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	rule := models.VulnBackport{
		Type:       strings.TrimSpace(req.Type),
		Slug:       strings.TrimSpace(req.Slug),
		VulnUUID:   strings.TrimSpace(req.VulnUUID),
		Branch:     strings.TrimSpace(req.Branch),
		MinVersion: strings.TrimSpace(req.MinVersion),
		Reason:     strings.TrimSpace(req.Reason),
	}

	warnings, err := vuln.ValidateVulnBackport(rule)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if warnings == nil {
		warnings = []string{}
	}

	username := "api"
	if claims := GetAuthClaims(r.Context()); claims != nil {
		username = claims.Username
	}

	if err := db.CreateVulnBackport(&rule, username); err != nil {
		if errors.Is(err, db.ErrDuplicateVulnBackport) {
			WriteError(w, http.StatusConflict, err.Error())
			return
		}
		verb.LogPrintf(verb.Normal, "CreateVulnBackportHandler: failed to save rule: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	matched, err := vuln.CountVulnBackportMatches(rule)
	if err != nil {
		// The rule is saved; only the count for the confirmation is missing.
		verb.LogPrintf(verb.Normal, "CreateVulnBackportHandler: failed to count matches: %v", err)
	}

	WriteJSON(w, http.StatusCreated, CreateVulnBackportResponse{
		Rule:     models.VulnBackportListing{VulnBackport: rule, MatchedSites: matched},
		Warnings: warnings,
	})
}

// DeleteVulnBackportHandler removes a backport rule.
func DeleteVulnBackportHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	existing, err := db.GetVulnBackport(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteVulnBackportHandler: failed to fetch rule: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Rule not found")
		return
	}

	if err := db.DeleteVulnBackport(id); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteVulnBackportHandler: failed to delete rule: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
