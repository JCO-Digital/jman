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
	// The patterns are anchored so the loaded and skipped runs can't both
	// match; the loaded run registers premium update checks (--exec).
	loaded := "--path=/srv/www\\ --exec=*\\ --skip-themes\\ plugin\\ list*"
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

	// A wp-cli without update_unavailable_reason is asked for its default
	// fields, with the plugins still loaded.
	withFields := "--path=/srv/www\\ --exec=*\\ --skip-themes\\ plugin\\ list\\ --fields=*"
	defaults := "--path=/srv/www\\ --exec=*\\ --skip-themes\\ plugin\\ list\\ --format=json"
	fakeWP(t, map[string]string{
		"*user\\ list*": "echo '[]'",
		withFields:      "echo 'Error: Invalid field: update_unavailable_reason.' >&2; exit 1",
		defaults:        `echo '[{"name":"premium","status":"active","version":"1.0","update_version":"1.1"}]'`,
	})
	plugins, err = GetPluginsWithUpdates(site)
	if err != nil || len(plugins) != 1 || plugins[0].Update != "1.1" {
		t.Errorf("old wp-cli: plugins = %+v, err = %v", plugins, err)
	}
}

// An update wp-cli reports as unavailable is blocked with its reason; the
// reason wp-cli leaves over from an earlier plugin isn't another plugin's.
func TestParsePluginListUnavailableUpdates(t *testing.T) {
	out := `[
{"name":"duracelltomi-google-tag-manager","status":"active","update":"unavailable","version":"1.20","update_version":"1.21","auto_update":"off","update_unavailable_reason":"This update requires PHP version 8.0, but the version installed is 7.4.33."},
{"name":"akismet","status":"active","update":"available","version":"5.3","update_version":"5.4","auto_update":"off","update_unavailable_reason":"This update requires PHP version 8.0, but the version installed is 7.4.33."},
{"name":"wordpress-seo","status":"active","update":"unavailable","version":"26.4","update_version":"27.0","auto_update":"on","update_unavailable_reason":"This update requires WordPress version 7.0, but the version installed is 6.9.12."},
{"name":"sitepress-multilingual-cms","status":"active","update":"unavailable","version":"5.2.0","update_version":"5.3.0","auto_update":"off","update_unavailable_reason":""},
{"name":"hello","status":"inactive","update":"none","version":"1.7.2","update_version":null,"auto_update":"off","update_unavailable_reason":null},
{"name":"old-cli","status":"active","update":"available","version":"1.0","update_version":"1.1","auto_update":"off"}
]`
	got, err := ParsePluginList(out, "site")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"duracelltomi-google-tag-manager": "This update requires PHP version 8.0, but the version installed is 7.4.33.",
		"akismet":                         "",
		"wordpress-seo":                   "This update requires WordPress version 7.0, but the version installed is 6.9.12.",
		"sitepress-multilingual-cms":      "The update needs a newer WordPress or PHP version than the site has.",
		"hello":                           "",
		"old-cli":                         "",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d plugins, want %d", len(got), len(want))
	}
	for _, p := range got {
		if p.UpdateBlocked != want[p.Name] {
			t.Errorf("%s: UpdateBlocked = %q, want %q", p.Name, p.UpdateBlocked, want[p.Name])
		}
	}
	if got[2].Update != "27.0" {
		t.Errorf("wordpress-seo: Update = %q, want the blocked version kept", got[2].Update)
	}
}
