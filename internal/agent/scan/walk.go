package scan

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// maxHashBytes is the largest file the walk hashes. A bigger tracked file
// is still recorded, by size only.
const maxHashBytes = 16 << 20

// headBytes is how much of a tracked file is kept as its snippet.
const headBytes = 512

// maxIniBytes is how much of a .user.ini or .htaccess is searched for
// PHP directives.
const maxIniBytes = 1 << 20

// maxDirectiveLength caps a reported directive line.
const maxDirectiveLength = 300

// maxTrackedEntries and maxOtherEntries cap how many files a walk records:
// tracked files first, then the rest (foreign-owned, stray and unreadable
// ones), which a misconfigured site can have tens of thousands of.
const (
	maxTrackedEntries = 50000
	maxOtherEntries   = 10000
)

// Entry is one file or directory the walk recorded.
type Entry struct {
	Path    string `json:"path"` // slash-separated, relative to the site's directory
	Dir     bool   `json:"dir,omitempty"`
	Symlink string `json:"symlink,omitempty"` // the link's target, for a symlink
	UID     uint32 `json:"uid"`
	Size    int64  `json:"size"`
	// SHA256 is set for tracked regular files up to maxHashBytes.
	SHA256 string `json:"sha256,omitempty"`
	// Unreadable is set when the walk couldn't read the file or list the
	// directory.
	Unreadable bool `json:"unreadable,omitempty"`
	// Directives are the auto_prepend_file and auto_append_file settings
	// in a .user.ini or .htaccess.
	Directives []string `json:"directives,omitempty"`
	// Head is the start of a tracked file, for snippets. It isn't kept in
	// the baseline.
	Head string `json:"head,omitempty"`
	// Checksum is the checksum rule the file breaks (see coreRule and
	// pluginRule), for a core or plugin file checked against wordpress.org's
	// checksums.
	Checksum string `json:"checksum,omitempty"`
	// Content names the content rules a PHP file matches (see
	// contentPatterns), and ContentDetail quotes where. The detail isn't
	// kept in the baseline.
	Content       []string `json:"content,omitempty"`
	ContentDetail string   `json:"content_detail,omitempty"`
}

// Result is what a walk found.
type Result struct {
	Entries []Entry `json:"entries"`
	// Omitted counts files left out because the caps were reached.
	Omitted int `json:"omitted,omitempty"`
	// CoreVersion and CoreLocale are the WordPress version and package
	// locale in wp-includes/version.php ("" when there is none).
	CoreVersion string `json:"core_version,omitempty"`
	CoreLocale  string `json:"core_locale,omitempty"`
	// CoreChecked is set when core files were checked against checksums,
	// and CoreNote says why they weren't when checksums were given.
	CoreChecked bool   `json:"core_checked,omitempty"`
	CoreNote    string `json:"core_note,omitempty"`
	// Plugins are the versions of the plugins in wp-content/plugins, by
	// slug, PluginsChecked the ones checked against checksums, and
	// PluginNotes why one wasn't when checksums were given.
	Plugins        map[string]string `json:"plugins,omitempty"`
	PluginsChecked map[string]bool   `json:"plugins_checked,omitempty"`
	PluginNotes    []string          `json:"plugin_notes,omitempty"`
	// ContentChecked is set when the walk searched every PHP file in
	// wp-content and the site directory for the content rules.
	ContentChecked bool `json:"content_checked,omitempty"`
}

// Walk looks through the site directory root and records the files that
// matter to the rules: tracked files (see isTracked) with their hashes,
// stray files, and anything not owned by ownerUID or that can't be read.
// With checksums for the site's WordPress and plugin versions in opts, it
// also records the core and plugin files that differ from them, and files
// they don't have (see coreRule and pluginRule). It never follows symlinks
// and opens only regular files.
func Walk(root string, ownerUID uint32, opts Options) (*Result, error) {
	res := &Result{Entries: []Entry{}, ContentChecked: opts.Content}
	res.CoreVersion, res.CoreLocale = readCoreVersion(root)
	watch := make(map[string]bool, len(opts.Watch))
	for _, p := range opts.Watch {
		watch[p] = true
	}
	var sums map[string]string
	if c := opts.Core; c != nil {
		if c.Version == res.CoreVersion && c.Locale == res.CoreLocale {
			sums = c.MD5
			res.CoreChecked = true
		} else {
			// Updated since the last scan: the next scan checks against
			// the new version.
			res.CoreNote = "WordPress " + res.CoreVersion + " (" + res.CoreLocale + ") isn't the version the checksums are for"
		}
	}
	var elsewhere []string
	res.Plugins, elsewhere = readPluginVersions(root)
	for _, p := range elsewhere {
		res.PluginNotes = append(res.PluginNotes, p+" doesn't update from wordpress.org")
	}
	pluginSums := map[string]*PluginChecksums{}
	for slug, sums := range opts.Plugins {
		switch v, ok := res.Plugins[slug]; {
		case !ok:
		case sums.Version == v:
			pluginSums[slug] = sums
			if res.PluginsChecked == nil {
				res.PluginsChecked = map[string]bool{}
			}
			res.PluginsChecked[slug] = true
		default:
			res.PluginNotes = append(res.PluginNotes, fmt.Sprintf("%s %s isn't the version the checksums are for", slug, v))
		}
	}
	// checksumOnly holds the entries recorded only for the checksum rules.
	checksumOnly := map[string]bool{}
	coreModified := 0
	pluginModified := map[string]int{}
	pluginUnknown := map[string]int{}
	tracked, other := 0, 0
	add := func(e Entry, isTracked bool) {
		switch {
		case isTracked && tracked < maxTrackedEntries:
			tracked++
		case !isTracked && other < maxOtherEntries:
			other++
		default:
			res.Omitted++
			return
		}
		res.Entries = append(res.Entries, e)
	}

	err := filepath.WalkDir(root, func(full string, d fs.DirEntry, walkErr error) error {
		if full == root {
			return walkErr
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if walkErr != nil {
			// A directory that can't be listed. WalkDir has already
			// passed it once without an error, so it may be recorded
			// already; mark that entry instead of adding a second one.
			if n := len(res.Entries); n > 0 && res.Entries[n-1].Path == rel {
				res.Entries[n-1].Unreadable = true
			} else {
				add(Entry{Path: rel, Dir: true, Unreadable: true}, false)
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // removed during the walk
			}
			add(Entry{Path: rel, Dir: d.IsDir(), Unreadable: true}, false)
			return nil
		}
		e := Entry{Path: rel, Dir: d.IsDir(), Size: info.Size()}
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			e.UID = st.Uid
		}
		foreign := e.UID != ownerUID

		if d.IsDir() {
			if isStrayDir(rel) {
				add(e, false)
				return fs.SkipDir
			}
			// A directory that can't be listed is recorded when WalkDir
			// reports the error, so it isn't recorded here unless it's
			// foreign.
			if foreign {
				add(e, false)
			}
			return nil
		}

		if sums != nil && isCoreArea(rel) && !isTracked(rel) {
			if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
				// Core has no symlinks or special files.
				e.Checksum = coreRule(rel, sums, func() (string, bool) { return "", false })
			} else {
				e.Checksum = coreRule(rel, sums, func() (string, bool) { return fileMD5(full) })
			}
			if e.Checksum == RuleCoreModified {
				coreModified++
			}
		}
		if e.Checksum != "" {
			if info.Mode()&fs.ModeSymlink != 0 {
				if target, err := os.Readlink(full); err == nil {
					e.Symlink = target
				} else {
					e.Symlink = "?"
				}
			} else if info.Mode().IsRegular() {
				body := readTracked(full, rel, &e, IsPHPPath(rel))
				checkContent(&e, body)
			}
			if !foreign && !isStrayFile(rel) {
				checksumOnly[rel] = true
			}
			add(e, true)
			return nil
		}

		// verified is set for a plugin file that matches its checksum: the
		// content rules trust it.
		verified := false
		if slug, inner := pluginOf(rel); pluginSums[slug] != nil && IsPHPPath(rel) && info.Mode().IsRegular() && !isTracked(rel) {
			body := readTracked(full, rel, &e, true)
			if e.Checksum = pluginRule(inner, pluginSums[slug], e); e.Checksum != "" {
				checkContent(&e, body)
				if e.Checksum == RulePluginModified {
					pluginModified[slug]++
				} else {
					pluginUnknown[slug]++
				}
				if !foreign {
					checksumOnly[rel] = true
				}
				add(e, true)
				return nil
			}
			e.SHA256, e.Head = "", ""
			verified = true
		}

		if info.Mode()&fs.ModeSymlink != 0 {
			if recordSymlink(rel, e.UID, ownerUID) {
				if target, err := os.Readlink(full); err == nil {
					e.Symlink = target
				} else {
					e.Symlink = "?"
				}
				add(e, isTracked(rel))
			}
			return nil
		}

		// The content rules search the PHP files the walk reads anyway, the
		// ones flagged before, and, on a content scan, every PHP file in
		// wp-content and the site directory.
		wantContent := IsPHPPath(rel) && !verified && (isTracked(rel) || watch[rel] || (opts.Content && isContentArea(rel)))
		if info.Mode().IsRegular() && (isTracked(rel) || wantContent) {
			body := readTracked(full, rel, &e, wantContent)
			checkContent(&e, body)
			switch {
			case isTracked(rel) || len(e.Content) > 0:
				add(e, true)
			case isStrayFile(rel) || foreign:
				// Recorded as it would be without reading it.
				e.SHA256, e.Head = "", ""
				add(e, false)
			}
			return nil
		}

		switch {
		case isTracked(rel):
			add(e, true)
		case isStrayFile(rel) || foreign:
			add(e, false)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Mid-update, or not the version the checksums are for: leave the
	// rules out rather than flag half of core or a plugin.
	if coreModified > maxCoreModified {
		res.dropChecksums(checksumOnly, func(e Entry) bool { return isCoreRule(e.Checksum) })
		res.CoreChecked = false
		res.CoreNote = fmt.Sprintf("%d core files differ from the checksums of WordPress %s (%s); an update in progress?", coreModified, res.CoreVersion, res.CoreLocale)
	}
	for slug := range pluginSums {
		var note string
		switch {
		case pluginModified[slug] > maxPluginModified:
			note = fmt.Sprintf("%d files of %s %s differ from its checksums; an update in progress?", pluginModified[slug], slug, res.Plugins[slug])
		case pluginUnknown[slug] > maxPluginUnknown:
			note = fmt.Sprintf("%s %s has %d PHP files its wordpress.org release doesn't; another build, such as a premium one?", slug, res.Plugins[slug], pluginUnknown[slug])
		default:
			continue
		}
		res.dropChecksums(checksumOnly, func(e Entry) bool {
			s, _ := pluginOf(e.Path)
			return s == slug && !isCoreRule(e.Checksum)
		})
		delete(res.PluginsChecked, slug)
		res.PluginNotes = append(res.PluginNotes, note)
	}
	slices.Sort(res.PluginNotes)
	return res, nil
}

// dropChecksums takes the checksum rule off the entries match selects, and
// drops the ones recorded only for it.
func (res *Result) dropChecksums(checksumOnly map[string]bool, match func(Entry) bool) {
	kept := res.Entries[:0]
	for _, e := range res.Entries {
		if e.Checksum != "" && match(e) {
			if checksumOnly[e.Path] {
				continue
			}
			e.Checksum = ""
		}
		kept = append(kept, e)
	}
	res.Entries = kept
}

// recordSymlink reports whether the walk records a symlink. A root-owned
// one is recorded only if it's tracked or stray: no other rule looks at it
// (see Evaluate), and a site whose media are symlinked from another site
// can have thousands, which would fill the baseline.
func recordSymlink(rel string, uid, ownerUID uint32) bool {
	return isTracked(rel) || isStrayFile(rel) || (uid != ownerUID && uid != 0)
}

// checkContent sets the content rules a PHP file's body matches. A data
// file that exits first runs nothing, so it isn't searched.
func checkContent(e *Entry, body []byte) {
	if isGuarded(string(body[:min(len(body), headBytes)])) {
		return
	}
	if matches := matchContent(body); len(matches) > 0 {
		e.Content, e.ContentDetail = contentFlags(matches)
	}
}

// readTracked fills in a tracked file's hash, head and PHP directives, or
// marks it unreadable. With keepBody, it returns the file's first
// maxContentBytes for the content rules.
func readTracked(full, rel string, e *Entry, keepBody bool) []byte {
	// O_NOFOLLOW and O_NONBLOCK keep a file swapped for a symlink or a FIFO
	// since the walk saw it from being followed or blocking the walk.
	fd, err := syscall.Open(full, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		e.Unreadable = true
		return nil
	}
	f := os.NewFile(uintptr(fd), full)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		e.Unreadable = true
		return nil
	}
	e.Size = info.Size()
	if e.Size > maxHashBytes {
		// Too large to hash, but its start still shows what it is.
		start := make([]byte, max(headBytes, maxContentBytes))
		n, _ := io.ReadFull(f, start)
		if !hasSecrets(rel) {
			e.Head = strings.ToValidUTF8(string(start[:min(n, headBytes)]), "\uFFFD")
		}
		if keepBody {
			return start[:n]
		}
		return nil
	}

	h := sha256.New()
	var head, ini, body bytes.Buffer
	w := io.MultiWriter(h, &cappedWriter{buf: &head, max: headBytes})
	if isPHPIni(rel) {
		w = io.MultiWriter(w, &cappedWriter{buf: &ini, max: maxIniBytes})
	}
	if keepBody {
		w = io.MultiWriter(w, &cappedWriter{buf: &body, max: maxContentBytes})
	}
	if _, err := io.Copy(w, io.LimitReader(f, maxHashBytes)); err != nil {
		e.Unreadable = true
		return nil
	}
	e.SHA256 = hex.EncodeToString(h.Sum(nil))
	if !hasSecrets(rel) {
		e.Head = strings.ToValidUTF8(head.String(), "�")
	}
	if isPHPIni(rel) {
		e.Directives = phpDirectives(ini.Bytes())
	}
	return body.Bytes()
}

// cappedWriter keeps the first max bytes written to it.
type cappedWriter struct {
	buf *bytes.Buffer
	max int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		if room > len(p) {
			room = len(p)
		}
		c.buf.Write(p[:room])
	}
	return len(p), nil
}

// phpDirectives returns the lines of a .user.ini or .htaccess that set
// auto_prepend_file or auto_append_file to a file.
func phpDirectives(content []byte) []string {
	var found []string
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 64<<10), maxIniBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		lower := strings.ToLower(line)
		for _, key := range []string{"auto_prepend_file", "auto_append_file"} {
			i := strings.Index(lower, key)
			if i < 0 {
				continue
			}
			value := strings.Trim(lower[i+len(key):], " \t=\"'")
			if value == "" || value == "none" {
				continue
			}
			if len(line) > maxDirectiveLength {
				line = line[:maxDirectiveLength]
			}
			found = append(found, strings.ToValidUTF8(line, "�"))
			break
		}
	}
	return found
}
