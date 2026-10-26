package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"testing"
)

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestReadPluginVersions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "wp-content/plugins/akismet/akismet.php", "<?php\n/*\nPlugin Name: Akismet\nVersion: 5.3\nRequires PHP: 7.2\n*/")
	writeFile(t, root, "wp-content/plugins/akismet/class.akismet.php", "<?php // Version: 9")
	writeFile(t, root, "wp-content/plugins/custom/custom.php", "<?php\n/**\n * Plugin Name: Custom\n * Version:     1.2.0 */")
	writeFile(t, root, "wp-content/plugins/noversion/x.php", "<?php\n/* Plugin Name: X */")
	writeFile(t, root, "wp-content/plugins/hello.php", "<?php\n/* Plugin Name: Hello\nVersion: 1 */")
	writeFile(t, root, "wp-content/plugins/bsr/bsr.php", "<?php\n/* Plugin Name: BSR\nVersion: 1.4\nUpdate URI: false */")
	writeFile(t, root, "wp-content/plugins/w/w.php", "<?php\n/* Plugin Name: W\nVersion: 2\nUpdate URI: https://wordpress.org/plugins/w/ */")
	got, elsewhere := readPluginVersions(root)
	want := map[string]string{"akismet": "5.3", "custom": "1.2.0", "w": "2"}
	if len(got) != len(want) || got["akismet"] != "5.3" || got["custom"] != "1.2.0" || got["w"] != "2" {
		t.Fatalf("versions = %v, want %v", got, want)
	}
	if !slices.Equal(elsewhere, []string{"bsr (Update URI: false)"}) {
		t.Fatalf("elsewhere = %q", elsewhere)
	}
}

func TestWalkChecksPlugins(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Getuid())
	main := "<?php\n/*\nPlugin Name: Akismet\nVersion: 5.3\n*/"
	files := map[string]string{
		"akismet.php":       main,
		"class.akismet.php": "<?php class Akismet {}",
		"views/notice.php":  "<?php // notice",
		"_inc/akismet.js":   "var a;",
	}
	sums := &PluginChecksums{Slug: "akismet", Version: "5.3", SHA256: map[string][]string{}}
	for p, content := range files {
		writeFile(t, root, "wp-content/plugins/akismet/"+p, content)
		if IsPHPPath(p) {
			sums.SHA256[p] = []string{"0000", sha256Of(content)}
		}
	}
	writeFile(t, root, "wp-content/plugins/akismet/views/notice.php", "<?php eval($_POST['x']); // notice")
	writeFile(t, root, "wp-content/plugins/akismet/_inc/akismet.js", "var hacked;")
	writeFile(t, root, "wp-content/plugins/akismet/wp-load.php", "<?php system($_GET['c']);")
	writeFile(t, root, "wp-content/plugins/akismet/index.php", "")
	writeFile(t, root, "wp-content/plugins/akismet/cache.php", "<?php exit; ?>{\"a\":1}")
	writeFile(t, root, "wp-content/plugins/custom/custom.php", "<?php\n/* Plugin Name: Custom\nVersion: 1 */ eval($x);")

	opts := Options{Plugins: map[string]*PluginChecksums{"akismet": sums}}
	res, err := Walk(root, owner, opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"plugin-modified wp-content/plugins/akismet/views/notice.php",
		"plugin-unknown-file wp-content/plugins/akismet/wp-load.php",
	}
	if got := coreFindings(res.Entries); !slices.Equal(got, want) || !res.PluginsChecked["akismet"] || res.PluginsChecked["custom"] {
		t.Fatalf("plugin entries = %q, checked %v; want %q", got, res.PluginsChecked, want)
	}
	prev := NewState(res.Entries)
	prev.Plugins = res.Plugins

	// An update since: the plugin isn't checked, and its findings are kept.
	writeFile(t, root, "wp-content/plugins/akismet/akismet.php", "<?php\n/*\nPlugin Name: Akismet\nVersion: 5.4\n*/")
	res, _ = Walk(root, owner, opts)
	if res.PluginsChecked["akismet"] || len(res.PluginNotes) != 1 || len(coreFindings(res.Entries)) != 0 {
		t.Fatalf("walk after an update = %+v", res)
	}
	CarryChecksums(prev, res)
	if got := coreFindings(res.Entries); !slices.Equal(got, want) {
		t.Fatalf("carried = %q", got)
	}

	// Once the plugin is gone, they aren't.
	if err := os.RemoveAll(root + "/wp-content/plugins/akismet"); err != nil {
		t.Fatal(err)
	}
	res, _ = Walk(root, owner, opts)
	CarryChecksums(prev, res)
	if got := coreFindings(res.Entries); len(got) != 0 {
		t.Fatalf("carried for a removed plugin = %q", got)
	}
}

// A build with many PHP files the release doesn't have is another build
// with the same slug, and left out.
func TestWalkSkipsOtherBuilds(t *testing.T) {
	root := t.TempDir()
	header := "<?php\n/* Plugin Name: SM Pro\nVersion: 8 */"
	writeFile(t, root, "wp-content/plugins/sm/sm.php", header)
	sums := &PluginChecksums{Slug: "sm", Version: "8", SHA256: map[string][]string{"sm.php": {sha256Of(header)}}}
	for i := range maxPluginUnknown + 1 {
		writeFile(t, root, fmt.Sprintf("wp-content/plugins/sm/pro/f%d.php", i), "<?php // pro")
	}
	res, _ := Walk(root, uint32(os.Getuid()), Options{Plugins: map[string]*PluginChecksums{"sm": sums}})
	if res.PluginsChecked["sm"] || len(coreFindings(res.Entries)) != 0 || len(res.PluginNotes) != 1 {
		t.Fatalf("walk of another build = %+v", res)
	}
}

// A plugin in the middle of an update is left out rather than flagged.
func TestWalkSkipsPluginMidUpdate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "wp-content/plugins/big/big.php", "<?php\n/* Plugin Name: Big\nVersion: 2 */")
	sums := &PluginChecksums{Slug: "big", Version: "2", SHA256: map[string][]string{"big.php": {sha256Of("<?php\n/* Plugin Name: Big\nVersion: 2 */")}}}
	for i := range maxPluginModified + 1 {
		p := fmt.Sprintf("inc/f%d.php", i)
		writeFile(t, root, "wp-content/plugins/big/"+p, "<?php // new")
		sums.SHA256[p] = []string{sha256Of("<?php // old")}
	}
	res, _ := Walk(root, uint32(os.Getuid()), Options{Plugins: map[string]*PluginChecksums{"big": sums}})
	if res.PluginsChecked["big"] || len(coreFindings(res.Entries)) != 0 || len(res.PluginNotes) != 1 {
		t.Fatalf("walk mid-update = %+v", res)
	}
}
