package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

const (
	defaultPluginHistoryLimit = 100
	maxPluginHistoryLimit     = 500
)

// PluginHistoryEvent is one change of a plugin on one site, taken from the
// site update ledger.
type PluginHistoryEvent struct {
	LedgerID int    `json:"ledger_id"`
	SiteID   string `json:"site_id"`
	// SiteName is empty for sites jman no longer knows.
	SiteName string    `json:"site_name"`
	At       time.Time `json:"at"`
	// By is the user, "cron" for scheduled updates, or "detected (agent)"
	// and the like for changes made outside jman.
	By string `json:"by"`
	// Action is "update", "install", "activate", "deactivate", "delete" or
	// "uninstall".
	Action string `json:"action"`
	// Detected is set for changes made outside jman, which jman-agent or a
	// refresh found.
	Detected bool `json:"detected,omitempty"`
	// Outcome is "success", "failed", "skipped" (held back by an update
	// lock) or "unchanged" (an update that left the version as it was).
	Outcome    string `json:"outcome"`
	OldVersion string `json:"old_version,omitempty"`
	NewVersion string `json:"new_version,omitempty"`
	// Source is the ZIP or slug an install job installed.
	Source string `json:"source,omitempty"`
	Note   string `json:"note,omitempty"`
	Error  string `json:"error,omitempty"`
}

// PluginHistoryHandler lists a plugin's changes on every site, newest
// first: updates, installs and other plugin actions made through jman, and
// changes found on sites that jman didn't make. It reads at most limit
// ledger entries (default 100, at most 500) older than ?before (a ledger
// ID); next_before continues the list, and is null at its end.
//
//	GET /api/plugins/{slug}/history?limit=100&before=<ledger id>
func PluginHistoryHandler(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !pluginSlugRegex.MatchString(slug) || strings.Contains(slug, "/") {
		WriteError(w, http.StatusBadRequest, "Invalid plugin slug")
		return
	}
	limit := defaultPluginHistoryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			WriteError(w, http.StatusBadRequest, "Invalid limit")
			return
		}
		limit = min(n, maxPluginHistoryLimit)
	}
	before := 0
	if raw := r.URL.Query().Get("before"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			WriteError(w, http.StatusBadRequest, "Invalid before")
			return
		}
		before = n
	}

	entries, err := db.ListPluginLedgerEntries(slug, limit, before)
	if err != nil {
		verb.LogPrintf(verb.Normal, "PluginHistoryHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to read the update ledger")
		return
	}

	names := map[string]string{}
	if sites, err := cache.GetFastSiteList(); err == nil {
		for _, s := range sites {
			names[s.ID] = s.Name
		}
	}
	events := []PluginHistoryEvent{}
	for _, e := range entries {
		for _, ev := range pluginLedgerEvents(e, slug) {
			ev.SiteName = names[e.SiteID]
			events = append(events, ev)
		}
	}
	var next *int
	if len(entries) == limit {
		next = &entries[len(entries)-1].ID
	}
	WriteJSON(w, http.StatusOK, map[string]any{"events": events, "next_before": next})
}

// ledgerData holds every field of the plugin ledger data formats: a single
// update, a bulk update, a plugin action and a detected change.
type ledgerData struct {
	ledgerPlugin
	Updates  []ledgerPlugin `json:"updates"`
	Action   string         `json:"action"`
	Plugins  []ledgerPlugin `json:"plugins"`
	Source   string         `json:"source"`
	Detected bool           `json:"detected"`
	Changes  []struct {
		Plugin     string `json:"plugin"`
		Change     string `json:"change"`
		OldVersion string `json:"old_version"`
		NewVersion string `json:"new_version"`
	} `json:"changes"`
}

type ledgerPlugin struct {
	Plugin     string `json:"plugin"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	// Version is the version a plugin action left.
	Version string `json:"version"`
	// Status is set in bulk updates and actions: "success", "failed" or
	// "skipped".
	Status  string `json:"status"`
	Skipped bool   `json:"skipped"`
	Note    string `json:"note"`
	Error   string `json:"error"`
}

// detectedActions maps detected change kinds to event actions.
var detectedActions = map[string]string{
	"installed":   "install",
	"removed":     "delete",
	"updated":     "update",
	"downgraded":  "update",
	"activated":   "activate",
	"deactivated": "deactivate",
}

// pluginLedgerEvents returns the events of a ledger entry that concern the
// plugin.
func pluginLedgerEvents(e models.SiteUpdateLedgerEntry, slug string) []PluginHistoryEvent {
	var data ledgerData
	if e.UpdateType != "plugin" || json.Unmarshal([]byte(e.DataJSON), &data) != nil {
		return nil
	}
	base := PluginHistoryEvent{LedgerID: e.ID, SiteID: e.SiteID, At: e.UpdatedAt, By: e.UpdatedBy}
	var events []PluginHistoryEvent

	switch {
	case data.Detected:
		for _, c := range data.Changes {
			if c.Plugin != slug {
				continue
			}
			ev := base
			ev.Action = detectedActions[c.Change]
			if ev.Action == "" {
				ev.Action = c.Change
			}
			ev.Detected = true
			ev.Outcome = "success"
			ev.OldVersion, ev.NewVersion = c.OldVersion, c.NewVersion
			events = append(events, ev)
		}
	case data.Action != "":
		for _, p := range data.Plugins {
			if p.Plugin != slug {
				continue
			}
			ev := base
			ev.Action = data.Action
			ev.Outcome = outcome(p)
			ev.NewVersion = p.Version
			ev.Note, ev.Error = p.Note, p.Error
			if data.Action == "install" {
				ev.Source = data.Source
			}
			events = append(events, ev)
		}
	default:
		updates := data.Updates
		if data.Plugin != "" {
			updates = []ledgerPlugin{data.ledgerPlugin}
		}
		for _, u := range updates {
			if u.Plugin != slug {
				continue
			}
			ev := base
			ev.Action = "update"
			ev.Outcome = outcome(u)
			ev.OldVersion, ev.NewVersion = u.OldVersion, u.NewVersion
			ev.Note, ev.Error = u.Note, u.Error
			events = append(events, ev)
		}
	}
	return events
}

func outcome(p ledgerPlugin) string {
	switch {
	case p.Error != "" || p.Status == "failed":
		return "failed"
	case p.Skipped || p.Status == "skipped":
		return "skipped"
	case p.OldVersion != "" && p.OldVersion == p.NewVersion:
		return "unchanged"
	}
	return "success"
}
