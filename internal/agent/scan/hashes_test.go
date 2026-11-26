package scan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// trustRecentFiles lets a test keep the hashes of the files it just wrote.
func trustRecentFiles(t *testing.T) {
	t.Helper()
	old := racyWindow
	racyWindow = -time.Hour
	t.Cleanup(func() { racyWindow = old })
}

func TestWalkReusesCoreHashes(t *testing.T) {
	trustRecentFiles(t)
	root := t.TempDir()
	sums := coreSite(t, root)
	owner := uint32(os.Getuid())

	res, err := Walk(root, owner, Options{Core: sums})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := res.Hashes["wp-includes/load.php"]
	if !ok || h.MD5 != sums.MD5["wp-includes/load.php"] || h.Ino == 0 || h.Ctime == 0 {
		t.Fatalf("hashes = %+v", res.Hashes)
	}

	// An unchanged file isn't read again: its cached hash decides.
	tampered := cloneHashes(res.Hashes)
	h.MD5 = "00000000000000000000000000000000"
	tampered["wp-includes/load.php"] = h
	again, _ := Walk(root, owner, Options{Core: sums, Hashes: tampered})
	if got := coreFindings(again.Entries); !slices.Equal(got, []string{"core-modified wp-includes/load.php"}) {
		t.Fatalf("with a planted hash: %q", got)
	}

	// An edit that keeps the size and puts the mtime back still changes
	// the ctime, so the file is read again and flagged.
	full := filepath.Join(root, "wp-includes/load.php")
	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	writeFile(t, root, "wp-includes/load.php", "<?php evil();")
	if err := os.Chtimes(full, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if n, _ := os.Stat(full); n.Size() != info.Size() {
		t.Fatalf("test edit changed the size: %d vs %d", n.Size(), info.Size())
	}
	edited, _ := Walk(root, owner, Options{Core: sums, Hashes: res.Hashes})
	if got := coreFindings(edited.Entries); !slices.Equal(got, []string{"core-modified wp-includes/load.php"}) {
		t.Fatalf("after an edit hidden from the mtime: %q", got)
	}
}

func TestWalkReusesPluginHashes(t *testing.T) {
	trustRecentFiles(t)
	root := t.TempDir()
	owner := uint32(os.Getuid())
	good := "<?php // notice"
	writeFile(t, root, "wp-content/plugins/akismet/akismet.php", "<?php\n/*\nPlugin Name: Akismet\nVersion: 5.3\n*/")
	writeFile(t, root, "wp-content/plugins/akismet/views/notice.php", "<?php eval($_POST['x']); // notice")
	writeFile(t, root, "wp-content/plugins/akismet/views/ok.php", good)
	sums := &PluginChecksums{Slug: "akismet", Version: "5.3", SHA256: map[string][]string{
		"akismet.php":      {sha256Of("<?php\n/*\nPlugin Name: Akismet\nVersion: 5.3\n*/")},
		"views/notice.php": {sha256Of(good)},
		"views/ok.php":     {sha256Of(good)},
	}}
	opts := Options{Plugins: map[string]*PluginChecksums{"akismet": sums}}

	res, err := Walk(root, owner, opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"plugin-modified wp-content/plugins/akismet/views/notice.php"}
	if got := coreFindings(res.Entries); !slices.Equal(got, want) {
		t.Fatalf("first walk: %q", got)
	}
	if len(res.Hashes) != 3 || res.Hashes["wp-content/plugins/akismet/views/ok.php"].SHA256 != sha256Of(good) {
		t.Fatalf("hashes = %+v", res.Hashes)
	}

	// With its real hash kept, the modified file is still flagged.
	opts.Hashes = res.Hashes
	if again, _ := Walk(root, owner, opts); !slices.Equal(coreFindings(again.Entries), want) || len(again.Hashes) != 3 {
		t.Fatalf("second walk: %q, %d hashes", coreFindings(again.Entries), len(again.Hashes))
	}

	// A kept hash that matches the checksums is trusted without reading
	// the file, which is why the hashes live where the site can't write.
	planted := cloneHashes(res.Hashes)
	h := planted["wp-content/plugins/akismet/views/notice.php"]
	h.SHA256 = sha256Of(good)
	planted["wp-content/plugins/akismet/views/notice.php"] = h
	opts.Hashes = planted
	if trusted, _ := Walk(root, owner, opts); len(coreFindings(trusted.Entries)) != 0 {
		t.Fatalf("with a planted hash: %q", coreFindings(trusted.Entries))
	}
}

func TestWalkSkipsHashesOfRecentChanges(t *testing.T) {
	root := t.TempDir()
	sums := coreSite(t, root)
	res, err := Walk(root, uint32(os.Getuid()), Options{Core: sums})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hashes) != 0 {
		t.Errorf("kept the hashes of files written just now: %+v", res.Hashes)
	}
}

func TestSaveAndLoadHashes(t *testing.T) {
	dir := t.TempDir()
	if h, err := LoadHashes(dir, "site-1"); err != nil || h != nil {
		t.Fatalf("no file: %v, %v", h, err)
	}
	want := map[string]FileHash{"wp-load.php": {Dev: 1, Ino: 2, Size: 3, Mtime: 4, Ctime: 5, MD5: "abc"}}
	if err := SaveHashes(dir, "site-1", want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHashes(dir, "site-1")
	if err != nil || got["wp-load.php"] != want["wp-load.php"] {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "scan", "site-1.hashes.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, %v; want 0600", info.Mode(), err)
	}
	if err := SaveHashes(dir, "../x", want); err == nil {
		t.Error("saved under an invalid site ID")
	}
}

func cloneHashes(h map[string]FileHash) map[string]FileHash {
	out := make(map[string]FileHash, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}
