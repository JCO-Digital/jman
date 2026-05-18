package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
)

const AppName = "jman"

var AppVersion = "dev"

// Runtime holds the dynamic paths and version information for the application.
type Runtime struct {
	ConfigDir string
	CacheDir  string
	DataDir   string
	BackupDir string
	Version   string
}

// AppConfig represents the user-defined settings mapped from the config file or environment variables.
type AppConfig struct {
	TokenSpinup         string            `toml:"tokenSpinup" mapstructure:"tokenSpinup"`
	TokenSlack          string            `toml:"slackToken" mapstructure:"slackToken"`
	SlackChannel        string            `toml:"slackChannel" mapstructure:"slackChannel"`
	SlackMonitorChannel string            `toml:"slackMonitorChannel" mapstructure:"slackMonitorChannel"`
	SlackTasksChannel   string            `toml:"slackTasksChannel" mapstructure:"slackTasksChannel"`
	MonitorThreshold    int               `toml:"monitorThreshold" mapstructure:"monitorThreshold"`
	MonitorTimeout      int               `toml:"monitorTimeout" mapstructure:"monitorTimeout"`
	MonitorCacheBypass  bool              `toml:"monitorCacheBypass" mapstructure:"monitorCacheBypass"`
	CVSSThreshold       float64           `toml:"cvssThreshold" mapstructure:"cvssThreshold"`
	VulnThreshold       float64           `toml:"vulnThreshold" mapstructure:"vulnThreshold"`
	BehindProxy         bool              `toml:"behindProxy" mapstructure:"behindProxy"`
	AllowedOrigins      []string          `toml:"allowedOrigins" mapstructure:"allowedOrigins"`
	IgnoreSites         []string          `toml:"ignoreSites" mapstructure:"ignoreSites"`
	PluginAliases       map[string]string `toml:"pluginAliases" mapstructure:"pluginAliases"`
}

var (
	RunData Runtime
	Cfg     AppConfig
)

// Init sets up the application runtime directories and loads the configuration.
func Init() error {
	RunData = Runtime{
		ConfigDir: filepath.Join(xdg.ConfigHome, AppName),
		CacheDir:  filepath.Join(xdg.CacheHome, AppName),
		DataDir:   filepath.Join(xdg.DataHome, AppName),
		BackupDir: filepath.Join(xdg.DataHome, AppName, "backups"),
	}

	// Ensure directories exist.
	// ConfigDir uses 0700 because it stores secrets (users.toml with JWT/TOTP keys).
	// CacheDir, DataDir and BackupDir use 0755 as they don't contain secrets.
	if err := os.MkdirAll(RunData.ConfigDir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", RunData.ConfigDir, err)
	}
	for _, dir := range []string{RunData.CacheDir, RunData.DataDir, RunData.BackupDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return loadConfig()
}

// loadConfig reads and parses the configuration using viper.
func loadConfig() error {
	// Set defaults
	viper.SetDefault("slackChannel", "#testing")
	viper.SetDefault("slackTasksChannel", "")
	viper.SetDefault("monitorThreshold", 3)
	viper.SetDefault("monitorTimeout", 10)
	viper.SetDefault("monitorCacheBypass", false)
	viper.SetDefault("cvssThreshold", 7.0)
	viper.SetDefault("vulnThreshold", 7.0)
	viper.SetDefault("allowedOrigins", []string{})
	viper.SetDefault("ignoreSites", []string{})
	viper.SetDefault("pluginAliases", map[string]string{})

	// Viper configuration
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(RunData.ConfigDir)

	// Environment variables support
	viper.SetEnvPrefix(strings.ToUpper(AppName))
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Explicitly bind environment variables for better control and clarity.
	// This ensures that environment variables like JMAN_SLACKTOKEN correctly map to the slackToken key.
	envBindings := map[string]string{
		"tokenSpinup":         "TOKENSPINUP",
		"slackToken":          "SLACKTOKEN",
		"slackChannel":        "SLACKCHANNEL",
		"slackMonitorChannel": "SLACKMONITORCHANNEL",
		"slackTasksChannel":   "SLACKTASKSCHANNEL",
		"monitorThreshold":    "MONITORTHRESHOLD",
		"monitorTimeout":      "MONITORTIMEOUT",
		"monitorCacheBypass":  "MONITORCACHEBYPASS",
		"cvssThreshold":       "CVSSTHRESHOLD",
		"vulnThreshold":       "VULNTHRESHOLD",
		"allowedOrigins":      "ALLOWEDORIGINS",
		"ignoreSites":         "IGNORESITES",
	}

	for key, envVar := range envBindings {
		if err := viper.BindEnv(key, fmt.Sprintf("%s_%s", strings.ToUpper(AppName), envVar)); err != nil {
			return fmt.Errorf("failed to bind env var %s for key %s: %w", envVar, key, err)
		}
	}

	// Read the config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// If file is missing, check if required environment variables are set.
			// At a minimum, TokenSpinup is usually required for the app to be useful.
			if viper.GetString("tokenSpinup") == "" {
				configPath := filepath.Join(RunData.ConfigDir, "config.toml")
				return fmt.Errorf("config file not found at %s and JMAN_TOKENSPINUP environment variable is not set", configPath)
			}
		} else {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal into the Cfg struct
	if err := viper.Unmarshal(&Cfg); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	return nil
}
