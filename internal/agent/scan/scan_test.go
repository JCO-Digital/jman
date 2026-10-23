package scan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func byPath(entries []Entry) map[string]Entry {
	m := make(map[string]Entry, len(entries))
	for _, e := range entries {
		m[e.Path] = e
	}
	return m
}

// newSite builds a small WordPress tree with one of everything the walk
// should record, and some files it should ignore.
func newSite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "wp-config.php", "<?php define('DB_PASSWORD', 'secret');")
	writeFile(t, root, ".htaccess", "# BEGIN WordPress\nphp_value auto_prepend_file /tmp/x.php\n")
	writeFile(t, root, "index.php", "<?php // ignored")
	writeFile(t, root, "wp-content/plugins/foo/foo.php", "<?php // ignored")
	writeFile(t, root, "wp-content/uploads/2026/10/photo.jpg", "jpeg")
	writeFile(t, root, "wp-content/uploads/2026/10/shell.php", "<?php eval($_POST['x']);")
	writeFile(t, root, "wp-content/uploads/index.php", "")
	writeFile(t, root, "wp-content/mu-plugins/loader.php", "<?php // mu")
	writeFile(t, root, "wp-content/object-cache.php", "<?php // cache")
	writeFile(t, root, "wp-content/themes/x/.user.ini", "; auto_prepend_file = /commented.php\nauto_prepend_file = none\nauto_append_file=\"/tmp/a.php\"\n")
	writeFile(t, root, "backup.sql.gz", "dump")
	writeFile(t, root, ".git/HEAD", "ref: refs/heads/main")
	return root
}

func TestWalkRecordsWhatTheRulesNeed(t *testing.T) {
	root := newSite(t)
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "wp-content/uploads/link.php")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "wp-content/uploads/fifo.php"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Walk(root, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(res.Entries)

	want := []string{
		"wp-config.php", ".htaccess",
		"wp-content/uploads/2026/10/shell.php", "wp-content/uploads/index.php",
		"wp-content/uploads/link.php", "wp-content/uploads/fifo.php",
		"wp-content/mu-plugins/loader.php", "wp-content/object-cache.php",
		"wp-content/themes/x/.user.ini", "backup.sql.gz", ".git",
	}
	for _, p := range want {
		if _, ok := got[p]; !ok {
			t.Errorf("missing %s", p)
		}
	}
	if len(got) != len(want) {
		var paths []string
		for p := range got {
			paths = append(paths, p)
		}
		slices.Sort(paths)
		t.Errorf("recorded %d entries, want %d: %v", len(got), len(want), paths)
	}
	if _, ok := got[".git/HEAD"]; ok {
		t.Error("walk descended into .git")
	}

	shell := got["wp-content/uploads/2026/10/shell.php"]
	if shell.SHA256 == "" || !strings.Contains(shell.Head, "eval") {
		t.Errorf("shell.php = %+v, want hash and head", shell)
	}
	if cfg := got["wp-config.php"]; cfg.SHA256 == "" || cfg.Head != "" {
		t.Errorf("wp-config.php = %+v, want a hash and no head", cfg)
	}
	if link := got["wp-content/uploads/link.php"]; link.Symlink != "/etc/passwd" || link.SHA256 != "" {
		t.Errorf("link.php = %+v, want a symlink that isn't followed", link)
	}
	if fifo := got["wp-content/uploads/fifo.php"]; fifo.SHA256 != "" {
		t.Errorf("fifo.php = %+v, want no hash", fifo)
	}
	if d := got[".htaccess"].Directives; len(d) != 1 || !strings.Contains(d[0], "auto_prepend_file") {
		t.Errorf(".htaccess directives = %q", d)
	}
	if d := got["wp-content/themes/x/.user.ini"].Directives; len(d) != 1 || !strings.Contains(d[0], "auto_append_file") {
		t.Errorf(".user.ini directives = %q, want only the auto_append_file line", d)
	}
}

func TestWalkMarksUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := newSite(t)
	locked := filepath.Join(root, "wp-content/uploads/locked")
	writeFile(t, root, "wp-content/uploads/locked/x.php", "<?php")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	if err := os.Chmod(filepath.Join(root, "wp-content/mu-plugins/loader.php"), 0); err != nil {
		t.Fatal(err)
	}

	res, err := Walk(root, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(res.Entries)
	if e := got["wp-content/uploads/locked"]; !e.Unreadable || !e.Dir {
		t.Errorf("locked dir = %+v, want unreadable", e)
	}
	if e := got["wp-content/mu-plugins/loader.php"]; !e.Unreadable {
		t.Errorf("loader.php = %+v, want unreadable", e)
	}
}

func TestWalkRecordsForeignOwnedFiles(t *testing.T) {
	root := newSite(t)
	// Every file is owned by the test's user, so pretend the site belongs
	// to someone else: everything becomes foreign.
	res, err := Walk(root, uint32(os.Getuid())+1)
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(res.Entries)
	for _, p := range []string{"index.php", "wp-content/plugins/foo/foo.php", "wp-content/uploads"} {
		if _, ok := got[p]; !ok {
			t.Errorf("foreign %s not recorded", p)
		}
	}
}

// evaluate returns the findings as sorted "rule path" strings.
func evaluate(prev map[string]Entry, cur []Entry, owner uint32) []string {
	var out []string
	for _, f := range Evaluate(prev, cur, owner, func(uint32) string { return "u" }) {
		out = append(out, f.Rule+" "+f.Path)
	}
	slices.Sort(out)
	return out
}

func TestEvaluateFlagsChangesOnly(t *testing.T) {
	root := newSite(t)
	owner := uint32(os.Getuid())
	first, err := Walk(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	baseline := NewState(first.Entries)

	// Against an empty baseline everything is new.
	all := evaluate(map[string]Entry{}, first.Entries, owner)
	for _, want := range []string{
		"uploads-php wp-content/uploads/2026/10/shell.php",
		"mu-plugin-change wp-content/mu-plugins/loader.php",
		"dropin-change wp-content/object-cache.php",
		"config-change wp-config.php",
		"config-change .htaccess",
		"auto-prepend .htaccess",
		"auto-prepend wp-content/themes/x/.user.ini",
		"stray-file backup.sql.gz",
		"stray-file .git",
	} {
		if !slices.Contains(all, want) {
			t.Errorf("against an empty baseline, missing %q in %q", want, all)
		}
	}

	// Nothing changed: nothing to flag.
	again, _ := Walk(root, owner)
	if got := evaluate(baseline.Entries, again.Entries, owner); len(got) != 0 {
		t.Errorf("unchanged site flagged %q", got)
	}
	if Fingerprint(again.Entries) != baseline.Fingerprint {
		t.Error("fingerprint changed for an unchanged site")
	}

	// A changed upload, a new mu-plugin and a new dump.
	writeFile(t, root, "wp-content/uploads/index.php", "<?php system($_GET['c']);")
	writeFile(t, root, "wp-content/mu-plugins/new.php", "<?php")
	writeFile(t, root, "db.sql", "dump")
	writeFile(t, root, "index.php", "<?php // changed, but not tracked")
	changed, _ := Walk(root, owner)
	want := []string{
		"mu-plugin-change wp-content/mu-plugins/new.php",
		"stray-file db.sql",
		"uploads-php wp-content/uploads/index.php",
	}
	if got := evaluate(baseline.Entries, changed.Entries, owner); !slices.Equal(got, want) {
		t.Errorf("flagged %q, want %q", got, want)
	}
	if Fingerprint(changed.Entries) == baseline.Fingerprint {
		t.Error("fingerprint unchanged after changes")
	}
}

func TestEvaluateForeignOwnerOnlyWhenOwnerChanges(t *testing.T) {
	prev := map[string]Entry{
		"index.php": {Path: "index.php", UID: 33},
	}
	cur := []Entry{
		{Path: "index.php", UID: 33},      // already foreign
		{Path: "wp-load.php", UID: 0},     // newly foreign
		{Path: "wp-settings.php", UID: 1}, // the owner's
	}
	want := []string{"foreign-owner wp-load.php"}
	if got := evaluate(prev, cur, 1); !slices.Equal(got, want) {
		t.Errorf("flagged %q, want %q", got, want)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if s, err := LoadState(dir, "site-1"); err != nil || s != nil {
		t.Fatalf("LoadState with no file = %v, %v", s, err)
	}
	s := NewState([]Entry{{Path: "a.php", SHA256: "abc", Head: "<?php"}})
	if err := SaveState(dir, "site-1", s); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "scan", "site-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("state file mode = %o, want 600", info.Mode().Perm())
	}
	loaded, err := LoadState(dir, "site-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fingerprint != s.Fingerprint || loaded.Entries["a.php"].SHA256 != "abc" || loaded.Entries["a.php"].Head != "" {
		t.Errorf("loaded = %+v", loaded)
	}
	if _, err := LoadState(dir, "../escape"); err == nil {
		t.Error("LoadState accepted a site ID with a path in it")
	}
}

// Patterns from a real server that aren't worth flagging, or not as high.
func TestEvaluateSkipsKnownNoise(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "wp-content/mu-plugins/acf/acf.php", "<?php // bundled plugin")
	writeFile(t, root, "wp-content/uploads/wpallimport/index.php", "")
	writeFile(t, root, "wp-content/languages/loco/x-fi-backup.po~", "msgid")
	writeFile(t, root, "wp-content/uploads/cache/wpml/twig/01/abc.php", "<?php // compiled")
	writeFile(t, root, "wp-content/uploads/sites/2/cache/wpml/twig/02/def.php", "<?php // compiled")
	writeFile(t, root, "wp-content/uploads/2020/06/logos.zip", "PK")
	writeFile(t, root, "wp-content/uploads/db.sql.zip", "PK")
	owner := uint32(os.Getuid())
	res, err := Walk(root, owner)
	if err != nil {
		t.Fatal(err)
	}
	// SpinupWP's root-owned debug.log symlink.
	cur := append(res.Entries, Entry{Path: "wp-content/debug.log", UID: 0, Symlink: "/sites/x/logs/debug.log"})

	findings := Evaluate(map[string]Entry{}, cur, owner, func(uint32) string { return "u" })
	var got []string
	for _, f := range findings {
		got = append(got, f.Severity+" "+f.Rule+" "+f.Path)
	}
	slices.Sort(got)
	want := []string{
		"low stray-file wp-content/uploads/db.sql.zip",
		"low uploads-php wp-content/uploads/cache/wpml/twig/01/abc.php",
		"low uploads-php wp-content/uploads/sites/2/cache/wpml/twig/02/def.php",
	}
	if !slices.Equal(got, want) {
		t.Errorf("flagged %q, want %q", got, want)
	}

	// An empty upload that gets content is flagged.
	baseline := NewState(res.Entries)
	writeFile(t, root, "wp-content/uploads/wpallimport/index.php", "<?php system($_GET['c']);")
	again, _ := Walk(root, owner)
	if got := evaluate(baseline.Entries, again.Entries, owner); !slices.Equal(got, []string{"uploads-php wp-content/uploads/wpallimport/index.php"}) {
		t.Errorf("after filling an empty upload, flagged %q", got)
	}
}
