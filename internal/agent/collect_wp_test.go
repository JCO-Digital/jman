package agent

import "testing"

func TestParseWpFlagsAutoUpdateCore(t *testing.T) {
	cases := []struct {
		config, want string
	}{
		{`<?php define('DB_NAME', 'x');`, "default"},
		{`define( 'WP_AUTO_UPDATE_CORE', true );`, "true"},
		{`define('WP_AUTO_UPDATE_CORE', false);`, "false"},
		{`define("WP_AUTO_UPDATE_CORE", 'minor');`, "minor"},
		{`define('WP_AUTO_UPDATE_CORE', 'true');`, "true"},
		// AUTOMATIC_UPDATER_DISABLED turns every automatic update off.
		{"define('WP_AUTO_UPDATE_CORE', true);\ndefine('AUTOMATIC_UPDATER_DISABLED', true);", "disabled"},
		{`define('AUTOMATIC_UPDATER_DISABLED', false);`, "default"},
	}
	for _, c := range cases {
		if got := parseWpFlags([]byte(c.config)).AutoUpdateCore; got != c.want {
			t.Errorf("%q: AutoUpdateCore = %q, want %q", c.config, got, c.want)
		}
	}

	flags := parseWpFlags([]byte(`define('MULTISITE', true); define('DISALLOW_FILE_MODS', true);`))
	if !flags.IsMultisite || !flags.DisallowFileMods {
		t.Errorf("flags = %+v", flags)
	}
}
