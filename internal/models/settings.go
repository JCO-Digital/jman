package models

import "time"

// Setting represents a user-specific key/value setting.
// The Value field is expected to be a JSON object (stored as a string in the database).
type Setting struct {
	UserID    string    `json:"user_id"`
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
