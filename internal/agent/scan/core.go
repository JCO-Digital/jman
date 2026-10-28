package scan

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"syscall"
)

// CoreChecksums are the MD5 checksums of a WordPress release's files,
// keyed by path relative to the site directory, as api.wordpress.org
// publishes them.
type CoreChecksums struct {
	Version string            `json:"version"`
	Locale  string            `json:"locale"`
	MD5     map[string]string `json:"md5"`
}

// Options are what a walk needs beyond the site directory.
type Options struct {
	// Core, when set, are the checksums of the WordPress version and
	// locale the site last had. The walk checks core files against them
	// if the site still has that version.
	Core *CoreChecksums `json:"core,omitempty"`
	// Plugins, by slug, are the checksums of the plugin versions the site
	// last had; each plugin that still has that version is checked.
	Plugins map[string]*PluginChecksums `json:"plugins,omitempty"`
	// Content makes the walk search every PHP file in wp-content and the
	// site directory for the content rules, not only the ones it reads
	// anyway and Watch.
	Content bool `json:"content,omitempty"`
	// Watch are files the content rules flagged before, which the walk
	// reads and searches again.
	Watch []string `json:"watch,omitempty"`
	// Home, when set, is the site user's home directory: the walk also
	// records its shell startup files and programs (see WalkHome).
	Home string `json:"home,omitempty"`
}

// maxCoreModified is how many core files may differ from their checksums
// before the walk takes it for an update in progress (or checksums for
// the wrong version) and leaves the core rules out.
const maxCoreModified = 50

// maxVersionBytes is how much of wp-includes/version.php is read.
const maxVersionBytes = 64 << 10

var (
	wpVersionRe      = regexp.MustCompile(`(?m)^\s*\$wp_version\s*=\s*['"]([^'"]+)['"]\s*;`)
	wpLocalPackageRe = regexp.MustCompile(`(?m)^\s*\$wp_local_package\s*=\s*['"]([^'"]+)['"]\s*;`)
)

// readCoreVersion returns the WordPress version and package locale set in
// a site's wp-includes/version.php, or "" if there is none. The locale is
// en_US unless the package is localized.
func readCoreVersion(root string) (string, string) {
	b, ok := readSmallFile(root+"/wp-includes/version.php", maxVersionBytes)
	if !ok {
		return "", ""
	}
	m := wpVersionRe.FindSubmatch(b)
	if m == nil {
		return "", ""
	}
	locale := "en_US"
	if l := wpLocalPackageRe.FindSubmatch(b); l != nil {
		locale = string(l[1])
	}
	return string(m[1]), locale
}

// readSmallFile reads up to max bytes of a regular file without following
// a symlink.
func readSmallFile(full string, max int64) ([]byte, bool) {
	fd, err := syscall.Open(full, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, false
	}
	f := os.NewFile(uintptr(fd), full)
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(f, max))
	return b, err == nil
}

// isCoreArea reports whether WordPress core owns p: everything in
// wp-admin and wp-includes, and the files in the site directory itself.
// wp-content is the site's own, bundled themes and plugins included.
func isCoreArea(p string) bool {
	return strings.HasPrefix(p, "wp-admin/") || strings.HasPrefix(p, "wp-includes/") || !strings.Contains(p, "/")
}

// coreRule returns the core rule a file in the core area breaks, if any:
// RuleCoreModified for a core file that differs from its checksum, and
// RuleCoreUnknown for a file core doesn't have in wp-admin or wp-includes,
// or a PHP file it doesn't have in the site directory. wp-config.php is
// the site's own. readMD5 hashes the file; it's only called for core
// files.
func coreRule(p string, sums map[string]string, readMD5 func() (string, bool)) string {
	want, known := sums[p]
	if known {
		if got, ok := readMD5(); !ok || got != want {
			return RuleCoreModified
		}
		return ""
	}
	if strings.Contains(p, "/") || (isPHP(path.Base(p)) && p != "wp-config.php") {
		return RuleCoreUnknown
	}
	return ""
}

// fileMD5 returns a regular file's MD5, without following a symlink.
func fileMD5(full string) (string, bool) {
	fd, err := syscall.Open(full, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", false
	}
	f := os.NewFile(uintptr(fd), full)
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > maxHashBytes {
		return "", false
	}
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
}
