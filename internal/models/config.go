package models

// Config represents the user configuration typically stored in a TOML file.
type Config struct {
	UrlMainwp     string   `toml:"urlMainwp" json:"urlMainwp"`
	TokenSpinup   string   `toml:"tokenSpinup" json:"tokenSpinup"`
	TokenMainwp   string   `toml:"tokenMainwp" json:"tokenMainwp"`
	SlackToken    string   `toml:"slackToken" json:"slackToken"`
	SlackChannel  string   `toml:"slackChannel" json:"slackChannel"`
	CvssThreshold float64  `toml:"cvssThreshold" json:"cvssThreshold"`
	VulnThreshold float64  `toml:"vulnThreshold" json:"vulnThreshold"`
	IgnoreSites   []string `toml:"ignoreSites" json:"ignoreSites"`
}

// Runtime contains runtime-specific paths and information.
type Runtime struct {
	ConfigDir  string
	CacheDir   string
	DataDir    string
	Version    string
	NodePath   string // Retained from TS, though less relevant in Go
	ScriptPath string // Retained from TS, though less relevant in Go
	ExecPath   string
}

// DefaultConfig returns a Config initialized with default values.
func DefaultConfig() Config {
	return Config{
		SlackChannel:  "#testing",
		CvssThreshold: 7.0,
		VulnThreshold: 7.0,
		IgnoreSites:   []string{},
	}
}
