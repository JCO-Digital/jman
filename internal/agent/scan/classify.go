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
// archives and backups, and editor leftovers.
var straySuffixes = []string{
	".sql", ".sql.gz", ".sql.bz2", ".sql.zip",
	".zip", ".tar", ".tar.gz", ".tgz",
	".bak", ".old", ".orig",
	".swp", ".swo", "~",
}

// strayDirs are directories that shouldn't be in a webroot. The walk
// records them and doesn't descend into them.
var strayDirs = map[string]bool{".git": true, ".svn": true}

const (
	uploadsPrefix   = "wp-content/uploads/"
	muPluginsPrefix = "wp-content/mu-plugins/"
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

func isMuPlugin(p string) bool {
	return strings.HasPrefix(p, muPluginsPrefix)
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
func isStrayFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	if base == ".env" {
		return true
	}
	for _, suffix := range straySuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
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
