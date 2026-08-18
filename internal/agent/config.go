package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	toml "github.com/pelletier/go-toml/v2"
)

// Config holds jman-agent's runtime configuration, loaded from a single
// TOML file rather than jman's XDG-layered config system — jman-agent runs
// standalone on managed servers with no jman installation alongside it.
type Config struct {
	APIURL                       string `toml:"apiUrl"`
	Token                        string `toml:"token"`
	ReportIntervalMinutes        int    `toml:"reportIntervalMinutes"`
	SelfUpdateEnabled            bool   `toml:"selfUpdateEnabled"`
	SelfUpdateCheckIntervalHours int    `toml:"selfUpdateCheckIntervalHours"`
	// StateDir holds local, never-transmitted log-tailing state (byte
	// offsets, processed-rotation markers, in-progress hourly
	// aggregates) keyed per site. Defaults to DefaultStateDir().
	StateDir string `toml:"stateDir"`
}

// DefaultConfigPath returns /etc/jman-agent/config.toml when running as
// root (the expected systemd deployment), or an XDG-config-relative path
// otherwise (for local development/testing).
func DefaultConfigPath() string {
	if os.Geteuid() == 0 {
		return "/etc/jman-agent/config.toml"
	}
	return filepath.Join(xdg.ConfigHome, "jman-agent", "config.toml")
}

// DefaultStateDir returns the directory jman-agent uses to persist its
// local per-site log-tailing state. Root deployments (the expected systemd
// setup) use /var/lib/jman-agent; otherwise it falls back to the XDG state
// directory (for local development/testing).
func DefaultStateDir() string {
	if os.Geteuid() == 0 {
		return "/var/lib/jman-agent"
	}
	return filepath.Join(xdg.StateHome, "jman-agent")
}

// LoadConfig reads and validates the agent config file at path. The token
// and API URL may also be supplied via JMAN_AGENT_TOKEN/JMAN_AGENT_API_URL
// environment variables, which take precedence over the file.
func LoadConfig(path string) (Config, error) {
	cfg := Config{
		ReportIntervalMinutes:        15,
		SelfUpdateEnabled:            true,
		SelfUpdateCheckIntervalHours: 24,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// This file holds the agent's API token — refuse to run if it's
	// readable by group or others, mirroring jman's own config/users.toml checks.
	if info, statErr := os.Stat(path); statErr == nil {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return Config{}, fmt.Errorf(
				"%s has permissions %04o, which are too open. It contains the agent's API token "+
					"and must not be accessible by group or others. Fix with: chmod 600 %s",
				path, perm, path)
		}
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	if token := os.Getenv("JMAN_AGENT_TOKEN"); token != "" {
		cfg.Token = token
	}
	if apiURL := os.Getenv("JMAN_AGENT_API_URL"); apiURL != "" {
		cfg.APIURL = apiURL
	}

	if cfg.APIURL == "" {
		return Config{}, fmt.Errorf("apiUrl must be set in %s or JMAN_AGENT_API_URL", path)
	}
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("token must be set in %s or JMAN_AGENT_TOKEN", path)
	}
	cfg.APIURL = strings.TrimSuffix(cfg.APIURL, "/")

	if cfg.ReportIntervalMinutes <= 0 {
		cfg.ReportIntervalMinutes = 15
	}
	if cfg.SelfUpdateCheckIntervalHours <= 0 {
		cfg.SelfUpdateCheckIntervalHours = 24
	}
	if cfg.StateDir == "" {
		cfg.StateDir = DefaultStateDir()
	}

	return cfg, nil
}
