package api

import (
	"encoding/json"
	"fmt"
	"math"
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

// ValidatePasswordStrength enforces a minimum entropy based on character pools.
// The requirement is at least 200,000,000,000,000 variations, calculated as (poolSize ^ length).
func ValidatePasswordStrength(password string) error {
	if len(password) == 0 {
		return fmt.Errorf("password cannot be empty")
	}

	var basePool int
	var hasLower, hasUpper, hasDigit, hasSpecial bool

	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			// Treat anything else as a special character (pool size 16)
			hasSpecial = true
		}
	}

	if hasLower {
		basePool += 26
	}
	if hasUpper {
		basePool += 26
	}
	if hasDigit {
		basePool += 10
	}
	if hasSpecial {
		basePool += 16
	}

	if basePool == 0 {
		return fmt.Errorf("password contains invalid characters")
	}

	// Calculate log10 of total variations: length * log10(basePool)
	// We require at least 200,000,000,000,000 variations.
	variationsLog10 := float64(len(password)) * math.Log10(float64(basePool))
	if variationsLog10 < math.Log10(200_000_000_000_000) {
		return fmt.Errorf("password is too weak: try a longer password or use more character types")
	}

	return nil
}
