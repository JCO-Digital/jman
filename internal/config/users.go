package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// UserEntry represents a single user defined in the users.toml configuration file.
type UserEntry struct {
	Username     string `toml:"username" mapstructure:"username"`
	PasswordHash string `toml:"passwordHash" mapstructure:"passwordHash"`
	DisplayName  string `toml:"displayName" mapstructure:"displayName"`
	TOTPSecret   string `toml:"totpSecret" mapstructure:"totpSecret"`
}

// UsersConfig holds the authentication-related configuration loaded from users.toml.
type UsersConfig struct {
	JWTSecret          string      `toml:"jwtSecret" mapstructure:"jwtSecret"`
	TokenLifetimeHours int         `toml:"tokenLifetimeHours" mapstructure:"tokenLifetimeHours"`
	Users              []UserEntry `toml:"users" mapstructure:"users"`
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

	if cfg.JWTSecret == "" {
		return UsersConfig{}, fmt.Errorf("jwtSecret is required in users.toml")
	}

	if len(cfg.JWTSecret) < 32 {
		return UsersConfig{}, fmt.Errorf("jwtSecret must be at least 32 characters long")
	}

	if len(cfg.Users) == 0 {
		return UsersConfig{}, fmt.Errorf("at least one user must be defined in users.toml")
	}

	// Check file permissions and warn if too open
	usersFilePath := filepath.Join(configDir, "users.toml")
	info, err := os.Stat(usersFilePath)
	if err == nil {
		perm := info.Mode().Perm()
		if perm&0o177 != 0 {
			log.Printf("WARNING: %s has permissions %o, which are more open than the recommended 0600. Consider running: chmod 600 %s", usersFilePath, perm, usersFilePath)
		}
	}

	return cfg, nil
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
