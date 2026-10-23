package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// State is the baseline the agent keeps for a site: what the last
// reported scan found. It lives under the agent's state directory, owned
// by root and readable only by root, so the site's owner can't change it.
type State struct {
	RuleSetVersion string           `json:"rule_set_version"`
	Fingerprint    string           `json:"fingerprint"`
	Entries        map[string]Entry `json:"entries"`
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
		e.Head = ""
		s.Entries[e.Path] = e
	}
	return s
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
