package wpcli

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestParsePluginDetails(t *testing.T) {
	out := `Warning: something noisy
[{"name":"akismet","title":"Akismet Anti-spam","version":"5.3","status":"active","author":"Automattic","update_version":"5.4"},{"name":"hello","title":"Hello Dolly","version":"1.7.2","status":"inactive","author":"Matt Mullenweg","update_version":""}]`
	got, err := parsePluginDetails(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "Akismet Anti-spam" || got[0].UpdateVersion != "5.4" || got[1].Author != "Matt Mullenweg" {
		t.Errorf("parsePluginDetails = %+v", got)
	}
}

func TestGetPluginsWithUpdates(t *testing.T) {
	site := models.CliSite{ID: "test-site", Name: "example.com", Path: "/srv/www"}
	// The patterns are anchored so the loaded and skipped runs can't both match.
	loaded := "--path=/srv/www\\ --skip-themes\\ plugin\\ list*"
	skipped := "--path=/srv/www\\ --skip-plugins\\ --skip-themes\\ plugin\\ list*"

	fakeWP(t, map[string]string{
		"*user\\ list*": "echo '[]'",
		loaded:          `echo '[{"name":"premium","status":"active","version":"1.0","update_version":"1.1"}]'`,
		skipped:         `echo '[{"name":"premium","status":"active","version":"1.0","update_version":""}]'`,
	})
	plugins, err := GetPluginsWithUpdates(site)
	if err != nil || len(plugins) != 1 || plugins[0].Update != "1.1" {
		t.Errorf("with plugins loaded: plugins = %+v, err = %v", plugins, err)
	}

	// A plugin that breaks wp-cli falls back to listing them skipped.
	fakeWP(t, map[string]string{
		"*user\\ list*": "echo '[]'",
		loaded:          "echo 'PHP Fatal error: broken plugin' >&2; exit 255",
		skipped:         `echo '[{"name":"premium","status":"active","version":"1.0","update_version":""}]'`,
	})
	plugins, err = GetPluginsWithUpdates(site)
	if err != nil || len(plugins) != 1 || plugins[0].Version != "1.0" {
		t.Errorf("after a failed loaded run: plugins = %+v, err = %v", plugins, err)
	}
}
