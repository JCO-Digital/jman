package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// AgentWPData is one site's WordPress state as jman-agent collected it with
// wp-cli: installed plugins and the core version with available updates.
// The agent sends it in full only when its hash differs from the one
// jman-api holds (AgentManifestSite.WPDataHash); otherwise it sends just
// the hash and collection time, so jman-api still knows the site is being
// collected.
type AgentWPData struct {
	// CollectedAt is when the agent collected the data (RFC3339, UTC).
	CollectedAt string `json:"collected_at"`
	// Hash is WPDataHash of the collected data; empty if Error is set.
	Hash string `json:"hash,omitempty"`
	// Plugins and Core are present only when Hash differs from the hash
	// jman-api sent in the manifest. Plugins' SiteID is left empty.
	Plugins []WPPlugin `json:"plugins,omitempty"`
	Core    *SiteCore  `json:"core,omitempty"`
	// Error explains why the site couldn't be collected (e.g. the agent
	// refused to run wp-cli as the directory's owner, or wp-cli failed).
	Error string `json:"error,omitempty"`
}

// WPDataHash returns a hash of a site's plugins and core state, used to
// tell whether jman-api's copy is current. It ignores plugin order and
// SiteID, so the same state hashes the same whichever side computes it.
func WPDataHash(plugins []WPPlugin, core SiteCore) string {
	lines := make([]string, 0, len(plugins)+1)
	for _, p := range plugins {
		lines = append(lines, fmt.Sprintf("p\x00%s\x00%s\x00%s\x00%s\x00%t", p.Name, p.Status, p.Version, p.Update, p.AutoUpdate))
	}
	sort.Strings(lines)
	lines = append(lines, fmt.Sprintf("c\x00%s\x00%s\x00%s", core.Version, core.MinorUpdate, core.MajorUpdate))
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}
