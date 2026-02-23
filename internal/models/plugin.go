package models

// WPPlugin represents a WordPress plugin installed on a specific site.
type WPPlugin struct {
	SiteID     int    `json:"site_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Version    string `json:"version"`
	Update     string `json:"update"`
	AutoUpdate bool   `json:"autoUpdate"`
}

// PluginSite represents a specific site where a plugin is installed and its version.
type PluginSite struct {
	SiteID  int    `json:"site_id"`
	Version string `json:"version"`
}

// WPPluginData groups sites by a specific plugin.
type WPPluginData struct {
	Name  string       `json:"name"`
	Sites []PluginSite `json:"sites"`
}
