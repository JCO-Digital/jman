package mirror

import (
	"sort"

	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/models"
)

// The lookups below replace the cache.Get* functions the commands use.
// They all work on a loaded Mirror and never contact the API. A Mirror
// without a snapshot behaves as empty.

// Sites returns every site, sorted by domain.
func (m *Mirror) Sites() []cliproto.Site {
	if m.Snapshot == nil {
		return nil
	}
	return m.Snapshot.Sites
}

// Site returns the site with the given ID, if there is one.
func (m *Mirror) Site(id string) (cliproto.Site, bool) {
	for _, s := range m.Sites() {
		if s.ID == id {
			return s, true
		}
	}
	return cliproto.Site{}, false
}

// CliSites returns every WordPress site WP-CLI can reach over SSH, sorted
// by domain, in the form the WP-CLI helpers take.
func (m *Mirror) CliSites() []models.CliSite {
	out := []models.CliSite{}
	for _, s := range m.Sites() {
		if !s.IsWordpress || !s.CanWPCLI || s.SSHHost == "" {
			continue
		}
		out = append(out, toManagedSite(s).ToCliSite())
	}
	return out
}

// Servers returns every server, sorted by name.
func (m *Mirror) Servers() []cliproto.Server {
	if m.Snapshot == nil {
		return nil
	}
	return m.Snapshot.Servers
}

// Plugins returns every installed plugin on every site, of any status.
func (m *Mirror) Plugins() []models.WPPlugin {
	if m.Snapshot == nil {
		return nil
	}
	return m.Snapshot.Plugins
}

// SitePlugins returns the plugins installed on a site, sorted by slug.
func (m *Mirror) SitePlugins(siteID string) []models.WPPlugin {
	out := []models.WPPlugin{}
	for _, p := range m.Plugins() {
		if p.SiteID == siteID {
			out = append(out, p)
		}
	}
	return out
}

// CoreVersions returns the WordPress core version of every site that has
// one recorded.
func (m *Mirror) CoreVersions() []models.SiteCore {
	out := []models.SiteCore{}
	for _, s := range m.Sites() {
		if s.WPCore != nil {
			out = append(out, *s.WPCore)
		}
	}
	return out
}

// PluginName returns a plugin's display name, or its slug if unknown.
func (m *Mirror) PluginName(slug string) string {
	if info, ok := m.PluginInfo(slug); ok && info.Name != "" {
		return info.Name
	}
	return slug
}

// PluginInfo returns what is known about a plugin by slug.
func (m *Mirror) PluginInfo(slug string) (cliproto.PluginInfo, bool) {
	if m.Snapshot == nil {
		return cliproto.PluginInfo{}, false
	}
	infos := m.Snapshot.PluginInfo
	i := sort.Search(len(infos), func(i int) bool { return infos[i].Slug >= slug })
	if i < len(infos) && infos[i].Slug == slug {
		return infos[i], true
	}
	return cliproto.PluginInfo{}, false
}

// IsUpdateLocked reports whether updates of plugin on a site are limited
// to fix releases, by a lock on the plugin or on the whole site.
func (m *Mirror) IsUpdateLocked(siteID, plugin string) bool {
	s, ok := m.Site(siteID)
	if !ok {
		return false
	}
	for _, l := range s.UpdateLocks {
		if l.Plugin == "" || l.Plugin == plugin {
			return true
		}
	}
	return false
}

// toManagedSite converts a snapshot site to the inventory model, whose
// ToCliSite builds the WP-CLI connection details.
func toManagedSite(s cliproto.Site) models.ManagedSite {
	ms := models.ManagedSite{
		ID:             s.ID,
		ServerName:     s.ServerName,
		Provider:       s.Provider,
		Domain:         s.Domain,
		Environment:    s.Environment,
		IsWordpress:    s.IsWordpress,
		ConnectionType: s.ConnectionType,
		SSHHost:        s.SSHHost,
		SSHPort:        s.SSHPort,
		SSHUser:        s.SSHUser,
		SitePath:       s.SitePath,
		CanWPCLI:       s.CanWPCLI,
		Status:         s.Status,
		WPCore:         s.WPCore,
	}
	if s.ServerID != "" {
		id := s.ServerID
		ms.ServerID = &id
	}
	return ms
}
