// Package sitestate is the single way a site's observed WordPress state
// (installed plugins, core version) gets into jman's cache. Whoever read
// it, jman-agent, an SSH refresh or an update job, the state is compared
// with what jman knew before, and changes jman didn't make itself are
// recorded in the site's update ledger as detected changes.
//
// Ledger entries need jman-api's database; the CLI, which only has the
// inventory, just updates its cache.
package sitestate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/hashicorp/go-version"
)

// Source says where an observed state came from.
type Source string

const (
	// SourceAgent is jman-agent's periodic collection.
	SourceAgent Source = "agent"
	// SourceSSH is a refresh over SSH: the periodic sweep, agentless
	// collection or an explicit "check for updates".
	SourceSSH Source = "ssh"
	// SourceJob is the refresh after one of jman's own update or plugin
	// management jobs, which records its own ledger entry; changes in it
	// are attributed to the job and not logged again.
	SourceJob Source = "job"
)

// minPluginsForLossCheck and the half-lost rule below decide when a plugin
// list is suspect: a read that drops most of a site's plugins is far more
// likely a failed or partial wp-cli run than a real mass removal.
const minPluginsForLossCheck = 4

var (
	// suspect holds, by site, the hash of the last plugin list held back
	// as suspect. The same list read again is accepted: a real removal
	// keeps showing up, a failed read usually doesn't.
	suspect   = map[string]string{}
	suspectMu sync.Mutex
)

// ApplyPlugins stores a site's observed plugin list and returns whether it
// did. It skips a list observed before the stored one (an older agent
// snapshot arriving after an SSH refresh) and holds back a suspect list
// (see minPluginsForLossCheck) until it is seen twice. Unless src is
// SourceJob or the site had no stored list yet, changes to regular
// plugins (not must-use plugins or drop-ins) are written to the update
// ledger.
func ApplyPlugins(siteID string, plugins []models.WPPlugin, src Source, observedAt time.Time) (bool, error) {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	known, err := db.GetSitePlugins(siteID)
	if err != nil {
		return false, err
	}
	knownAt, hasKnown, err := db.GetSitePluginsObservedAt(siteID)
	if err != nil {
		return false, err
	}
	if hasKnown && observedAt.Before(knownAt) {
		return false, nil
	}

	for i := range plugins {
		plugins[i].SiteID = siteID
	}
	if hasKnown && src != SourceJob && looksTruncated(known, plugins) {
		hash := pluginListHash(plugins)
		suspectMu.Lock()
		seen := suspect[siteID] == hash
		suspect[siteID] = hash
		suspectMu.Unlock()
		if !seen {
			log.Printf("Holding back the plugin list of site %s from %s: it lacks most of the %d known plugins, which usually means a failed read", siteID, src, len(regular(known)))
			return false, nil
		}
	}
	suspectMu.Lock()
	delete(suspect, siteID)
	suspectMu.Unlock()

	if err := db.ReplaceSitePlugins(siteID, plugins, observedAt); err != nil {
		return false, err
	}

	if hasKnown && src != SourceJob && db.GetAPIDB() != nil {
		if changes := diffPlugins(known, plugins); len(changes) > 0 {
			writeLedger(siteID, "plugin", src, map[string]any{
				"detected":    true,
				"source":      src,
				"changes":     changes,
				"observed_at": observedAt.UTC().Format(time.RFC3339),
				"since":       knownAt.UTC().Format(time.RFC3339),
			})
		}
	}
	return true, nil
}

// ApplyCore stores a site's observed core state and returns whether it did
// (it skips a state observed before the stored one). Unless src is
// SourceJob, a change of the major version (6.6.x → 6.7.y) jman didn't
// make is written to the update ledger; minor changes, which WordPress
// applies on its own by default, aren't.
func ApplyCore(siteID string, core models.SiteCore, src Source, observedAt time.Time) (bool, error) {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	known, knownAt, err := db.GetSiteCore(siteID)
	if err != nil {
		return false, err
	}
	if known != nil && !knownAt.IsZero() && observedAt.Before(knownAt) {
		return false, nil
	}
	if err := db.SaveSiteCoreObserved(siteID, core.Version, core.MinorUpdate, core.MajorUpdate, observedAt); err != nil {
		return false, err
	}

	if known != nil && src != SourceJob && db.GetAPIDB() != nil && majorChange(known.Version, core.Version) {
		data := map[string]any{
			"detected":    true,
			"source":      src,
			"target":      "major",
			"old_version": known.Version,
			"new_version": core.Version,
			"observed_at": observedAt.UTC().Format(time.RFC3339),
		}
		if !knownAt.IsZero() {
			data["since"] = knownAt.UTC().Format(time.RFC3339)
		}
		writeLedger(siteID, "core", src, data)
	}
	return true, nil
}

// StoredHash returns the models.WPDataHash of the plugins and core state
// stored for a site, or "" if neither is stored.
func StoredHash(siteID string) (string, error) {
	plugins, err := db.GetSitePlugins(siteID)
	if err != nil {
		return "", err
	}
	core, _, err := db.GetSiteCore(siteID)
	if err != nil {
		return "", err
	}
	if len(plugins) == 0 && core == nil {
		return "", nil
	}
	var c models.SiteCore
	if core != nil {
		c = *core
	}
	return models.WPDataHash(plugins, c), nil
}

// PluginChange is one detected change of a regular plugin.
type PluginChange struct {
	Plugin string `json:"plugin"`
	// Change is "installed", "removed", "updated", "downgraded",
	// "activated" or "deactivated".
	Change     string `json:"change"`
	OldVersion string `json:"old_version,omitempty"`
	NewVersion string `json:"new_version,omitempty"`
}

// diffPlugins lists the changes from known to observed, ignoring must-use
// plugins and drop-ins, sorted by plugin.
func diffPlugins(known, observed []models.WPPlugin) []PluginChange {
	before := regular(known)
	after := regular(observed)
	var changes []PluginChange
	for name, old := range before {
		cur, ok := after[name]
		if !ok {
			changes = append(changes, PluginChange{Plugin: name, Change: "removed", OldVersion: old.Version})
			continue
		}
		if old.Version != cur.Version {
			change := "updated"
			if older(cur.Version, old.Version) {
				change = "downgraded"
			}
			changes = append(changes, PluginChange{Plugin: name, Change: change, OldVersion: old.Version, NewVersion: cur.Version})
		}
		if wasActive, isActive := active(old.Status), active(cur.Status); wasActive != isActive {
			change := "deactivated"
			if isActive {
				change = "activated"
			}
			changes = append(changes, PluginChange{Plugin: name, Change: change, NewVersion: cur.Version})
		}
	}
	for name, cur := range after {
		if _, ok := before[name]; !ok {
			changes = append(changes, PluginChange{Plugin: name, Change: "installed", NewVersion: cur.Version})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Plugin < changes[j].Plugin })
	return changes
}

// looksTruncated reports whether observed lacks most of known's regular
// plugins (or all of them).
func looksTruncated(known, observed []models.WPPlugin) bool {
	before := regular(known)
	after := regular(observed)
	if len(before) == 0 {
		return false
	}
	if len(after) == 0 {
		return true
	}
	if len(before) < minPluginsForLossCheck {
		return false
	}
	lost := 0
	for name := range before {
		if _, ok := after[name]; !ok {
			lost++
		}
	}
	return lost*2 > len(before)
}

func regular(plugins []models.WPPlugin) map[string]models.WPPlugin {
	m := make(map[string]models.WPPlugin, len(plugins))
	for _, p := range plugins {
		if p.Status == "must-use" || p.Status == "dropin" {
			continue
		}
		m[p.Name] = p
	}
	return m
}

func active(status string) bool {
	return status == "active" || status == "active-network"
}

func older(a, b string) bool {
	va, errA := version.NewVersion(a)
	vb, errB := version.NewVersion(b)
	return errA == nil && errB == nil && va.LessThan(vb)
}

// majorChange reports whether two WordPress versions differ in their
// major.minor part (WordPress calls 6.6 → 6.7 a major release).
func majorChange(a, b string) bool {
	if a == b || a == "" || b == "" {
		return false
	}
	return branch(a) != branch(b)
}

func branch(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return parts[0] + ".0"
	}
	// "6.7-RC1" belongs to the 6.7 branch.
	minor, _, _ := strings.Cut(parts[1], "-")
	return parts[0] + "." + minor
}

func pluginListHash(plugins []models.WPPlugin) string {
	lines := make([]string, len(plugins))
	for i, p := range plugins {
		lines[i] = p.Name + "\x00" + p.Status + "\x00" + p.Version
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func writeLedger(siteID, updateType string, src Source, data map[string]any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to encode detected %s change for site %s: %v", updateType, siteID, err)
		return
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     siteID,
		UpdateType: updateType,
		Status:     models.LedgerDetected,
		DataJSON:   string(encoded),
		UpdatedBy:  fmt.Sprintf("detected (%s)", src),
	}); err != nil {
		log.Printf("Failed to record detected %s change for site %s: %v", updateType, siteID, err)
	}
}
