package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// State is the baseline the agent keeps for a site: what the last
// reported scan found. It lives under the agent's state directory, owned
// by root and readable only by root, so the site's owner can't change it.
type State struct {
	RuleSetVersion string           `json:"rule_set_version"`
	Fingerprint    string           `json:"fingerprint"`
	Entries        map[string]Entry `json:"entries"`
	// CoreVersion and CoreLocale are the site's WordPress version and
	// package locale at the last scan: the next scan checks core files
	// against their checksums.
	CoreVersion string `json:"core_version,omitempty"`
	CoreLocale  string `json:"core_locale,omitempty"`
	// ContentScannedAt is when the walk last searched every PHP file for
	// the content rules (RFC 3339).
	ContentScannedAt string `json:"content_scanned_at,omitempty"`
	// Plugins are the plugin versions at the last scan, by slug, which the
	// next scan checks against their checksums.
	Plugins map[string]string `json:"plugins,omitempty"`
}

// siteIDRe guards the state file name, which is built from the site ID
// jman-api sends.
var siteIDRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func statePath(stateDir, siteID string) (string, error) {
	if !siteIDRe.MatchString(siteID) {
		return "", fmt.Errorf("invalid site ID %q", siteID)
	}
	return filepath.Join(stateDir, "scan", siteID+".json"), nil
}

// LoadState returns a site's baseline, or nil if there is none.
func LoadState(stateDir, siteID string) (*State, error) {
	p, err := statePath(stateDir, siteID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", p, err)
	}
	if s.Entries == nil {
		s.Entries = map[string]Entry{}
	}
	return &s, nil
}

// NewState builds the baseline for a walk's entries. Heads aren't kept.
func NewState(entries []Entry) *State {
	s := &State{RuleSetVersion: RuleSetVersion, Fingerprint: Fingerprint(entries), Entries: make(map[string]Entry, len(entries))}
	for _, e := range entries {
		e.Head, e.ContentDetail = "", ""
		s.Entries[e.Path] = e
	}
	return s
}

// CarryChecksums keeps the checksum findings of the previous baseline for
// core and the plugins a walk didn't check (no checksums, or updated
// since): they're neither new nor cleared until the next check. Those of a
// plugin that's gone, or of core when there's none, aren't kept.
func CarryChecksums(prev *State, res *Result) {
	if prev == nil {
		return
	}
	at := make(map[string]int, len(res.Entries))
	for i, e := range res.Entries {
		at[e.Path] = i
	}
	paths := make([]string, 0)
	for p, e := range prev.Entries {
		if e.Checksum == "" {
			continue
		}
		if isCoreRule(e.Checksum) {
			if res.CoreChecked || res.CoreVersion == "" {
				continue
			}
		} else {
			slug, _ := pluginOf(p)
			if _, present := res.Plugins[slug]; !present || res.PluginsChecked[slug] {
				continue
			}
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		old := prev.Entries[p]
		if i, ok := at[p]; ok {
			res.Entries[i].Checksum = old.Checksum
			continue
		}
		res.Entries = append(res.Entries, old)
	}
}

// SaveState writes a site's baseline, replacing the old one atomically.
func SaveState(stateDir, siteID string, s *State) error {
	p, err := statePath(stateDir, siteID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), siteID+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Watched returns the files the content rules flagged in the baseline,
// which the next walk searches again.
func (s *State) Watched() []string {
	var paths []string
	for p, e := range s.Entries {
		if len(e.Content) > 0 {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}
