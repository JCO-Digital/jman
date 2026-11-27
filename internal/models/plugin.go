package models

import "github.com/JCO-Digital/jman/internal/utils"

// WPPlugin represents a WordPress plugin installed on a specific site.
type WPPlugin struct {
	SiteID     string `json:"site_id"` // site UUID
	Name       string `json:"name"`
	Status     string `json:"status"`
	Version    string `json:"version"`
	Update     string `json:"update"`
	AutoUpdate bool   `json:"autoUpdate"`
	// UpdateBlocked is why the site can't install Update, as wp-cli words
	// it (e.g. "This update requires WordPress version 7.0, but the
	// version installed is 6.9.12."), or "" when it can. wp-cli refuses
	// such an update, so update jobs skip the plugin.
	UpdateBlocked string `json:"update_blocked,omitempty"`
}

// PluginSite represents a specific site where a plugin is installed and its version.
type PluginSite struct {
	SiteID     string `json:"site_id"` // site UUID
	SiteName   string `json:"site_name,omitempty"`
	Version    string `json:"version"`
	Suppressed bool   `json:"suppressed"`
}

// WPPluginData groups sites by a specific plugin.
type WPPluginData struct {
	Name  string       `json:"name"`
	Sites []PluginSite `json:"sites"`
}

// PluginInfo holds metadata fetched from the WordPress.org plugin API.
type PluginInfo struct {
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Version       string `json:"version"`
	Author        string `json:"author"`
	AuthorProfile string `json:"author_profile"`
	Requires      string `json:"requires"`
	Tested        string `json:"tested"`
	LastUpdated   string `json:"last_updated"`
	Homepage      string `json:"homepage"`
}

// SanitizePluginInfo normalizes fields in PluginInfo.
func SanitizePluginInfo(info *PluginInfo) {
	if info == nil {
		return
	}

	info.Name = utils.CleanHTML(info.Name)
	info.Author = utils.CleanHTML(info.Author)
}

// PluginDetails is a plugin as `wp plugin list` describes it.
type PluginDetails struct {
	Slug          string `json:"name"`
	Title         string `json:"title"`
	Version       string `json:"version"`
	Status        string `json:"status"`
	Author        string `json:"author"`
	UpdateVersion string `json:"update_version"`
}
