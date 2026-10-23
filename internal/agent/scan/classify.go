// Package scan looks through a site's files for signs of compromise. The
// walk (Walk) runs in a child process as the site's owner; comparing its
// result with the baseline (Evaluate) and keeping that baseline (LoadState,
// SaveState) happen in the root-running agent.
package scan

import (
	"path"
	"strings"
)

// phpExtensions are the extensions a web server may run as PHP.
var phpExtensions = []string{".php", ".phtml", ".pht", ".phar", ".php3", ".php4", ".php5", ".php7", ".php8", ".phps"}

// dropins are the files in wp-content that WordPress loads on its own
// when they exist.
var dropins = map[string]bool{
	"advanced-cache.php":      true,
	"object-cache.php":        true,
	"db.php":                  true,
	"db-error.php":            true,
	"install.php":             true,
	"maintenance.php":         true,
	"php-error.php":           true,
	"fatal-error-handler.php": true,
	"sunrise.php":             true,
	"blog-deleted.php":        true,
	"blog-inactive.php":       true,
	"blog-suspended.php":      true,
}

// straySuffixes mark files that shouldn't be in a webroot: database dumps,
// archives and backups, and editor swap files. A trailing "~" isn't one:
// Loco Translate leaves hundreds of harmless *.po~ backups.
var straySuffixes = []string{
	".sql", ".sql.gz", ".sql.bz2", ".sql.zip",
	".zip", ".tar", ".tar.gz", ".tgz",
	".bak", ".old", ".orig",
	".swp", ".swo",
}

// strayDirs are directories that shouldn't be in a webroot. The walk
// records them and doesn't descend into them.
var strayDirs = map[string]bool{".git": true, ".svn": true}

const (
	uploadsPrefix   = "wp-content/uploads/"
	muPluginsPrefix = "wp-content/mu-plugins/"
	// wpmlTwigCache is where WPML keeps compiled Twig templates, PHP files
	// it regenerates whenever its cache is rebuilt.
	wpmlTwigCache = "/cache/wpml/twig/"
)

// isPHP reports whether name has an extension a web server may run as PHP.
func isPHP(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range phpExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// isUploadsPHP reports whether p, a slash-separated path relative to the
// site's directory, is a PHP file in the uploads directory.
func isUploadsPHP(p string) bool {
	return strings.HasPrefix(p, uploadsPrefix) && isPHP(path.Base(p))
}

// isMuPlugin reports whether p is directly in mu-plugins, where WordPress
// loads every PHP file on its own. Files in its subdirectories, such as a
// bundled plugin, only run when one of those includes them.
func isMuPlugin(p string) bool {
	rest, ok := strings.CutPrefix(p, muPluginsPrefix)
	return ok && !strings.Contains(rest, "/")
}

// isWPMLTwigCache reports whether p is one of WPML's compiled templates in
// uploads (on a multisite, under uploads/sites/<n>/).
func isWPMLTwigCache(p string) bool {
	rest, ok := strings.CutPrefix(p, uploadsPrefix)
	return ok && strings.Contains("/"+rest, wpmlTwigCache)
}

func isDropin(p string) bool {
	return path.Dir(p) == "wp-content" && dropins[path.Base(p)]
}

// isConfig reports whether p is wp-config.php or the site root's .htaccess.
func isConfig(p string) bool {
	return p == "wp-config.php" || p == ".htaccess"
}

// isPHPIni reports whether p can set PHP options for its directory, and so
// is read for auto_prepend_file and auto_append_file.
func isPHPIni(p string) bool {
	base := path.Base(p)
	return base == ".user.ini" || base == ".htaccess"
}

// isStrayFile reports whether p is a file that shouldn't be in a webroot.
// In plugins and themes only .env files are: archives, SQL files and
// backups there are nearly always shipped with the code (test fixtures,
// schemas, bundled plugins). Zip files in uploads aren't either: they're
// nearly always there on purpose, as downloads. Zipped database dumps
// still are.
func isStrayFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	if base == ".env" {
		return true
	}
	if isCode(p) {
		return false
	}
	if strings.HasPrefix(p, uploadsPrefix) && strings.HasSuffix(base, ".zip") && !strings.HasSuffix(base, ".sql.zip") {
		return false
	}
	for _, suffix := range straySuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

// isCode reports whether p is in the plugins or themes directory.
func isCode(p string) bool {
	return strings.HasPrefix(p, "wp-content/plugins/") || strings.HasPrefix(p, "wp-content/themes/")
}

func isStrayDir(p string) bool {
	return strayDirs[path.Base(p)]
}

// isTracked reports whether a file's content is hashed and compared
// between scans.
func isTracked(p string) bool {
	return isUploadsPHP(p) || isMuPlugin(p) || isDropin(p) || isConfig(p) || isPHPIni(p)
}

// hasSecrets reports whether a file's content must not leave the server,
// so no snippet of it is sent.
func hasSecrets(p string) bool {
	return p == "wp-config.php"
}
