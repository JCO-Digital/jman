package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
)

const (
	VulnBackportTypePlugin = "plugin"
	VulnBackportTypeCore   = "core"
)

// VulnBackport records that a vulnerability was fixed by a backport release
// the vulnerability database doesn't know about: every version within Branch
// (e.g. "10.9") that is at least MinVersion (e.g. "10.9.5") is not affected.
type VulnBackport struct {
	ID         int       `json:"id"`
	Type       string    `json:"type"`      // plugin or core
	Slug       string    `json:"slug"`      // plugin slug; empty for core
	VulnUUID   string    `json:"vuln_uuid"` // vulnerability UUID
	Branch     string    `json:"branch"`
	MinVersion string    `json:"min_version"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  string    `json:"created_by"`
	UpdatedAt  time.Time `json:"updated_at"`
	UpdatedBy  string    `json:"updated_by"`
}

// parseBranch parses a branch like "10.9" into its numeric segments.
func parseBranch(branch string) ([]int64, error) {
	if branch == "" {
		return nil, fmt.Errorf("branch is required")
	}
	parts := strings.Split(branch, ".")
	segs := make([]int64, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid branch %q: segments must be non-negative integers", branch)
		}
		segs = append(segs, n)
	}
	return segs, nil
}

// inBranch reports whether v's leading segments equal the branch segments.
// Segments are compared numerically, so "10.9" never matches "10.90.1".
func inBranch(v *version.Version, branch []int64) bool {
	segs := v.Segments64()
	if len(segs) < len(branch) {
		return false
	}
	for i, b := range branch {
		if segs[i] != b {
			return false
		}
	}
	return true
}

// Validate checks the rule's own shape, independent of any vulnerability data.
func (b VulnBackport) Validate() error {
	switch b.Type {
	case VulnBackportTypePlugin:
		if b.Slug == "" {
			return fmt.Errorf("slug is required for plugin rules")
		}
		if strings.ContainsAny(b.Slug, `/\`) || b.Slug == "." || b.Slug == ".." {
			return fmt.Errorf("invalid plugin slug %q", b.Slug)
		}
	case VulnBackportTypeCore:
		if b.Slug != "" {
			return fmt.Errorf("slug must be empty for core rules")
		}
	default:
		return fmt.Errorf("type must be %q or %q", VulnBackportTypePlugin, VulnBackportTypeCore)
	}
	if b.VulnUUID == "" {
		return fmt.Errorf("vulnerability UUID is required")
	}
	// Rules are permanent, so record why each one exists.
	if strings.TrimSpace(b.Reason) == "" {
		return fmt.Errorf("reason is required")
	}

	branch, err := parseBranch(b.Branch)
	if err != nil {
		return err
	}

	if b.MinVersion == "" {
		return fmt.Errorf("minimum version is required")
	}
	minVer, err := version.NewVersion(b.MinVersion)
	if err != nil {
		return fmt.Errorf("invalid minimum version %q", b.MinVersion)
	}
	if minVer.Prerelease() != "" || minVer.Metadata() != "" {
		return fmt.Errorf("minimum version %q must not have a pre-release or build suffix", b.MinVersion)
	}
	// Plain dotted numbers only: the version also names a cache file.
	if _, err := parseBranch(b.MinVersion); err != nil {
		return fmt.Errorf("invalid minimum version %q", b.MinVersion)
	}
	if strings.Count(b.MinVersion, ".")+1 <= len(branch) {
		return fmt.Errorf("minimum version %q must have more segments than branch %q", b.MinVersion, b.Branch)
	}
	if !inBranch(minVer, branch) {
		return fmt.Errorf("minimum version %q is not within branch %q", b.MinVersion, b.Branch)
	}
	return nil
}

// Matches reports whether the installed version ver is covered by the rule:
// within the branch and at least the minimum version. Unparseable versions
// never match, so they keep being reported as vulnerable.
func (b VulnBackport) Matches(ver string) bool {
	v, err := version.NewVersion(ver)
	if err != nil {
		return false
	}
	minVer, err := version.NewVersion(b.MinVersion)
	if err != nil {
		return false
	}
	branch, err := parseBranch(b.Branch)
	if err != nil {
		return false
	}
	return inBranch(v, branch) && v.GreaterThanOrEqual(minVer)
}

// VulnBackportListing is a backport rule with the number of sites whose
// installed version it currently matches.
type VulnBackportListing struct {
	VulnBackport
	MatchedSites int `json:"matched_sites"`
}
