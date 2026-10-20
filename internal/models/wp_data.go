package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// AgentWPData is one site's WordPress state as jman-agent collected it with
// wp-cli: installed plugins, the core version with available updates and
// the site's users.
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
	// Users is sent with Plugins and Core by agents that collect users
	// (and could list them this time); nil otherwise. Its SiteIDs are
	// left empty.
	Users *SiteUsers `json:"users,omitempty"`
	// Error explains why the site couldn't be collected (e.g. the agent
	// refused to run wp-cli as the directory's owner, or wp-cli failed).
	Error string `json:"error,omitempty"`
}

// WPDataHash returns a hash of a site's plugins, core state and users,
// used to tell whether jman-api's copy is current. It ignores order and
// SiteID, so the same state hashes the same whichever side computes it.
// Nil users add nothing, so the hash of an agent that doesn't collect
// users is unchanged.
func WPDataHash(plugins []WPPlugin, core SiteCore, users *SiteUsers) string {
	lines := make([]string, 0, len(plugins)+1)
	for _, p := range plugins {
		lines = append(lines, fmt.Sprintf("p\x00%s\x00%s\x00%s\x00%s\x00%t", p.Name, p.Status, p.Version, p.Update, p.AutoUpdate))
	}
	if users != nil {
		for _, u := range users.Users {
			line := fmt.Sprintf("u\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s", u.ID, u.Login, u.Email, u.DisplayName, u.Roles, u.Registered)
			// Unknown counts add nothing, so users stored without them
			// still match.
			if u.PostCount != nil {
				line += fmt.Sprintf("\x00p%d", *u.PostCount)
			}
			lines = append(lines, line)
		}
		for role, n := range users.ExcludedCounts {
			lines = append(lines, fmt.Sprintf("x\x00%s\x00%d", role, n))
		}
		lines = append(lines, fmt.Sprintf("t\x00%t", users.Truncated))
	}
	sort.Strings(lines)
	lines = append(lines, fmt.Sprintf("c\x00%s\x00%s\x00%s", core.Version, core.MinorUpdate, core.MajorUpdate))
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}
