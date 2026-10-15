package models

import "time"

// CLIToken is a jman CLI API token, issued to one device through the
// browser login. The token itself is only shown once, to the CLI.
type CLIToken struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	DeviceName   string    `json:"device_name"`
	TokenPrefix  string    `json:"token_prefix"`
	TokenVersion int       `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
	// ExpiresAt is when the token expires if it isn't used before then.
	ExpiresAt time.Time `json:"expires_at"`
}
