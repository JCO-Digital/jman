package scan

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
}

// Result is what a walk found.
type Result struct {
	Entries []Entry `json:"entries"`
	// Omitted counts files left out because the caps were reached.
	Omitted int `json:"omitted,omitempty"`
}

// Walk looks through the site directory root and records the files that
// matter to the rules: tracked files (see isTracked) with their hashes,
// stray files, and anything not owned by ownerUID or that can't be read.
// It never follows symlinks and opens only regular files.
func Walk(root string, ownerUID uint32) (*Result, error) {
	res := &Result{Entries: []Entry{}}
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

		switch {
		case isTracked(rel):
			if info.Mode().IsRegular() {
				readTracked(full, rel, &e)
			}
			add(e, true)
		case isStrayFile(rel) || foreign:
			add(e, false)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// recordSymlink reports whether the walk records a symlink. A root-owned
// one is recorded only if it's tracked or stray: no other rule looks at it
// (see Evaluate), and a site whose media are symlinked from another site
// can have thousands, which would fill the baseline.
func recordSymlink(rel string, uid, ownerUID uint32) bool {
	return isTracked(rel) || isStrayFile(rel) || (uid != ownerUID && uid != 0)
}

// readTracked fills in a tracked file's hash, head and PHP directives, or
// marks it unreadable.
func readTracked(full, rel string, e *Entry) {
	// O_NOFOLLOW and O_NONBLOCK keep a file swapped for a symlink or a FIFO
	// since the walk saw it from being followed or blocking the walk.
	fd, err := syscall.Open(full, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		e.Unreadable = true
		return
	}
	f := os.NewFile(uintptr(fd), full)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		e.Unreadable = true
		return
	}
	e.Size = info.Size()
	if e.Size > maxHashBytes {
		// Too large to hash, but its start still shows what it is.
		head := make([]byte, headBytes)
		n, _ := io.ReadFull(f, head)
		if !hasSecrets(rel) {
			e.Head = strings.ToValidUTF8(string(head[:n]), "\uFFFD")
		}
		return
	}

	h := sha256.New()
	var head bytes.Buffer
	var ini bytes.Buffer
	w := io.MultiWriter(h, &cappedWriter{buf: &head, max: headBytes})
	if isPHPIni(rel) {
		w = io.MultiWriter(w, &cappedWriter{buf: &ini, max: maxIniBytes})
	}
	if _, err := io.Copy(w, io.LimitReader(f, maxHashBytes)); err != nil {
		e.Unreadable = true
		return
	}
	e.SHA256 = hex.EncodeToString(h.Sum(nil))
	if !hasSecrets(rel) {
		e.Head = strings.ToValidUTF8(head.String(), "�")
	}
	if isPHPIni(rel) {
		e.Directives = phpDirectives(ini.Bytes())
	}
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
