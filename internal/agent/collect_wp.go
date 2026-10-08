package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	multisiteRe        = regexp.MustCompile(`(?i)define\s*\(\s*['"]MULTISITE['"]\s*,\s*(true|false)\s*\)`)
	disallowFileModsRe = regexp.MustCompile(`(?i)define\s*\(\s*['"]DISALLOW_FILE_MODS['"]\s*,\s*(true|false)\s*\)`)
	autoUpdateCoreRe   = regexp.MustCompile(`(?i)define\s*\(\s*['"]WP_AUTO_UPDATE_CORE['"]\s*,\s*(true|false|['"](?:minor|true|false)['"])\s*\)`)
	updaterDisabledRe  = regexp.MustCompile(`(?i)define\s*\(\s*['"]AUTOMATIC_UPDATER_DISABLED['"]\s*,\s*(true|false)\s*\)`)
)

// WpFlags are the wp-config.php constants the agent reports.
type WpFlags struct {
	IsMultisite      bool
	DisallowFileMods bool
	// AutoUpdateCore is WordPress's core auto-update setting: "disabled"
	// (AUTOMATIC_UPDATER_DISABLED), the WP_AUTO_UPDATE_CORE value ("true",
	// "false" or "minor"), or "default" if neither is set.
	AutoUpdateCore string
}

// CollectWpFlags reads wp-config.php in the given site path and reports the
// MULTISITE, DISALLOW_FILE_MODS, WP_AUTO_UPDATE_CORE and
// AUTOMATIC_UPDATER_DISABLED constants. This is a best-effort regex parse —
// sites that set these constants indirectly (e.g. via an included file, or
// a computed expression rather than a literal) won't be detected and will
// report the defaults. The core auto-update setting can also be changed in
// wp-admin (the auto_update_core_major option), which this doesn't see.
func CollectWpFlags(sitePath string) (WpFlags, error) {
	content, err := os.ReadFile(filepath.Join(sitePath, "wp-config.php"))
	if err != nil {
		return WpFlags{}, fmt.Errorf("failed to read wp-config.php: %w", err)
	}
	return parseWpFlags(content), nil
}

func parseWpFlags(content []byte) WpFlags {
	flags := WpFlags{AutoUpdateCore: "default"}
	if m := multisiteRe.FindSubmatch(content); m != nil {
		flags.IsMultisite = strings.EqualFold(string(m[1]), "true")
	}
	if m := disallowFileModsRe.FindSubmatch(content); m != nil {
		flags.DisallowFileMods = strings.EqualFold(string(m[1]), "true")
	}
	if m := autoUpdateCoreRe.FindSubmatch(content); m != nil {
		flags.AutoUpdateCore = strings.ToLower(strings.Trim(string(m[1]), `'"`))
	}
	if m := updaterDisabledRe.FindSubmatch(content); m != nil && strings.EqualFold(string(m[1]), "true") {
		flags.AutoUpdateCore = "disabled"
	}
	return flags
}
