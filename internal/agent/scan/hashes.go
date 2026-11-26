package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// FileHash is a file's hash as a walk computed it, with the stat details
// that tell whether the file has changed since. The walk reuses it instead
// of reading the file again while they all still match: an edit changes
// the size, mtime or ctime, and replacing the file changes the inode. The
// site's owner can set the mtime but not the ctime, which every change
// sets to the current time.
type FileHash struct {
	Dev   uint64 `json:"d"`
	Ino   uint64 `json:"i"`
	Size  int64  `json:"s"`
	Mtime int64  `json:"m"` // nanoseconds since the epoch
	Ctime int64  `json:"c"`
	// MD5 is set for core files (checked against core checksums), SHA256
	// for plugin files.
	MD5    string `json:"md5,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

// sameFile reports whether a cached hash still describes the file whose
// stat details are cur.
func (h FileHash) sameFile(cur FileHash) bool {
	return h.Dev == cur.Dev && h.Ino == cur.Ino && h.Size == cur.Size && h.Mtime == cur.Mtime && h.Ctime == cur.Ctime
}

// racyWindow is how recently a file may have changed and still have its
// hash kept. A file changed in the same clock tick as the walk could
// change again without its ctime moving (on filesystems with coarse
// timestamps), so its hash isn't trusted later.
var racyWindow = 2 * time.Second

// hashCache looks up and records file hashes during one walk.
type hashCache struct {
	old   map[string]FileHash
	new   map[string]FileHash
	since int64 // files changed after this (ns) aren't kept
}

func newHashCache(old map[string]FileHash, start time.Time) *hashCache {
	return &hashCache{old: old, new: map[string]FileHash{}, since: start.Add(-racyWindow).UnixNano()}
}

// get returns the cached hash of rel if the file hasn't changed, and keeps
// it for the next walk.
func (c *hashCache) get(rel string, info fs.FileInfo) (FileHash, bool) {
	h, ok := c.old[rel]
	if !ok {
		return FileHash{}, false
	}
	cur, ok := statHash(info)
	if !ok || !h.sameFile(cur) {
		return FileHash{}, false
	}
	c.new[rel] = h
	return h, true
}

// put records the hash the walk computed for rel, unless the file changed
// too recently to trust its stat details later.
func (c *hashCache) put(rel string, info fs.FileInfo, md5, sha256 string) {
	cur, ok := statHash(info)
	if !ok || cur.Ctime >= c.since || cur.Mtime >= c.since {
		return
	}
	cur.MD5, cur.SHA256 = md5, sha256
	c.new[rel] = cur
}

func hashesPath(stateDir, siteID string) (string, error) {
	if !siteIDRe.MatchString(siteID) {
		return "", fmt.Errorf("invalid site ID %q", siteID)
	}
	return filepath.Join(stateDir, "scan", siteID+".hashes.json"), nil
}

// LoadHashes returns the file hashes the last walk of a site kept, or nil.
// Like the baseline, they live in the agent's root-only state directory:
// the site's owner can't plant hashes that would hide a change.
func LoadHashes(stateDir, siteID string) (map[string]FileHash, error) {
	p, err := hashesPath(stateDir, siteID)
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
	var hashes map[string]FileHash
	if err := json.Unmarshal(b, &hashes); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", p, err)
	}
	return hashes, nil
}

// SaveHashes replaces a site's kept file hashes.
func SaveHashes(stateDir, siteID string, hashes map[string]FileHash) error {
	p, err := hashesPath(stateDir, siteID)
	if err != nil {
		return err
	}
	return writeFileAtomic(p, siteID, hashes)
}
