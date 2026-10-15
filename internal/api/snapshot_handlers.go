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

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/refresh"
	"github.com/JCO-Digital/jman/internal/verb"
)

// SnapshotSchemaVersion is the format version of the CLI snapshot. Bump it
// when a change would break an older CLI's reading of the snapshot.
const SnapshotSchemaVersion = 1

// RefreshedAtHeader carries when servers and sites were last refreshed from
// SpinupWP. It is a header rather than part of the snapshot so that the
// snapshot, and so its ETag, only changes when the data does.
const RefreshedAtHeader = "X-Jman-Refreshed-At"

// cliSnapshot is everything the jman CLI mirrors locally. It is the same for
// every user, so one ETag covers everyone.
type cliSnapshot struct {
	SchemaVersion int                  `json:"schema_version"`
	Servers       []snapshotServer     `json:"servers"`
	Sites         []snapshotSite       `json:"sites"`
	Plugins       []models.WPPlugin    `json:"plugins"`
	PluginInfo    []snapshotPluginInfo `json:"plugin_info"`
}

type snapshotServer struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	IsLogical bool   `json:"is_logical"`
	IPAddress string `json:"ip_address"`
	SSHPort   int    `json:"ssh_port"`
}

type snapshotSite struct {
	ID             string                     `json:"id"`
	Provider       string                     `json:"provider"`
	Domain         string                     `json:"domain"`
	ServerID       string                     `json:"server_id,omitempty"`
	ServerName     string                     `json:"server_name"`
	Environment    models.SiteEnvironmentType `json:"environment,omitempty"`
	Status         string                     `json:"status"`
	IsWordpress    bool                       `json:"is_wordpress"`
	ConnectionType string                     `json:"connection_type"`
	SSHHost        string                     `json:"ssh_host"`
	SSHPort        int                        `json:"ssh_port"`
	SSHUser        string                     `json:"ssh_user"`
	// SSH is the WP-CLI --ssh target ("user@host" or "user@host:port").
	SSH         string              `json:"ssh,omitempty"`
	SitePath    string              `json:"site_path"`
	CanWPCLI    bool                `json:"can_wp_cli"`
	WPCore      *models.SiteCore    `json:"wp_core,omitempty"`
	UpdateLocks []models.UpdateLock `json:"update_locks,omitempty"`
}

type snapshotPluginInfo struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Latest string `json:"latest,omitempty"`
}

// buildCLISnapshot assembles the snapshot. Every list is sorted so that the
// same data always serializes to the same bytes.
func buildCLISnapshot() (*cliSnapshot, error) {
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

	snap := &cliSnapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Servers:       []snapshotServer{},
		Sites:         []snapshotSite{},
		Plugins:       []models.WPPlugin{},
		PluginInfo:    []snapshotPluginInfo{},
	}

	for _, s := range newAPIServers(managedServers) {
		snap.Servers = append(snap.Servers, snapshotServer{
			ID:        s.ID,
			Provider:  s.Provider,
			Name:      s.Name,
			IsLogical: s.IsLogical,
			IPAddress: s.IPAddress,
			SSHPort:   s.SSHPort,
		})
	}

	included := map[string]bool{}
	for _, s := range overrideSpinupWPSites(sites) {
		included[s.ID] = true
		site := snapshotSite{
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
		snap.PluginInfo = append(snap.PluginInfo, snapshotPluginInfo{Slug: info.Slug, Name: info.Name, Latest: info.Version})
	}
	sort.Slice(snap.PluginInfo, func(i, j int) bool { return snap.PluginInfo[i].Slug < snap.PluginInfo[j].Slug })

	return snap, nil
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
		w.Header().Set(RefreshedAtHeader, t.Format(time.RFC3339))
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
