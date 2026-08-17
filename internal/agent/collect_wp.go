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
)

// CollectWpFlags reads wp-config.php in the given site path and reports the
// MULTISITE and DISALLOW_FILE_MODS constants. This is a best-effort regex
// parse — sites that set these constants indirectly (e.g. via an included
// file, or a computed expression rather than a literal true/false) won't be
// detected and will report false.
func CollectWpFlags(sitePath string) (isMultisite bool, disallowFileMods bool, err error) {
	content, err := os.ReadFile(filepath.Join(sitePath, "wp-config.php"))
	if err != nil {
		return false, false, fmt.Errorf("failed to read wp-config.php: %w", err)
	}

	if m := multisiteRe.FindSubmatch(content); m != nil {
		isMultisite = strings.EqualFold(string(m[1]), "true")
	}
	if m := disallowFileModsRe.FindSubmatch(content); m != nil {
		disallowFileMods = strings.EqualFold(string(m[1]), "true")
	}

	return isMultisite, disallowFileMods, nil
}
