package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
)

// UserLevel represents the authorization level of a user.
type UserLevel string

const (
	LevelBasic   UserLevel = "basic"
	LevelEdit    UserLevel = "edit"
	LevelAdmin   UserLevel = "admin"
	LevelExecute UserLevel = "execute"
)

// UserEntry represents a single user defined in the users.toml configuration file.
type UserEntry struct {
	Username          string    `toml:"username" mapstructure:"username"`
	PasswordHash      string    `toml:"passwordHash" mapstructure:"passwordHash"`
	DisplayName       string    `toml:"displayName" mapstructure:"displayName"`
	TOTPSecret        string    `toml:"totpSecret" mapstructure:"totpSecret"`
	PendingTOTPSecret string    `toml:"pendingTotpSecret,omitempty" mapstructure:"pendingTotpSecret"`
	Level             UserLevel `toml:"level" mapstructure:"level"`
	TokenVersion      int       `toml:"tokenVersion" mapstructure:"tokenVersion"`
}

// UsersConfig holds the authentication-related configuration loaded from users.toml.
type UsersConfig struct {
	JWTSecret          string        `toml:"jwtSecret,omitempty" mapstructure:"jwtSecret"`
	TokenLifetimeHours int           `toml:"tokenLifetimeHours" mapstructure:"tokenLifetimeHours"`
	Users              []UserEntry   `toml:"users" mapstructure:"users"`
	mu                 *sync.RWMutex `toml:"-" mapstructure:"-"` // Protects fields from concurrent access
}

// Lock locks the config for writing.
func (c *UsersConfig) Lock() {
	if c.mu != nil {
		c.mu.Lock()
	}
}

// Unlock unlocks the config.
func (c *UsersConfig) Unlock() {
	if c.mu != nil {
		c.mu.Unlock()
	}
}

// RLock locks the config for reading.
func (c *UsersConfig) RLock() {
	if c.mu != nil {
		c.mu.RLock()
	}
}

// RUnlock unlocks the config.
func (c *UsersConfig) RUnlock() {
	if c.mu != nil {
		c.mu.RUnlock()
	}
}

// NewUsersConfig creates an empty UsersConfig with the given JWT secret and
// token lifetime, ready for use (e.g. when creating a fresh users.toml).
// Unlike a bare UsersConfig{} literal, this ensures the config's internal
// lock is initialized so concurrent access is actually protected.
func NewUsersConfig(jwtSecret string, tokenLifetimeHours int) UsersConfig {
	return UsersConfig{
		JWTSecret:          jwtSecret,
		TokenLifetimeHours: tokenLifetimeHours,
		mu:                 &sync.RWMutex{},
	}
}

// LoadUsersConfig reads and validates the users.toml configuration from the given directory.
func LoadUsersConfig(configDir string) (UsersConfig, error) {
	v := viper.New()

	v.SetConfigName("users")
	v.SetConfigType("toml")
	v.AddConfigPath(configDir)

	v.SetDefault("tokenLifetimeHours", 24)

	if err := v.ReadInConfig(); err != nil {
		expectedPath := filepath.Join(configDir, "users.toml")
		return UsersConfig{}, fmt.Errorf("failed to read users config at %s: %w", expectedPath, err)
	}

	var cfg UsersConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return UsersConfig{}, fmt.Errorf("failed to unmarshal users config: %w", err)
	}
	cfg.mu = &sync.RWMutex{}

	// Set default levels for users that don't have one defined, and validate levels.
	for i := range cfg.Users {
		if cfg.Users[i].Level == "" {
			cfg.Users[i].Level = LevelBasic
		} else {
			l := cfg.Users[i].Level
			if l != LevelBasic && l != LevelEdit && l != LevelAdmin && l != LevelExecute {
				return UsersConfig{}, fmt.Errorf("invalid level %q for user %q in users.toml", l, cfg.Users[i].Username)
			}
		}
	}

	// Allow environment variable to override the file-based JWT secret.
	// This supports 12-factor-app deployments where secrets should not live in files.
	if envSecret := os.Getenv("JMAN_JWTSECRET"); envSecret != "" {
		cfg.JWTSecret = envSecret
		log.Println("INFO: JWT secret loaded from JMAN_JWTSECRET environment variable")
	} else if cfg.JWTSecret == "" {
		return UsersConfig{}, fmt.Errorf("jwtSecret must be set in users.toml or via the JMAN_JWTSECRET environment variable")
	} else {
		log.Println("INFO: JWT secret loaded from users.toml")
	}

	if cfg.JWTSecret == "your_64_char_hex_string_here" {
		return UsersConfig{}, fmt.Errorf("jwtSecret is still set to the placeholder value")
	}

	if len(cfg.JWTSecret) < 32 {
		return UsersConfig{}, fmt.Errorf("jwtSecret must be at least 32 characters long")
	}

	if len(cfg.Users) == 0 {
		return UsersConfig{}, fmt.Errorf("at least one user must be defined in users.toml")
	}

	// Decrypt TOTP secrets that are stored encrypted.
	encKey, err := DeriveEncryptionKey(cfg.JWTSecret)
	if err != nil {
		return UsersConfig{}, fmt.Errorf("failed to derive encryption key: %w", err)
	}
	for i := range cfg.Users {
		if cfg.Users[i].TOTPSecret != "" {
			decrypted, err := DecryptTOTPSecret(cfg.Users[i].TOTPSecret, encKey)
			if err != nil {
				return UsersConfig{}, fmt.Errorf("failed to decrypt TOTP secret for user %q: %w", cfg.Users[i].Username, err)
			}
			cfg.Users[i].TOTPSecret = decrypted
		}
		if cfg.Users[i].PendingTOTPSecret != "" {
			decrypted, err := DecryptTOTPSecret(cfg.Users[i].PendingTOTPSecret, encKey)
			if err != nil {
				return UsersConfig{}, fmt.Errorf("failed to decrypt pending TOTP secret for user %q: %w", cfg.Users[i].Username, err)
			}
			cfg.Users[i].PendingTOTPSecret = decrypted
		}
	}

	// Enforce strict file permissions on users.toml. This file contains password
	// hashes, TOTP secrets, and potentially the JWT signing key. Refuse to start
	// if the file is readable by group or others.
	usersFilePath := filepath.Join(configDir, "users.toml")
	info, err := os.Stat(usersFilePath)
	if err == nil {
		perm := info.Mode().Perm()
		if perm&0o077 != 0 {
			return UsersConfig{}, fmt.Errorf(
				"%s has permissions %04o, which are too open. "+
					"This file contains secrets and must not be accessible by group or others. "+
					"Fix with: chmod 600 %s",
				usersFilePath, perm, usersFilePath)
		}
	}

	return cfg, nil
}

// SaveUsersConfig marshals the given UsersConfig to TOML and writes it to
// users.toml in the specified config directory with restrictive permissions (0600).
// If the JWT secret was loaded from the JMAN_JWTSECRET environment variable,
// it is not written to the file.
func SaveUsersConfig(configDir string, cfg UsersConfig) error {
	cfg.RLock()
	defer cfg.RUnlock()

	// Deep-copy the Users slice so encryption doesn't mutate the caller's data
	// (slices share the underlying array even when the struct is passed by value).
	usersCopy := make([]UserEntry, len(cfg.Users))
	copy(usersCopy, cfg.Users)
	cfg.Users = usersCopy

	// Derive encryption key from the JWT secret BEFORE potentially clearing it.
	// This ensures we can encrypt TOTP secrets even when the secret comes from env.
	if cfg.JWTSecret != "" {
		encKey, err := DeriveEncryptionKey(cfg.JWTSecret)
		if err != nil {
			return fmt.Errorf("failed to derive encryption key: %w", err)
		}
		for i := range cfg.Users {
			if cfg.Users[i].TOTPSecret != "" && !IsEncryptedTOTPSecret(cfg.Users[i].TOTPSecret) {
				encrypted, err := EncryptTOTPSecret(cfg.Users[i].TOTPSecret, encKey)
				if err != nil {
					return fmt.Errorf("failed to encrypt TOTP secret for user %q: %w", cfg.Users[i].Username, err)
				}
				cfg.Users[i].TOTPSecret = encrypted
			}
			if cfg.Users[i].PendingTOTPSecret != "" && !IsEncryptedTOTPSecret(cfg.Users[i].PendingTOTPSecret) {
				encrypted, err := EncryptTOTPSecret(cfg.Users[i].PendingTOTPSecret, encKey)
				if err != nil {
					return fmt.Errorf("failed to encrypt pending TOTP secret for user %q: %w", cfg.Users[i].Username, err)
				}
				cfg.Users[i].PendingTOTPSecret = encrypted
			}
		}
	}

	// If the JWT secret is sourced from an environment variable, don't persist
	// it to the file. This keeps the secret out of disk storage.
	if os.Getenv("JMAN_JWTSECRET") != "" {
		cfg.JWTSecret = ""
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal users config: %w", err)
	}

	header := []byte("# jman-api user configuration\n# See README_API.md for details\n\n")
	content := append(header, data...)

	filePath := filepath.Join(configDir, "users.toml")

	// Atomic write: write to temp file then rename
	tempFile, err := os.CreateTemp(configDir, "users.toml.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file to %s: %w", filePath, err)
	}
	return nil
}

// FindUser searches the UsersConfig for a user with the given username.
// Returns a pointer to the matching UserEntry, or nil if no match is found.
func FindUser(cfg *UsersConfig, username string) *UserEntry {
	for i := range cfg.Users {
		if cfg.Users[i].Username == username {
			return &cfg.Users[i]
		}
	}
	return nil
}

// IsSafeIdentifier checks if a string is a safe SQL identifier (table or column name).
// It only allows alphanumeric characters and underscores.
func IsSafeIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}
