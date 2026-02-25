package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/pelletier/go-toml/v2"
)

const AppName = "jman"

// Runtime holds the dynamic paths and version information for the application.
type Runtime struct {
	ConfigDir string
	CacheDir  string
	DataDir   string
	Version   string
}

// AppConfig represents the user-defined settings mapped from the TOML config file.
type AppConfig struct {
	TokenSpinup   string   `toml:"tokenSpinup"`
	TokenSlack    string   `toml:"slackToken"`
	SlackChannel  string   `toml:"slackChannel"`
	CVSSThreshold float64  `toml:"cvssThreshold"`
	VulnThreshold float64  `toml:"vulnThreshold"`
	IgnoreSites   []string `toml:"ignoreSites"`
}

var (
	RunData Runtime
	Cfg     AppConfig
)

// Init sets up the application runtime directories and loads the configuration.
func Init(version string) error {
	RunData = Runtime{
		ConfigDir: filepath.Join(xdg.ConfigHome, AppName),
		CacheDir:  filepath.Join(xdg.CacheHome, AppName),
		DataDir:   filepath.Join(xdg.DataHome, AppName),
		Version:   version,
	}

	// Ensure directories exist
	dirs := []string{RunData.ConfigDir, RunData.CacheDir, RunData.DataDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return loadConfig()
}

// loadConfig reads and parses the TOML configuration file.
func loadConfig() error {
	// Set defaults
	Cfg = AppConfig{
		SlackChannel:  "#testing",
		CVSSThreshold: 7.0,
		VulnThreshold: 7.0,
		IgnoreSites:   []string{},
	}

	configPath := filepath.Join(RunData.ConfigDir, "config.toml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config file not found at %s. Please create it and add your credentials", configPath)
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := toml.Unmarshal(data, &Cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}
