package config

import (
	"testing"
)

func TestIsSafeIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"simple table name", "plugin_info", true},
		{"simple column name", "slug", true},
		{"alphanumeric", "user123", true},
		{"mixed case", "MonitorStatus", true},
		{"empty string", "", false},
		{"contains space", "table name", false},
		{"contains semicolon", "users; DROP TABLE sites", false},
		{"contains dash", "user-data", false},
		{"contains single quote", "'users'", false},
		{"contains double quote", "\"users\"", false},
		{"contains asterisk", "*", false},
		{"contains backtick", "`users`", false},
		{"starts with number", "123user", true}, // SQL allows it if quoted, but our validator is strict
		{"contains dot", "db.users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSafeIdentifier(tt.input); got != tt.expected {
				t.Errorf("IsSafeIdentifier(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFindUser(t *testing.T) {
	cfg := &UsersConfig{
		Users: []UserEntry{
			{Username: "admin", DisplayName: "Admin"},
			{Username: "editor", DisplayName: "Editor"},
		},
	}

	t.Run("existing user", func(t *testing.T) {
		user := FindUser(cfg, "admin")
		if user == nil || user.DisplayName != "Admin" {
			t.Errorf("Expected to find admin, got %v", user)
		}
	})

	t.Run("non-existing user", func(t *testing.T) {
		user := FindUser(cfg, "nonexistent")
		if user != nil {
			t.Errorf("Expected nil for non-existing user, got %v", user)
		}
	})
}
