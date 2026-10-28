package scan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// Persistence is how an attacker with the site user's shell keeps it: a
// crontab entry, a line in a shell startup file, a program in the home
// directory, a process disguised as a kernel thread. The site's files
// don't show any of it, so the scan checks them too: the home directory as
// the site user, the crontab and processes as root (the agent).

// HomePrefix starts the paths of entries in the site user's home directory,
// outside the site's files.
const HomePrefix = "~/"

// CrontabPath and ProcessPrefix are the paths of the site user's crontab
// and running processes in a scan.
const (
	CrontabPath   = "crontab"
	ProcessPrefix = "process/"
)

// startupFiles are the shell startup files in a home directory, which run
// at every login.
var startupFiles = map[string]bool{
	".profile": true, ".bashrc": true, ".bash_profile": true, ".bash_login": true,
	".bash_logout": true, ".zshrc": true, ".zprofile": true, ".zshenv": true, ".zlogin": true,
}

// editorServerDirs are where remote editors install their server in the
// home directory of the user a developer connects as (VS Code Remote-SSH,
// its Dev Containers, and forks such as Cursor and Windsurf). Their
// programs are legitimate and change with every editor update, so they're
// reported as low; a process named like a kernel thread is still high.
var editorServerDirs = []string{".vscode-server", ".vscode-server-insiders", ".vscode-remote-containers", ".cursor-server", ".windsurf-server"}

// inEditorServer reports whether a path, a home path ("~/…") or an
// absolute one, is inside an editor's server directory.
func inEditorServer(p string) bool {
	for _, dir := range editorServerDirs {
		if strings.HasPrefix(p, HomePrefix+dir+"/") || strings.Contains(p, "/"+dir+"/") {
			return true
		}
	}
	return false
}

// isKernelThreadName reports whether a process name looks like a kernel
// thread's, "[kworker/0:1]": a site user's process has no reason to.
func isKernelThreadName(name string) bool {
	return strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]")
}

// skipHomeDirs are directories of a home directory the walk doesn't enter:
// caches full of downloads, and the site's logs.
var skipHomeDirs = map[string]bool{
	"logs": true, ".cache": true, ".npm": true, ".wp-cli/cache": true,
	".composer/cache": true, ".local/share/Trash": true,
}

// Limits of the home walk.
const (
	maxHomeEntries   = 200
	maxHomeDepth     = 8
	maxStartupBytes  = 1 << 20
	maxBinaryHashing = 64 << 20
)

// persistPattern is shell that runs something hidden or fetched: what an
// attacker writes into a crontab or startup file, and an admin doesn't.
type persistPattern struct {
	name string
	re   *regexp.Regexp
}

var persistPatterns = []persistPattern{
	{"decoded-shell", regexp.MustCompile(`(?i)base64\s+(?:-d|--decode|-D)\b[^\n]*\|\s*(?:ba|z|da)?sh\b|\|\s*base64\s+(?:-d|--decode)[^\n]*\|\s*(?:ba|z|da)?sh\b`)},
	{"download-shell", regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^\n|]*\|\s*(?:ba|z|da)?sh\b`)},
	{"dev-tcp", regexp.MustCompile(`/dev/(?:tcp|udp)/`)},
	{"renamed-process", regexp.MustCompile(`\bexec\s+-a\s`)},
	{"gsocket", regexp.MustCompile(`(?i)defunct-kernel|\bGS_ARGS\b|gs-netcat|gsocket`)},
	{"reverse-shell", regexp.MustCompile(`(?i)\b(?:nc|ncat|netcat)\b[^\n]*\s-[ec]\s|\bsocat\b[^\n]*exec:`)},
	{"inline-script", regexp.MustCompile(`\b(?:python[23]?|perl|php|ruby)\s+-[cer]\s+['"]`)},
}

// matchPersistence returns the persistence patterns content matches and a
// detail quoting the first line of each.
func matchPersistence(content []byte) ([]string, string) {
	var names, lines []string
	for _, p := range persistPatterns {
		loc := p.re.FindIndex(content)
		if loc == nil {
			continue
		}
		start := bytes.LastIndexByte(content[:loc[0]], '\n') + 1
		end := loc[1] + bytes.IndexByte(content[loc[1]:], '\n')
		if end < loc[1] {
			end = len(content)
		}
		line := strings.TrimSpace(strings.ToValidUTF8(string(content[start:min(end, start+maxMatchExcerpt)]), "�"))
		names = append(names, p.name)
		lines = append(lines, fmt.Sprintf("%s (line %d): %s", p.name, bytes.Count(content[:start], []byte("\n"))+1, line))
	}
	return names, strings.Join(lines, "\n")
}

// WalkHome records what in the site user's home directory can keep an
// attacker in: its shell startup files, and executable programs (ELF)
// anywhere outside the site's files and caches. siteDir is the site's
// files, which Walk covers. It never follows symlinks.
func WalkHome(home, siteDir string) []Entry {
	var entries []Entry
	_ = filepath.WalkDir(home, func(full string, d fs.DirEntry, err error) error {
		if err != nil || full == home {
			return nil
		}
		rel, err := filepath.Rel(home, full)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if full == siteDir || skipHomeDirs[rel] || strings.Count(rel, "/") >= maxHomeDepth {
				return fs.SkipDir
			}
			return nil
		}
		if len(entries) >= maxHomeEntries || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		switch {
		case startupFiles[rel]:
			e := Entry{Path: HomePrefix + rel, Size: info.Size(), UID: fileUID(info)}
			if b, ok := readSmallFile(full, maxStartupBytes); ok {
				sum := sha256.Sum256(b)
				e.SHA256 = hex.EncodeToString(sum[:])
				e.Head = strings.ToValidUTF8(string(b[:min(len(b), headBytes)]), "�")
				e.Persist, e.PersistDetail = matchPersistence(b)
			} else {
				e.Unreadable = true
			}
			entries = append(entries, e)
		case info.Mode()&0o111 != 0 && isELF(full):
			e := Entry{Path: HomePrefix + rel, Size: info.Size(), UID: fileUID(info), Binary: true}
			e.SHA256, _ = hashFile(full, maxBinaryHashing)
			entries = append(entries, e)
		}
		return nil
	})
	return entries
}

func fileUID(info fs.FileInfo) uint32 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Uid
	}
	return 0
}

// isELF reports whether a file is an ELF program.
func isELF(full string) bool {
	b, ok := readSmallFile(full, 4)
	return ok && bytes.Equal(b, []byte("\x7fELF"))
}

// hashFile returns the SHA-256 of a regular file, without following a
// symlink, if it's at most max bytes.
func hashFile(full string, max int64) (string, bool) {
	fd, err := syscall.Open(full, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", false
	}
	f := os.NewFile(uintptr(fd), full)
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > max {
		return "", false
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// crontabDirs are where cron keeps users' crontabs: Debian and Ubuntu, then
// Red Hat.
var crontabDirs = []string{"/var/spool/cron/crontabs", "/var/spool/cron"}

// ReadCrontab returns an entry for a user's crontab, or false if the user
// has none. The agent reads it as root: the directory is root's, and the
// file isn't followed if it's a symlink.
func ReadCrontab(username string, uid uint32) (Entry, bool) {
	for _, dir := range crontabDirs {
		b, ok := readSmallFile(filepath.Join(dir, username), maxStartupBytes)
		if !ok {
			continue
		}
		sum := sha256.Sum256(b)
		e := Entry{Path: CrontabPath, Size: int64(len(b)), UID: uid, SHA256: hex.EncodeToString(sum[:])}
		e.Head = strings.ToValidUTF8(string(b[:min(len(b), headBytes)]), "�")
		e.Persist, e.PersistDetail = matchPersistence(b)
		return e, true
	}
	return Entry{}, false
}

// suspiciousExeDirs are where no program a site user runs belongs.
var suspiciousExeDirs = []string{"/tmp/", "/var/tmp/", "/dev/shm/"}

// procRoot is a seam for tests.
var procRoot = "/proc"

// UserProcesses returns an entry for each process of a user that hides
// what it is: named like a kernel thread ("[kaluad]"), or running a program
// from the home directory, a temporary directory, or one that was deleted.
// The agent reads them as root.
func UserProcesses(uid uint32, home string) []Entry {
	dirs, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	var entries []Entry
	seen := map[string]bool{}
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		p := filepath.Join(procRoot, d.Name())
		if procUID(p) != int64(uid) {
			continue
		}
		cmdline, _ := os.ReadFile(filepath.Join(p, "cmdline"))
		argv0, _, _ := strings.Cut(string(cmdline), "\x00")
		exe, _ := os.Readlink(filepath.Join(p, "exe"))
		disguised := isKernelThreadName(argv0)
		hidden := strings.HasSuffix(exe, " (deleted)") || (home != "" && strings.HasPrefix(exe, strings.TrimSuffix(home, "/")+"/"))
		for _, dir := range suspiciousExeDirs {
			hidden = hidden || strings.HasPrefix(exe, dir)
		}
		if !disguised && !hidden {
			continue
		}
		name := argv0
		if name == "" {
			name = exe
		}
		path := ProcessPrefix + truncateName(name)
		if seen[path] {
			continue
		}
		seen[path] = true
		e := Entry{Path: path, UID: uid, Process: true, Exe: exe}
		e.SHA256, _ = hashProcessExe(filepath.Join(p, "exe"))
		e.PersistDetail = fmt.Sprintf("pid %d running %s: %s", pid, orUnknown(exe), truncateName(strings.ReplaceAll(strings.TrimRight(string(cmdline), "\x00"), "\x00", " ")))
		entries = append(entries, e)
	}
	return entries
}

// hashProcessExe returns the SHA-256 of the program a process runs.
// /proc/<pid>/exe is a link the kernel keeps to it, even once deleted, so
// unlike a file in a site it's followed.
func hashProcessExe(link string) (string, bool) {
	f, err := os.Open(link)
	if err != nil {
		return "", false
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() || info.Size() > maxBinaryHashing {
		return "", false
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// procUID returns a process's real user ID, or -1.
func procUID(p string) int64 {
	b, err := os.ReadFile(filepath.Join(p, "status"))
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				if n, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
					return n
				}
			}
		}
	}
	return -1
}

func truncateName(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func orUnknown(s string) string {
	if s == "" {
		return "an unknown program"
	}
	return s
}

// errNotUserPath is returned by UserPathExists for paths it can't check.
var errNotUserPath = errors.New("not a file path")

// UserPathExists reports whether a scan path outside the site's files still
// exists: a home path under home, the crontab in cron's directory. A
// process path is never checked: a process that's gone is gone.
func UserPathExists(p, home, username string) (bool, error) {
	switch {
	case strings.HasPrefix(p, HomePrefix):
		_, err := os.Lstat(filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(p, HomePrefix))))
		return err == nil, nil
	case p == CrontabPath:
		_, ok := ReadCrontab(username, 0)
		return ok, nil
	}
	return false, errNotUserPath
}

// IsUserPath reports whether a scan path is outside the site's files: in
// the home directory, the crontab or a process.
func IsUserPath(p string) bool {
	return strings.HasPrefix(p, HomePrefix) || p == CrontabPath || strings.HasPrefix(p, ProcessPrefix)
}
