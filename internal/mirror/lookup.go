package mirror

import (
	"fmt"
	"sort"

	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
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
		out = append(out, ToCliSite(s))
	}
	return out
}

// ToCliSite returns a site's WP-CLI connection details: the --ssh target,
// path, and short server name.
func ToCliSite(s cliproto.Site) models.CliSite {
	return toManagedSite(s).ToCliSite()
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

// Users returns the WordPress users of every site, sorted by site and ID,
// without the roles that are only counted (see SiteUserSummary).
func (m *Mirror) Users() []models.SiteUser {
	if m.Snapshot == nil {
		return nil
	}
	return m.Snapshot.Users
}

// SiteUsers returns a site's WordPress users, sorted by ID.
func (m *Mirror) SiteUsers(siteID string) []models.SiteUser {
	out := []models.SiteUser{}
	for _, u := range m.Users() {
		if u.SiteID == siteID {
			out = append(out, u)
		}
	}
	return out
}

// SiteUserSummary returns a site's user summary, and false if its users
// were never read.
func (m *Mirror) SiteUserSummary(siteID string) (models.SiteUserSummary, bool) {
	if m.Snapshot == nil {
		return models.SiteUserSummary{}, false
	}
	for _, s := range m.Snapshot.UserSummaries {
		if s.SiteID == siteID {
			return s, true
		}
	}
	return models.SiteUserSummary{}, false
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

// PluginData groups the active plugins by slug, with the sites (and their
// installed versions) each is active on.
func (m *Mirror) PluginData() []models.WPPluginData {
	names := map[string]string{}
	for _, s := range m.Sites() {
		names[s.ID] = s.Domain
	}
	bySlug := map[string]*models.WPPluginData{}
	var order []string
	for _, p := range m.Plugins() {
		if p.Status != "active" {
			continue
		}
		d, ok := bySlug[p.Name]
		if !ok {
			d = &models.WPPluginData{Name: p.Name, Sites: []models.PluginSite{}}
			bySlug[p.Name] = d
			order = append(order, p.Name)
		}
		d.Sites = append(d.Sites, models.PluginSite{SiteID: p.SiteID, SiteName: names[p.SiteID], Version: p.Version})
	}
	sort.Strings(order)
	out := make([]models.WPPluginData, 0, len(order))
	for _, slug := range order {
		out = append(out, *bySlug[slug])
	}
	return out
}

// SiteName returns a site's domain, or "" if the site is unknown.
func (m *Mirror) SiteName(id string) string {
	if s, ok := m.Site(id); ok {
		return s.Domain
	}
	return ""
}

// DisplayPluginName formats a plugin for display as "Name (slug)", or just
// the slug if its name is unknown. truncate shortens long names; color
// highlights the parts.
func (m *Mirror) DisplayPluginName(slug string, truncate, color bool) string {
	name := m.PluginName(slug)
	if name == slug {
		if color {
			return verb.Yellow(slug)
		}
		return slug
	}
	name = utils.CleanHTML(name)
	if truncate {
		name = utils.ShowFirstPart(name)
	}
	if color {
		name, slug = verb.Yellow(name), verb.Cyan(slug)
	}
	return fmt.Sprintf("%s (%s)", name, slug)
}

// IsCoreLocked reports whether a site's WordPress core updates are limited
// to minor releases, which only a lock on the whole site does.
func (m *Mirror) IsCoreLocked(siteID string) bool {
	s, ok := m.Site(siteID)
	if !ok {
		return false
	}
	for _, l := range s.UpdateLocks {
		if l.Plugin == "" {
			return true
		}
	}
	return false
}
