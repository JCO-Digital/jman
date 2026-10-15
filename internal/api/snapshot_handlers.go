package api

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/cliproto"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/refresh"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
)

// buildCLISnapshot assembles the snapshot. Every list is sorted so that the
// same data always serializes to the same bytes.
func buildCLISnapshot() (*cliproto.Snapshot, error) {
	managedServers, err := db.ListManagedServers()
	if err != nil {
		return nil, fmt.Errorf("servers: %w", err)
	}
	sites, err := loadAPISites()
	if err != nil {
		return nil, fmt.Errorf("sites: %w", err)
	}
	plugins, err := db.GetAllSitePlugins()
	if err != nil {
		return nil, fmt.Errorf("plugins: %w", err)
	}
	infos, err := db.GetAllPluginInfo()
	if err != nil {
		return nil, fmt.Errorf("plugin info: %w", err)
	}

	snap := &cliproto.Snapshot{
		SchemaVersion: cliproto.SnapshotSchemaVersion,
		Servers:       []cliproto.Server{},
		Sites:         []cliproto.Site{},
		Plugins:       []models.WPPlugin{},
		PluginInfo:    []cliproto.PluginInfo{},
	}

	for _, s := range newAPIServers(managedServers) {
		snap.Servers = append(snap.Servers, cliproto.Server{
			ID:        s.ID,
			Provider:  s.Provider,
			Name:      s.Name,
			IsLogical: s.IsLogical,
			IPAddress: s.IPAddress,
			SSHPort:   s.SSHPort,
		})
	}

	included := map[string]bool{}
	for _, s := range overrideSpinupWPSites(dropDeletedSpinupWPSites(sites)) {
		included[s.ID] = true
		site := cliproto.Site{
			ID:             s.ID,
			Provider:       s.Provider,
			Domain:         s.Domain,
			ServerName:     s.ServerName,
			Environment:    s.Environment,
			Status:         s.Status,
			IsWordpress:    s.IsWordpress,
			ConnectionType: s.ConnectionType,
			SSHHost:        s.SSHHost,
			SSHPort:        s.SSHPort,
			SSHUser:        s.SiteUser,
			SSH:            s.SSH,
			SitePath:       s.SitePath,
			CanWPCLI:       s.CanWPCLI,
			WPCore:         s.WPCore,
			UpdateLocks:    s.UpdateLocks,
		}
		if s.ServerID != nil {
			site.ServerID = *s.ServerID
		}
		sort.Slice(site.UpdateLocks, func(i, j int) bool { return site.UpdateLocks[i].ID < site.UpdateLocks[j].ID })
		snap.Sites = append(snap.Sites, site)
	}

	for _, p := range plugins {
		if included[p.SiteID] {
			snap.Plugins = append(snap.Plugins, p)
		}
	}
	sort.Slice(snap.Plugins, func(i, j int) bool {
		if snap.Plugins[i].SiteID != snap.Plugins[j].SiteID {
			return snap.Plugins[i].SiteID < snap.Plugins[j].SiteID
		}
		return snap.Plugins[i].Name < snap.Plugins[j].Name
	})

	for _, info := range infos {
		snap.PluginInfo = append(snap.PluginInfo, cliproto.PluginInfo{Slug: info.Slug, Name: info.Name, Latest: info.Version})
	}
	sort.Slice(snap.PluginInfo, func(i, j int) bool { return snap.PluginInfo[i].Slug < snap.PluginInfo[j].Slug })

	return snap, nil
}

// dropDeletedSpinupWPSites drops SpinupWP sites that are no longer in the
// cached SpinupWP site list. The SpinupWP sync only adds and updates
// inventory rows, so the inventory can still hold sites deleted in
// SpinupWP; the cached list is the source of truth for which exist. Without
// a cached list (SpinupWP not configured), the inventory is used as is.
func dropDeletedSpinupWPSites(sites []apiSite) []apiSite {
	cached := []models.Site{}
	if err := cache.ReadJSONCache("sites", &cached, -1); err != nil {
		return sites
	}
	current := make(map[string]bool, len(cached))
	for _, c := range cached {
		current[utils.SpinupWPSiteUUID(c.ID)] = true
	}
	out := make([]apiSite, 0, len(sites))
	for _, s := range sites {
		if s.Provider == "spinupwp" && !current[s.ID] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// overrideSpinupWPSites drops SpinupWP sites whose domain also belongs to a
// site from another provider. A manually added site for a SpinupWP domain is
// an intentional override, so the CLI should only ever see that one.
func overrideSpinupWPSites(sites []apiSite) []apiSite {
	overridden := map[string]bool{}
	for _, s := range sites {
		if s.Provider != "spinupwp" {
			overridden[strings.ToLower(s.Domain)] = true
		}
	}
	out := make([]apiSite, 0, len(sites))
	for _, s := range sites {
		if s.Provider == "spinupwp" && overridden[strings.ToLower(s.Domain)] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// CLISnapshotHandler returns the CLI snapshot. It supports If-None-Match
// (304 when unchanged) and gzip.
//
//	GET /api/cli/snapshot
func CLISnapshotHandler(w http.ResponseWriter, r *http.Request) {
	snap, err := buildCLISnapshot()
	if err != nil {
		verb.LogPrintf(verb.Normal, "CLISnapshotHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	body, err := json.Marshal(snap)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CLISnapshotHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Add("Vary", "Accept-Encoding")
	if t := refresh.LastRefresh(); !t.IsZero() {
		w.Header().Set(cliproto.RefreshedAtHeader, t.Format(time.RFC3339))
	}

	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(body); err == nil && gz.Close() == nil {
			w.Header().Set("Content-Encoding", "gzip")
			body = buf.Bytes()
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// etagMatches reports whether an If-None-Match header value names etag.
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == etag || candidate == "*" {
			return true
		}
	}
	return false
}
