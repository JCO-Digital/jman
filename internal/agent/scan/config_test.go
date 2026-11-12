package scan

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

const testConfig = `<?php
define( 'DB_NAME', 'site_db' );
define( 'DB_USER', 'site_user' );
define( 'DB_PASSWORD', 'hunter2-but-longer' );
define( 'DB_HOST', 'localhost' );
define( 'AUTH_KEY',         'x+3Kq!vZ@9pL#2mN$7rT%4wY^1bH&6cJ*8dF(0gS)5aE-' );
define( 'SECURE_AUTH_SALT',
	'multi line value' );
define( 'STRIPE_SK', 'Zq8Lw3Vt1Nx6Kp4Rm9Hs2Jd7Bf' );
putenv( 'MAILGUN_KEY=plainvalue' );
$table_prefix = 'wp_';
$db_pass = "short";
define( 'WP_DEBUG', false );
require_once ABSPATH . 'wp-settings.php';
`

// secretValues are the values in testConfig that must not be kept.
var secretValues = []string{"hunter2-but-longer", "x+3Kq", "multi line value", "Zq8Lw3Vt1Nx6Kp4Rm9Hs2Jd7Bf", "plainvalue", "short"}

func TestConfigCopyRedactsSecrets(t *testing.T) {
	c := newConfigCopy("wp-config.php", []byte(testConfig), int64(len(testConfig)))
	b, _ := json.Marshal(c)
	for _, v := range secretValues {
		if strings.Contains(string(b), v) {
			t.Errorf("copy keeps %q:\n%s", v, strings.Join(c.Lines, "\n"))
		}
	}
	for _, keep := range []string{"'site_db'", "'localhost'", "'wp_'", "define( 'WP_DEBUG', false );", "'wp-settings.php'", "define( 'DB_PASSWORD', '[redacted]' );"} {
		if !strings.Contains(strings.Join(c.Lines, "\n"), keep) {
			t.Errorf("copy lost %q:\n%s", keep, strings.Join(c.Lines, "\n"))
		}
	}
	for _, name := range []string{"DB_PASSWORD", "AUTH_KEY", "SECURE_AUTH_SALT", "MAILGUN_KEY", "db_pass"} {
		if c.Secrets[name] == "" {
			t.Errorf("no hash for secret %s", name)
		}
	}
}

func TestConfigCopyRedactsHtaccess(t *testing.T) {
	ht := "SetEnv API_TOKEN abc def\nRewriteRule ^x$ /y [L]\nHeader set X-Id 9fK2pQx7Lm4Rz8Tw1Vb6Nc3Hd5Js0Ga\n"
	c := newConfigCopy(".htaccess", []byte(ht), int64(len(ht)))
	got := strings.Join(c.Lines, "\n")
	if strings.Contains(got, "abc def") || strings.Contains(got, "9fK2pQx7") || !strings.Contains(got, "RewriteRule ^x$ /y [L]") {
		t.Fatalf("redacted .htaccess = %q", got)
	}
}

func TestLooksRandom(t *testing.T) {
	for s, want := range map[string]bool{
		"x+3Kq!vZ@9pL#2mN$7rT%4wY^1bH&6cJ*8dF(0gS)5aE-": true,
		"5f4dcc3b5aa765d61d8327deb882cf99":              true,
		"Zq8Lw3Vt1Nx6Kp4Rm9Hs2Jd7Bf":              true,
		"/sites/example.com/files/wp-content":           false,
		"https://example.com/wp-admin/":                 false,
		"Silence is golden and so on":                   false,
		"short":                                         false,
	} {
		if got := looksRandom(s); got != want {
			t.Errorf("looksRandom(%q) = %t, want %t", s, got, want)
		}
	}
}

func TestConfigDiff(t *testing.T) {
	old := newConfigCopy("wp-config.php", []byte(testConfig), int64(len(testConfig)))
	changed := strings.Replace(testConfig, "define( 'WP_DEBUG', false );", "define( 'WP_DEBUG', false );\n@include( '/tmp/.x.ico' );", 1)
	changed = strings.Replace(changed, "hunter2-but-longer", "new-password-here", 1)
	cur := newConfigCopy("wp-config.php", []byte(changed), int64(len(changed)))

	detail, diff := configChange(Entry{Config: old}, Entry{Config: cur}, true)
	if detail != "; secrets changed (values hidden): DB_PASSWORD" {
		t.Errorf("detail = %q", detail)
	}
	want := "@@ line 12 @@\n $db_pass = \"[redacted]\";\n define( 'WP_DEBUG', false );\n+@include( '/tmp/.x.ico' );\n require_once ABSPATH . 'wp-settings.php';"
	if diff != want {
		t.Errorf("diff =\n%s\nwant\n%s", diff, want)
	}
	if strings.Contains(diff, "new-password-here") {
		t.Error("diff shows the new password")
	}

	// Without a copy in the baseline, as after an upgrade, there's no diff.
	if detail, diff := configChange(Entry{}, Entry{Config: cur}, true); detail != "" || diff != "" {
		t.Errorf("without a baseline copy = %q, %q", detail, diff)
	}
}

func TestConfigDiffIsCapped(t *testing.T) {
	var b strings.Builder
	for range 500 {
		b.WriteString("# a line that a plugin wrote into .htaccess\n")
	}
	cur := newConfigCopy(".htaccess", []byte(b.String()), int64(b.Len()))
	diff := configDiff(&ConfigCopy{}, cur)
	if len(diff) > maxConfigDiff || !strings.Contains(diff, "more lines") {
		t.Fatalf("diff of %d bytes ends %q", len(diff), diff[max(0, len(diff)-40):])
	}
}

// A change to wp-config.php is reported with the diff as its snippet, and
// the baseline keeps only the redacted copy.
func TestEvaluateShowsConfigDiff(t *testing.T) {
	root := t.TempDir()
	owner := uint32(os.Getuid())
	writeFile(t, root, "wp-config.php", testConfig)
	first, _ := Walk(root, owner, Options{})
	baseline := NewState(first.Entries)
	b, _ := json.Marshal(baseline)
	for _, v := range secretValues {
		if strings.Contains(string(b), v) {
			t.Fatalf("baseline keeps %q", v)
		}
	}

	writeFile(t, root, "wp-config.php", strings.Replace(testConfig, "<?php\n", "<?php\n@include '/tmp/.x.ico';\n", 1))
	cur, _ := Walk(root, owner, Options{})
	var f *models.ScanFinding
	for _, g := range Evaluate(baseline.Entries, cur.Entries, owner, func(uint32) string { return "u" }) {
		if g.Rule == RuleConfig {
			f = &g
		}
	}
	if f == nil {
		t.Fatal("no config-change finding")
	}
	if f.Detail != "changed wp-config.php" || !strings.HasPrefix(f.Snippet, "@@ line 1 @@\n <?php\n+@include '/tmp/.x.ico';\n define( 'DB_NAME', 'site_db' );") {
		t.Fatalf("finding = %q, snippet:\n%s", f.Detail, f.Snippet)
	}
}
