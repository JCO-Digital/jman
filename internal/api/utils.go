package api

import (
	"encoding/json"
	"net/http"

	"github.com/JCO-Digital/jman/internal/verb"
)

// ErrorResponse represents a standard JSON error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteJSON encodes data as JSON and writes it to the response writer.
// It sets the Content-Type header to application/json.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		verb.LogPrintf(verb.Normal, "Error encoding JSON: %v", err)
	}
}

// WriteError writes a JSON error response with the given HTTP status code.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message})
}
