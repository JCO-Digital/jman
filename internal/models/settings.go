package models

import "time"

// Setting represents a user-specific key/value setting.
// The Value field is assumend to be a JSON object (stored as a string in the database), but can be any value that can be represented as a string.
type Setting struct {
	UserID    string    `json:"user_id"`
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
