package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// --- Global Asset Handlers ---

// ListAssetsHandler returns all asset templates.
func ListAssetsHandler(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	assets, err := db.GetAllAssets(search)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, assets)
}

// CreateAssetHandler creates a new asset template.
func CreateAssetHandler(w http.ResponseWriter, r *http.Request) {
	var asset models.Asset
	if err := json.NewDecoder(r.Body).Decode(&asset); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if asset.Name == "" || asset.Type == "" {
		WriteError(w, http.StatusBadRequest, "Name and Type are required")
		return
	}

	asset.ID = 0
	username := getUsername(r)
	if err := db.SaveAsset(&asset, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, asset)
}

// GetAssetHandler returns a specific asset template.
func GetAssetHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	asset, err := db.GetAsset(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if asset == nil {
		WriteError(w, http.StatusNotFound, "Asset not found")
		return
	}
	WriteJSON(w, http.StatusOK, asset)
}

// UpdateAssetHandler updates an existing asset template.
func UpdateAssetHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var updates models.Asset
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	asset, err := db.GetAsset(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if asset == nil {
		WriteError(w, http.StatusNotFound, "Asset not found")
		return
	}

	if updates.Name != "" {
		asset.Name = updates.Name
	}
	if updates.Type != "" {
		asset.Type = updates.Type
	}
	asset.Identifier = updates.Identifier
	asset.Description = updates.Description
	asset.DefaultPrice = updates.DefaultPrice
	asset.DefaultFreq = updates.DefaultFreq
	asset.Active = updates.Active

	username := getUsername(r)
	if err := db.SaveAsset(asset, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, asset)
}

// DeleteAssetHandler removes an asset template.
func DeleteAssetHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteAsset(id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Organization Asset Handlers ---

// ListOrganizationAssetsHandler returns assets for a specific organization.
func ListOrganizationAssetsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	assets, err := db.GetOrganizationAssetsByOrganization(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, assets)
}

// CreateOrganizationAssetHandler links an asset to an organization.
func CreateOrganizationAssetHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	orgID, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid organization ID")
		return
	}

	var oa models.OrganizationAsset
	if err := json.NewDecoder(r.Body).Decode(&oa); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	oa.OrganizationID = orgID
	oa.ID = 0

	// If asset_id is provided, we might want to default price/freq if not set
	if oa.AssetID != nil && *oa.AssetID > 0 {
		template, err := db.GetAsset(*oa.AssetID)
		if err == nil && template != nil {
			if oa.Price == 0 {
				oa.Price = template.DefaultPrice
			}
			if oa.BillingFreq == "" {
				oa.BillingFreq = template.DefaultFreq
			}
			if oa.Identifier == "" && template.Type != models.AssetTypeDomain {
				oa.Identifier = template.Identifier
			}
		}
	}

	if oa.Status == "" {
		oa.Status = models.AssetStatusActive
	}

	username := getUsername(r)
	if err := db.SaveOrganizationAsset(&oa, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, oa)
}

// GetOrganizationAssetHandler returns a specific organization asset link.
func GetOrganizationAssetHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	oa, err := db.GetOrganizationAsset(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if oa == nil {
		WriteError(w, http.StatusNotFound, "Organization asset not found")
		return
	}
	WriteJSON(w, http.StatusOK, oa)
}

// UpdateOrganizationAssetHandler updates an organization asset link.
func UpdateOrganizationAssetHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var updates models.OrganizationAsset
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	oa, err := db.GetOrganizationAsset(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if oa == nil {
		WriteError(w, http.StatusNotFound, "Organization asset not found")
		return
	}

	oa.SiteID = updates.SiteID
	oa.AssetID = updates.AssetID
	if updates.Identifier != "" {
		oa.Identifier = updates.Identifier
	}
	oa.Price = updates.Price
	if updates.BillingFreq != "" {
		oa.BillingFreq = updates.BillingFreq
	}
	oa.NextBilling = updates.NextBilling
	if updates.Status != "" {
		oa.Status = updates.Status
	}
	oa.Description = updates.Description

	username := getUsername(r)
	if err := db.SaveOrganizationAsset(oa, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, oa)
}

// DeleteOrganizationAssetHandler removes an organization asset link.
func DeleteOrganizationAssetHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteOrganizationAsset(id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Asset Payment Handlers ---

// ListAssetPaymentsHandler returns payment history for an organization asset.
func ListAssetPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid organization asset ID")
		return
	}

	payments, err := db.GetAssetPaymentsByAsset(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, payments)
}

// CreateAssetPaymentHandler records a new payment.
func CreateAssetPaymentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	oaID, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid organization asset ID")
		return
	}

	var payment models.AssetPayment
	if err := json.NewDecoder(r.Body).Decode(&payment); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	payment.OrgAssetID = oaID
	payment.ID = 0
	if payment.PaymentDate.IsZero() {
		payment.PaymentDate = time.Now()
	}

	username := getUsername(r)
	if err := db.SaveAssetPayment(&payment, username); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, payment)
}

// DeleteAssetPaymentHandler removes a payment record.
func DeleteAssetPaymentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeleteAssetPayment(id); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
