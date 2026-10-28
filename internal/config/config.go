package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	TokenSpinup         string `toml:"tokenSpinup" mapstructure:"tokenSpinup"`
	TokenSlack          string `toml:"slackToken" mapstructure:"slackToken"`
	SlackChannel        string `toml:"slackChannel" mapstructure:"slackChannel"`
	SlackMonitorChannel string `toml:"slackMonitorChannel" mapstructure:"slackMonitorChannel"`
	SlackTasksChannel   string `toml:"slackTasksChannel" mapstructure:"slackTasksChannel"`
	// TokenPagerDuty is the PagerDuty Events API v2 routing (integration) key.
	// Leave unset to disable PagerDuty alerting entirely.
	TokenPagerDuty string `toml:"pagerdutyRoutingKey" mapstructure:"pagerdutyRoutingKey"`
	// PagerDutyEscalationMinutes is how long a site must stay down before its
	// PagerDuty alert is escalated from warning to critical severity.
	PagerDutyEscalationMinutes int `toml:"pagerdutyEscalationMinutes" mapstructure:"pagerdutyEscalationMinutes"`
	// PagerDutyEURegion selects PagerDuty's EU Events API v2 endpoint
	// (events.eu.pagerduty.com) instead of the default US endpoint. Set this
	// to true if your PagerDuty account's URL contains "eu" (e.g.
	// yourcompany.eu.pagerduty.com).
	PagerDutyEURegion  bool `toml:"pagerdutyEuRegion" mapstructure:"pagerdutyEuRegion"`
	MonitorThreshold   int  `toml:"monitorThreshold" mapstructure:"monitorThreshold"`
	MonitorTimeout     int  `toml:"monitorTimeout" mapstructure:"monitorTimeout"`
	MonitorCacheBypass bool `toml:"monitorCacheBypass" mapstructure:"monitorCacheBypass"`
	// MonitorDisabled turns off jman-api's in-process uptime-monitor scheduler.
	MonitorDisabled bool `toml:"monitorDisabled" mapstructure:"monitorDisabled"`
	// RefreshDisabled turns off jman-api's in-process data-refresh scheduler
	// (the in-process replacement for the external `jman fetch` cron job).
	RefreshDisabled bool `toml:"refreshDisabled" mapstructure:"refreshDisabled"`
	// RefreshFastInterval/RefreshSlowInterval are the tick intervals, in
	// minutes, for the refresh scheduler's cheap (servers/sites) and
	// expensive (plugins/vulnerabilities/core versions) work respectively.
	RefreshFastInterval int               `toml:"refreshFastInterval" mapstructure:"refreshFastInterval"`
	RefreshSlowInterval int               `toml:"refreshSlowInterval" mapstructure:"refreshSlowInterval"`
	CVSSThreshold       float64           `toml:"cvssThreshold" mapstructure:"cvssThreshold"`
	VulnThreshold       float64           `toml:"vulnThreshold" mapstructure:"vulnThreshold"`
	VulnReportTime      string            `toml:"vulnReportTime" mapstructure:"vulnReportTime"` // "HH:MM" local time of the daily per-site Slack vuln report
	KnockdPorts         string            `toml:"knockdPorts" mapstructure:"knockdPorts"`
	KnockdTimeout       int               `toml:"knockdTimeout" mapstructure:"knockdTimeout"`
	BehindProxy         bool              `toml:"behindProxy" mapstructure:"behindProxy"`
	TrustedProxies      []string          `toml:"trustedProxies" mapstructure:"trustedProxies"`
	AllowedOrigins      []string          `toml:"allowedOrigins" mapstructure:"allowedOrigins"`
	IgnoreSites         []string          `toml:"ignoreSites" mapstructure:"ignoreSites"`
	PluginAliases       map[string]string `toml:"pluginAliases" mapstructure:"pluginAliases"`

	// AgentWPDataInterval is how often, in minutes, jman-agent collects
	// each site's plugins and core version (default 60; negative turns
	// agent collection off, leaving those sites to the SSH refresh).
	AgentWPDataInterval int `toml:"agentWpDataInterval" mapstructure:"agentWpDataInterval"`
	// AgentScanInterval is how often, in minutes, jman-agent scans each
	// site's files for signs of compromise (default 60; negative turns
	// scanning off).
	AgentScanInterval int `toml:"agentScanInterval" mapstructure:"agentScanInterval"`
	// AgentContentScanInterval is how often, in minutes, a site's scan
	// searches all its PHP files for malicious code (default 1440, a day,
	// run at night; negative turns it off).
	AgentContentScanInterval int `toml:"agentContentScanInterval" mapstructure:"agentContentScanInterval"`
	// ScanSlackSeverity and ScanPagerDutySeverity are the lowest severity of
	// a scan finding that alerts in Slack (default "high") and PagerDuty
	// (default "critical"); "off" turns that alert off. Findings below both
	// only show in the web UI. Slack alerts go to SlackSecurityChannel, or
	// slackMonitorChannel when it's unset.
	ScanSlackSeverity     string `toml:"scanSlackSeverity" mapstructure:"scanSlackSeverity"`
	ScanPagerDutySeverity string `toml:"scanPagerDutySeverity" mapstructure:"scanPagerDutySeverity"`
	SlackSecurityChannel  string `toml:"slackSecurityChannel" mapstructure:"slackSecurityChannel"`
	// TrustedAdminDomains are email domains (and their subdomains) whose
	// administrators, when they appear on a site without jman creating
	// them, are medium rather than high: an incident, but no Slack alert.
	// An email is easy to fake, so the finding is still raised.
	TrustedAdminDomains []string `toml:"trustedAdminDomains" mapstructure:"trustedAdminDomains"`
	// ScanSnippetRetentionDays is how long a scan finding keeps its snippet
	// and detail after it was closed, allowlisted or acknowledged (default
	// 90; negative keeps them). They quote the file, which can hold
	// personal data; the rule, path and hash stay.
	ScanSnippetRetentionDays int `toml:"scanSnippetRetentionDays" mapstructure:"scanSnippetRetentionDays"`
	// AgentServerStaleMinutes and AgentSiteStaleMinutes are how long a
	// server's agent may go without calling in (default 30), and a site
	// without a successful WordPress data collection (default 180),
	// before jman-api warns in Slack.
	AgentServerStaleMinutes int `toml:"agentServerStaleMinutes" mapstructure:"agentServerStaleMinutes"`
	AgentSiteStaleMinutes   int `toml:"agentSiteStaleMinutes" mapstructure:"agentSiteStaleMinutes"`
	// WPCLIHostTimeouts raises the WP-CLI timeout for slow SSH hosts, such
	// as WP Engine's SSH gateway. See WPCLIHostTimeout.
	WPCLIHostTimeouts []WPCLIHostTimeout `toml:"wpcliHostTimeouts" mapstructure:"wpcliHostTimeouts"`

	// WebURL is the web UI's base URL, e.g. "https://jman.example.com".
	// jman-api reports it in GET /api/health so the CLI can open the
	// browser login with only the API URL configured.
	WebURL string `toml:"webUrl" mapstructure:"webUrl"`
	// MinCLIVersion is the oldest jman CLI version jman-api serves. Older
	// CLIs refuse to run until updated. Empty means no minimum.
	MinCLIVersion string `toml:"minCliVersion" mapstructure:"minCliVersion"`
	// LatestCLIVersion is the newest jman CLI release; older CLIs print an
	// update notice. Defaults to jman-api's own version.
	LatestCLIVersion string `toml:"latestCliVersion" mapstructure:"latestCliVersion"`

	// APIURL is the jman-api base URL the CLI uses, e.g.
	// "https://jman.example.com". `jman login <url>` sets it.
	APIURL string `toml:"apiUrl" mapstructure:"apiUrl"`
	// CacheTTL is how old the CLI's local copy of the API data can get
	// before a command checks the API for changes.
	CacheTTL time.Duration `toml:"cacheTTL" mapstructure:"cacheTTL"`
	// APITimeout bounds that check, so commands don't hang when the API is
	// unreachable.
	APITimeout time.Duration `toml:"apiTimeout" mapstructure:"apiTimeout"`
	// OfflineBackoff is how long the CLI waits after a failed check before
	// trying the API again, working from its local copy meanwhile.
	OfflineBackoff time.Duration `toml:"offlineBackoff" mapstructure:"offlineBackoff"`
	// StaleWarning is how old the local copy can get before the CLI warns
	// that it is working from outdated data.
	StaleWarning time.Duration `toml:"staleWarning" mapstructure:"staleWarning"`
}

// WPCLIHostTimeout sets a minimum WP-CLI timeout, in minutes, for every SSH
// host matching Host, a shell glob such as "*.ssh.wpengine.net". It applies
// to every WP-CLI call made against that host, raising (never lowering) the
// call's own timeout. Configured as an array of tables:
//
//	[[wpcliHostTimeouts]]
//	host = "*.ssh.wpengine.net"
//	minutes = 5
type WPCLIHostTimeout struct {
	Host    string `toml:"host" mapstructure:"host"`
	Minutes int    `toml:"minutes" mapstructure:"minutes"`
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
	viper.SetDefault("pagerdutyEscalationMinutes", 10)
	viper.SetDefault("pagerdutyEuRegion", false)
	viper.SetDefault("monitorThreshold", 3)
	viper.SetDefault("monitorTimeout", 10)
	viper.SetDefault("monitorCacheBypass", false)
	viper.SetDefault("monitorDisabled", false)
	viper.SetDefault("refreshDisabled", false)
	viper.SetDefault("refreshFastInterval", 5)
	viper.SetDefault("refreshSlowInterval", 30)
	viper.SetDefault("cvssThreshold", 7.0)
	viper.SetDefault("vulnThreshold", 7.0)
	viper.SetDefault("vulnReportTime", "10:00")
	viper.SetDefault("knockdPorts", "")
	viper.SetDefault("knockdTimeout", 60)
	// Default to trusting proxy headers only from loopback, the common case
	// of a reverse proxy (e.g. nginx) running on the same host as jman-api.
	// Deployments with a proxy elsewhere on the network must set this explicitly.
	viper.SetDefault("trustedProxies", []string{"127.0.0.1/32", "::1/128"})
	viper.SetDefault("allowedOrigins", []string{})
	viper.SetDefault("ignoreSites", []string{})
	viper.SetDefault("pluginAliases", map[string]string{})
	viper.SetDefault("cacheTTL", "2m")
	viper.SetDefault("apiTimeout", "3s")
	viper.SetDefault("offlineBackoff", "15m")
	viper.SetDefault("staleWarning", "168h")

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
		"tokenSpinup":                "TOKENSPINUP",
		"slackToken":                 "SLACKTOKEN",
		"slackChannel":               "SLACKCHANNEL",
		"slackMonitorChannel":        "SLACKMONITORCHANNEL",
		"slackTasksChannel":          "SLACKTASKSCHANNEL",
		"slackSecurityChannel":       "SLACKSECURITYCHANNEL",
		"scanSlackSeverity":          "SCANSLACKSEVERITY",
		"agentScanInterval":          "AGENTSCANINTERVAL",
		"agentContentScanInterval":   "AGENTCONTENTSCANINTERVAL",
		"scanSnippetRetentionDays":   "SCANSNIPPETRETENTIONDAYS",
		"scanPagerDutySeverity":      "SCANPAGERDUTYSEVERITY",
		"pagerdutyRoutingKey":        "PAGERDUTYROUTINGKEY",
		"pagerdutyEscalationMinutes": "PAGERDUTYESCALATIONMINUTES",
		"pagerdutyEuRegion":          "PAGERDUTYEUREGION",
		"monitorThreshold":           "MONITORTHRESHOLD",
		"monitorTimeout":             "MONITORTIMEOUT",
		"monitorCacheBypass":         "MONITORCACHEBYPASS",
		"monitorDisabled":            "MONITORDISABLED",
		"refreshDisabled":            "REFRESHDISABLED",
		"refreshFastInterval":        "REFRESHFASTINTERVAL",
		"refreshSlowInterval":        "REFRESHSLOWINTERVAL",
		"cvssThreshold":              "CVSSTHRESHOLD",
		"vulnThreshold":              "VULNTHRESHOLD",
		"vulnReportTime":             "VULNREPORTTIME",
		"knockdPorts":                "KNOCKDPORTS",
		"knockdTimeout":              "KNOCKDTIMEOUT",
		"allowedOrigins":             "ALLOWEDORIGINS",
		"trustedProxies":             "TRUSTEDPROXIES",
		"ignoreSites":                "IGNORESITES",
		"webUrl":                     "WEBURL",
		"minCliVersion":              "MINCLIVERSION",
		"latestCliVersion":           "LATESTCLIVERSION",
		"apiUrl":                     "APIURL",
	}

	for key, envVar := range envBindings {
		if err := viper.BindEnv(key, fmt.Sprintf("%s_%s", strings.ToUpper(AppName), envVar)); err != nil {
			return fmt.Errorf("failed to bind env var %s for key %s: %w", envVar, key, err)
		}
	}

	// Read the config file if present. If missing, defaults and environment variables apply.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		// Enforce strict file permissions on config.toml. This file can hold
		// the SpinupWP API token and Slack bot token; refuse to start if it's
		// readable by group or others, mirroring the check on users.toml.
		configPath := viper.ConfigFileUsed()
		info, statErr := os.Stat(configPath)
		if statErr == nil {
			if perm := info.Mode().Perm(); perm&0o077 != 0 {
				return fmt.Errorf(
					"%s has permissions %04o, which are too open. "+
						"This file may contain secrets and must not be accessible by group or others. "+
						"Fix with: chmod 600 %s",
					configPath, perm, configPath)
			}
		}
	}

	// Unmarshal into the Cfg struct
	if err := viper.Unmarshal(&Cfg); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	return nil
}

// AgentWPDataIntervalMinutes returns the agent's WordPress data collection
// interval, or 0 if agent collection is turned off.
func AgentWPDataIntervalMinutes() int {
	switch {
	case Cfg.AgentWPDataInterval < 0:
		return 0
	case Cfg.AgentWPDataInterval == 0:
		return 60
	}
	return Cfg.AgentWPDataInterval
}

// AgentScanIntervalMinutes returns the agent's file scan interval, or 0 if
// scanning is turned off.
func AgentScanIntervalMinutes() int {
	switch {
	case Cfg.AgentScanInterval < 0:
		return 0
	case Cfg.AgentScanInterval == 0:
		return 60
	}
	return Cfg.AgentScanInterval
}

// AgentContentScanIntervalMinutes returns how often a site's PHP files are
// all searched for malicious code, or 0 if that's turned off (as it is
// with scanning).
func AgentContentScanIntervalMinutes() int {
	switch {
	case AgentScanIntervalMinutes() == 0 || Cfg.AgentContentScanInterval < 0:
		return 0
	case Cfg.AgentContentScanInterval == 0:
		return 1440
	}
	return Cfg.AgentContentScanInterval
}

// ScanSnippetRetention returns how long a triaged scan finding keeps its
// snippet and detail, or 0 if they're kept.
func ScanSnippetRetention() time.Duration {
	switch {
	case Cfg.ScanSnippetRetentionDays < 0:
		return 0
	case Cfg.ScanSnippetRetentionDays == 0:
		return 90 * 24 * time.Hour
	}
	return time.Duration(Cfg.ScanSnippetRetentionDays) * 24 * time.Hour
}

// ScanSeverityRank orders scan finding severities, from 1 for low to 4 for
// critical. Anything else is 0.
func ScanSeverityRank(severity string) int {
	switch severity {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	}
	return 0
}

// ScanSlackAlertRank and ScanPagerDutyAlertRank return the ScanSeverityRank
// a scan finding needs for a Slack or PagerDuty alert, or 0 when that alert
// is off.
func ScanSlackAlertRank() int {
	return scanAlertRank(Cfg.ScanSlackSeverity, "high")
}

func ScanPagerDutyAlertRank() int {
	return scanAlertRank(Cfg.ScanPagerDutySeverity, "critical")
}

func scanAlertRank(setting, def string) int {
	if setting == "" {
		setting = def
	}
	return ScanSeverityRank(setting)
}

// AgentServerStaleAfter returns how long a server's agent may stay silent
// before jman-api warns.
func AgentServerStaleAfter() time.Duration {
	if Cfg.AgentServerStaleMinutes <= 0 {
		return 30 * time.Minute
	}
	return time.Duration(Cfg.AgentServerStaleMinutes) * time.Minute
}

// AgentSiteStaleAfter returns how long a site may go without a successful
// agent collection of its WordPress data before jman-api warns.
func AgentSiteStaleAfter() time.Duration {
	if Cfg.AgentSiteStaleMinutes <= 0 {
		return 180 * time.Minute
	}
	return time.Duration(Cfg.AgentSiteStaleMinutes) * time.Minute
}
