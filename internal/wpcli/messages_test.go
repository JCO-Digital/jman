package wpcli

import (
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestMessages(t *testing.T) {
	stderr := strings.Join([]string{
		"PHP Deprecated:  Creation of dynamic property Foo::$bar is deprecated in /srv/www/wp-content/plugins/x/x.php on line 12",
		"Deprecated: Creation of dynamic property Foo::$bar is deprecated in /srv/www/x.php on line 12",
		"Warning: Undefined array key \"x\" in /srv/www/wp-content/plugins/y/y.php on line 40",
		"Warning: Download failed. Unauthorized",
		"Warning: Download failed. Unauthorized",
		"Error: No plugins updated (1 failed).",
		"",
	}, "\n")
	want := "Warning: Download failed. Unauthorized; Error: No plugins updated (1 failed)."
	if got := Messages(stderr); got != want {
		t.Errorf("Messages = %q, want %q", got, want)
	}
	if got := Messages("PHP Notice: x in /a.php on line 1\n"); got != "" {
		t.Errorf("Messages of PHP noise only = %q, want empty", got)
	}
	if got := Messages("Warning: " + strings.Repeat("x", 2*maxMessageLen)); len(got) > maxMessageLen+len("…") {
		t.Errorf("long message not capped: %d bytes", len(got))
	}
}

func TestCapOutput(t *testing.T) {
	if s := capOutput("short"); s != "short" {
		t.Errorf("capOutput(short) = %q", s)
	}
	long := "HEAD" + strings.Repeat("x", 2*maxRecordedOutput) + "TAIL"
	got := capOutput(long)
	if len(got) > maxRecordedOutput+100 || !strings.HasPrefix(got, "HEAD") || !strings.HasSuffix(got, "TAIL") || !strings.Contains(got, "bytes left out") {
		t.Errorf("capOutput kept %d bytes, head %q, tail %q", len(got), got[:10], got[len(got)-10:])
	}
}

func TestParseCLIInfoPHPVersion(t *testing.T) {
	out := `Warning: something first
{"php_binary_path":"/usr/bin/php8.1","php_version":"8.1.27","wp_cli_version":"2.10.0"}`
	if v, err := parseCLIInfoPHPVersion(out); err != nil || v != "8.1.27" {
		t.Errorf("parseCLIInfoPHPVersion = %q, %v", v, err)
	}
	if _, err := parseCLIInfoPHPVersion("no json"); err == nil {
		t.Error("parseCLIInfoPHPVersion without JSON succeeded")
	}
}

func TestParseUpdateTable(t *testing.T) {
	out := "Enabling Maintenance mode...\n" +
		"Downloading update from https://downloads.wordpress.org/plugin/duplicator.5.2.0.zip...\n" +
		"Plugin update failed.\n" +
		"Disabling Maintenance mode...\n" +
		"name\told_version\tnew_version\tstatus\n" +
		"contact-form-7\t6.0.1\t6.2.6\tUpdated\n" +
		"duplicator\t1.3.26\t5.2.0\tError\n"
	got, err := parseUpdateOutput(out)
	if err != nil || len(got) != 2 || got[0] != (UpdateResult{Name: "contact-form-7", OldVersion: "6.0.1", NewVersion: "6.2.6", Status: "Updated"}) || got[1].Status != "Error" {
		t.Errorf("parseUpdateOutput = %+v, %v", got, err)
	}
	if got, err := parseUpdateOutput("Success: Plugin already updated."); err != nil || len(got) != 0 {
		t.Errorf("up to date = %+v, %v", got, err)
	}
	// JSON still parses, for older callers' output.
	if got, err := parseUpdateOutput(`[{"name":"a","old_version":"1","new_version":"2","status":"Updated"}]`); err != nil || len(got) != 1 {
		t.Errorf("JSON = %+v, %v", got, err)
	}
}

// A plugin that failed gets the upgrader's warning, which wp-cli only
// prints without --format=json, and the call is recorded.
func TestUpdatePluginKeepsTheReason(t *testing.T) {
	fakeWP(t, map[string]string{
		"*eval*":                          "exit 0",
		"*user\\ list*":                   "echo '[]'",
		"*--format=json*plugin\\ update*": "echo 'unexpected --format=json' >&2; exit 98",
		"*plugin\\ update*": `printf 'Downloading update...\nname\told_version\tnew_version\tstatus\nok\t1.0\t1.1\tUpdated\nbad\t2.0\t2.1\tError\n'; ` +
			`printf 'Warning: The update cannot be installed because some files could not be copied. "a, b, c"\nError: Only updated 1 of 2 plugins (1 failed).\n' >&2; exit 1`,
		"*plugin\\ list*":                `echo '[{"name":"ok","status":"active","version":"1.1"},{"name":"bad","status":"active","version":"2.0"}]'`,
		"*maintenance-mode\\ is-active*": "exit 1",
	})
	var rec []models.WPCLICall
	updates, err := UpdatePlugin(models.CliSite{ID: "s", Name: "example.com"}, []string{"ok", "bad"}, &rec)
	if err == nil || len(updates) != 2 {
		t.Fatalf("UpdatePlugin = %+v, %v", updates, err)
	}
	want := `Warning: The update cannot be installed because some files could not be copied. "a, b, c"; Error: Only updated 1 of 2 plugins (1 failed).`
	if updates[1].Error != want || updates[0].Error != "" {
		t.Errorf("results = %+v, want the warning on the failed plugin", updates)
	}
	if !strings.Contains(err.Error(), "some files could not be copied") {
		t.Errorf("err = %v", err)
	}
	if len(rec) != 1 || !strings.Contains(rec[0].Stderr, "Warning:") || rec[0].Args[0] != "plugin" {
		t.Errorf("recorded calls = %+v", rec)
	}
}
