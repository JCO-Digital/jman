package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// --- Payment Method Handlers ---

// ListPaymentMethodsHandler returns all payment methods.
func ListPaymentMethodsHandler(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	pmType := r.URL.Query().Get("type")
	methods, err := db.GetAllPaymentMethods(search, pmType)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListPaymentMethodsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, methods)
}

// CreatePaymentMethodHandler creates a new payment method.
func CreatePaymentMethodHandler(w http.ResponseWriter, r *http.Request) {
	var pm models.PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&pm); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if pm.Name == "" || pm.Type == "" {
		WriteError(w, http.StatusBadRequest, "Name and Type are required")
		return
	}

	pm.ID = 0
	username := getUsername(r)
	if err := db.SavePaymentMethod(&pm, username); err != nil {
		verb.LogPrintf(verb.Normal, "CreatePaymentMethodHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusCreated, pm)
}

// GetPaymentMethodHandler returns a specific payment method.
func GetPaymentMethodHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	pm, err := db.GetPaymentMethod(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "GetPaymentMethodHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if pm == nil {
		WriteError(w, http.StatusNotFound, "Payment method not found")
		return
	}
	WriteJSON(w, http.StatusOK, pm)
}

// UpdatePaymentMethodHandler updates an existing payment method.
func UpdatePaymentMethodHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var updates models.PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	pm, err := db.GetPaymentMethod(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "UpdatePaymentMethodHandler: failed to get payment method: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if pm == nil {
		WriteError(w, http.StatusNotFound, "Payment method not found")
		return
	}

	if updates.Name != "" {
		pm.Name = updates.Name
	}
	if updates.Type != "" {
		pm.Type = updates.Type
	}
	pm.ExpiryDate = updates.ExpiryDate

	username := getUsername(r)
	if err := db.SavePaymentMethod(pm, username); err != nil {
		verb.LogPrintf(verb.Normal, "UpdatePaymentMethodHandler: failed to save payment method: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, pm)
}

// DeletePaymentMethodHandler removes a payment method.
func DeletePaymentMethodHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := db.DeletePaymentMethod(id); err != nil {
		verb.LogPrintf(verb.Normal, "DeletePaymentMethodHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
