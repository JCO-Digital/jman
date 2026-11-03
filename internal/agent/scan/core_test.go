package scan

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func md5Of(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

const versionPHP = "<?php\n$wp_version = '6.8.3';\n$wp_db_version = 60421;\n$wp_local_package = 'fi';\n"

// coreSite writes a small WordPress install and returns its checksums.
func coreSite(t *testing.T, root string) *CoreChecksums {
	t.Helper()
	files := map[string]string{
		"index.php":                     "<?php require 'wp-blog-header.php';",
		"wp-blog-header.php":            "<?php // header",
		"wp-includes/version.php":       versionPHP,
		"wp-includes/load.php":          "<?php // load",
		"wp-admin/index.php":            "<?php // admin",
		"wp-admin/css/about.css":        "body{}",
		"wp-content/themes/x/style.css": "/* theme */",
	}
	sums := map[string]string{}
	for p, content := range files {
		writeFile(t, root, p, content)
		if p != "wp-content/themes/x/style.css" {
			sums[p] = md5Of(content)
		}
	}
	return &CoreChecksums{Version: "6.8.3", Locale: "fi", MD5: sums}
}

// coreFindings lists the entries with a checksum rule, core's or a
// plugin's.
func coreFindings(entries []Entry) []string {
	var got []string
	for _, e := range entries {
		if e.Checksum != "" {
			got = append(got, e.Checksum+" "+e.Path)
		}
	}
	slices.Sort(got)
	return got
}

func TestWalkChecksCore(t *testing.T) {
	root := t.TempDir()
	sums := coreSite(t, root)
	owner := uint32(os.Getuid())

	// Without checksums, only the version is read.
	res, err := Walk(root, owner, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.CoreVersion != "6.8.3" || res.CoreLocale != "fi" || res.CoreChecked || len(coreFindings(res.Entries)) != 0 {
		t.Fatalf("walk without checksums = %+v", res)
	}

	writeFile(t, root, "wp-includes/load.php", "<?php @eval($_POST['x']); // load")
	writeFile(t, root, "wp-includes/class-wp-cache.php", "<?php system($_GET['c']);")
	writeFile(t, root, "wp-includes/images/favicon.ico", "<?php // ico")
	writeFile(t, root, "radio.php", "<?php // shell")
	writeFile(t, root, "wp-config.php", "<?php define('DB_NAME', 'x');")
	writeFile(t, root, "google1234.html", "google-site-verification")
	writeFile(t, root, "wp-content/plugins/y/y.php", "<?php // plugin")

	res, err = Walk(root, owner, Options{Core: sums})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"core-modified wp-includes/load.php",
		"core-unknown-file radio.php",
		"core-unknown-file wp-includes/class-wp-cache.php",
		"core-unknown-file wp-includes/images/favicon.ico",
	}
	if got := coreFindings(res.Entries); !res.CoreChecked || !slices.Equal(got, want) {
		t.Fatalf("core entries = %q (checked %t), want %q", got, res.CoreChecked, want)
	}

	// Flagged core files are reported like tracked ones, with a hash and a
	// snippet; a non-PHP unknown file is medium.
	findings := EvaluateFirst(res.Entries, owner, func(uint32) string { return "u" })
	var got []string
	for _, f := range findings {
		if f.SHA256 == "" || f.Snippet == "" {
			t.Errorf("finding without hash or snippet: %+v", f)
		}
		got = append(got, f.Severity+" "+f.Rule+" "+f.Path)
	}
	slices.Sort(got)
	wantFindings := []string{
		"high core-modified wp-includes/load.php",
		"high core-unknown-file radio.php",
		"high core-unknown-file wp-includes/class-wp-cache.php",
		"high suspicious-code wp-includes/class-wp-cache.php",
		"high suspicious-code wp-includes/load.php",
		"medium core-unknown-file wp-includes/images/favicon.ico",
	}
	if !slices.Equal(got, wantFindings) {
		t.Fatalf("findings = %q, want %q", got, wantFindings)
	}

	// Unchanged, they aren't flagged again.
	prev := NewState(res.Entries)
	again, _ := Walk(root, owner, Options{Core: sums})
	if f := Evaluate(prev.Entries, again.Entries, owner, func(uint32) string { return "u" }); len(f) != 0 {
		t.Fatalf("flagged again: %+v", f)
	}

	// Restoring the core file clears its finding.
	writeFile(t, root, "wp-includes/load.php", "<?php // load")
	fixed, _ := Walk(root, owner, Options{Core: sums})
	cleared := Cleared(prev.Entries, fixed.Entries, owner, true, func(string) bool { return true })
	if len(cleared) != 1 || cleared[0].Path != "wp-includes/load.php" || cleared[0].Detail != ClearedFixed {
		t.Fatalf("cleared = %+v", cleared)
	}
}

// Checksums for another version, or a site in the middle of an update,
// leave the core rules out, and the previous core findings are kept.
func TestWalkSkipsCoreMidUpdate(t *testing.T) {
	root := t.TempDir()
	sums := coreSite(t, root)
	owner := uint32(os.Getuid())
	writeFile(t, root, "wp-includes/load.php", "<?php // hacked")
	res, _ := Walk(root, owner, Options{Core: sums})
	prev := NewState(res.Entries)

	other := *sums
	other.Version = "6.8.2"
	res, _ = Walk(root, owner, Options{Core: &other})
	if res.CoreChecked || res.CoreNote == "" || len(coreFindings(res.Entries)) != 0 {
		t.Fatalf("walk with another version's checksums = %+v", res)
	}
	CarryChecksums(prev, res)
	if got := coreFindings(res.Entries); !slices.Equal(got, []string{"core-modified wp-includes/load.php"}) {
		t.Fatalf("carried = %q", got)
	}
	if f := Evaluate(prev.Entries, res.Entries, owner, func(uint32) string { return "u" }); len(f) != 0 {
		t.Fatalf("carried entries flagged: %+v", f)
	}

	many := &CoreChecksums{Version: "6.8.3", Locale: "fi", MD5: map[string]string{}}
	for i := range maxCoreModified + 1 {
		p := fmt.Sprintf("wp-includes/f%d.php", i)
		writeFile(t, root, p, "<?php // new release")
		many.MD5[p] = md5Of("<?php // old release")
	}
	res, _ = Walk(root, owner, Options{Core: many})
	if res.CoreChecked || len(coreFindings(res.Entries)) != 0 {
		t.Fatalf("walk mid-update = checked %t, %q", res.CoreChecked, coreFindings(res.Entries))
	}
}

// wordpress.org's 7.1.4 tarball keeps these two names in pax headers;
// unpacked by PharData, both land under their first 100 bytes.
const (
	speechModel     = "wp-includes/php-ai-client/src/Providers/Models/SpeechGeneration/Contracts/SpeechGenerationModelInterface.php"
	speechOperation = "wp-includes/php-ai-client/src/Providers/Models/SpeechGeneration/Contracts/SpeechGenerationOperationModelInterface.php"
	speechCut       = "wp-includes/php-ai-client/src/Providers/Models/SpeechGeneration/Contracts/SpeechGeneration"
)

func TestCoreRuleTruncated(t *testing.T) {
	if len("wordpress/"+speechCut) != 100 {
		t.Fatalf("fixture is %d bytes with wordpress/, want 100", len("wordpress/"+speechCut))
	}
	sums := map[string]string{
		speechModel:            md5Of("<?php interface SpeechGenerationModelInterface {}"),
		speechOperation:        md5Of("<?php interface SpeechGenerationOperationModelInterface {}"),
		"wp-includes/load.php": md5Of("<?php // load"),
	}
	hash := func(s string) func() (string, bool) { return func() (string, bool) { return md5Of(s), true } }
	for _, c := range []struct {
		name, path string
		readMD5    func() (string, bool)
		want       string
	}{
		{"last of the colliding files", speechCut, hash("<?php interface SpeechGenerationOperationModelInterface {}"), RuleCoreTruncated},
		{"the other one", speechCut, hash("<?php interface SpeechGenerationModelInterface {}"), RuleCoreTruncated},
		{"other content under the cut name", speechCut, hash("<?php eval($_POST['x']);"), RuleCoreUnknown},
		{"unreadable or a symlink", speechCut, func() (string, bool) { return "", false }, RuleCoreUnknown},
		{"shorter prefix of a core path", speechCut[:89], hash("<?php interface SpeechGenerationModelInterface {}"), RuleCoreUnknown},
		{"90 bytes, but no core path starts with it", "wp-includes/" + strings.Repeat("x", truncatedNameLen-len("wp-includes/")), hash("<?php // load"), RuleCoreUnknown},
	} {
		if got := coreRule(c.path, sums, c.readMD5); got != c.want {
			t.Errorf("%s: coreRule = %q, want %q", c.name, got, c.want)
		}
	}
}

// A site unpacked by PharData: the two interfaces are one file under the
// cut-short name, holding the second of them. It's reported as low, and
// clears once core is reinstalled.
func TestWalkTruncatedCore(t *testing.T) {
	root := t.TempDir()
	sums := coreSite(t, root)
	model := "<?php interface SpeechGenerationModelInterface {}"
	operation := "<?php interface SpeechGenerationOperationModelInterface {}"
	sums.MD5[speechModel] = md5Of(model)
	sums.MD5[speechOperation] = md5Of(operation)
	writeFile(t, root, speechCut, operation)
	owner := uint32(os.Getuid())

	res, err := Walk(root, owner, Options{Core: sums})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range EvaluateFirst(res.Entries, owner, func(uint32) string { return "u" }) {
		got = append(got, f.Severity+" "+f.Rule+" "+f.Path)
	}
	if want := []string{"low core-truncated " + speechCut}; !slices.Equal(got, want) {
		t.Fatalf("findings = %q, want %q", got, want)
	}

	// Reinstalling core puts both files back and removes the cut one.
	prev := NewState(res.Entries)
	if err := os.Remove(root + "/" + speechCut); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, speechModel, model)
	writeFile(t, root, speechOperation, operation)
	fixed, _ := Walk(root, owner, Options{Core: sums})
	if f := Evaluate(prev.Entries, fixed.Entries, owner, func(uint32) string { return "u" }); len(f) != 0 {
		t.Fatalf("findings after reinstall: %+v", f)
	}
	cleared := Cleared(prev.Entries, fixed.Entries, owner, true, func(string) bool { return true })
	if len(cleared) != 1 || cleared[0].Path != speechCut {
		t.Fatalf("cleared = %+v", cleared)
	}
}
