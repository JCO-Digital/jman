package models

import "time"

// UpdateLock restricts updates on a site to fix releases. A lock with an
// empty Plugin covers the whole site (WordPress core and every plugin);
// otherwise it covers that one plugin. Locked items only get patch updates
// (e.g. 9.3.0 → 9.3.2, or 6.6.1 → 6.6.2 for core) from bulk and ordinary
// updates; a bigger update needs an explicit, confirmed major update, and
// the lock stays in place afterwards.
type UpdateLock struct {
	ID        int64     `json:"id"`
	SiteID    string    `json:"site_id"` // site UUID
	Plugin    string    `json:"plugin"`  // plugin slug, or "" for a site lock
	Comment   string    `json:"comment"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// SiteLocks holds the update locks of one site.
type SiteLocks struct {
	// Site is the site-wide lock, if any.
	Site *UpdateLock
	// Plugins holds the plugin locks by plugin slug.
	Plugins map[string]UpdateLock
}

// PluginLocked reports whether a plugin is locked, by a lock of its own or
// by the site lock.
func (l SiteLocks) PluginLocked(plugin string) bool {
	if l.Site != nil {
		return true
	}
	_, ok := l.Plugins[plugin]
	return ok
}

// CoreLocked reports whether WordPress core is locked, which only the site
// lock does.
func (l SiteLocks) CoreLocked() bool {
	return l.Site != nil
}
