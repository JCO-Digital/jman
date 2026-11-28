package models

import (
	"path"
	"strings"
)

// ScanSeverityInfo is the severity below low: findings worth listing that
// nobody needs to act on soon, such as an editor's backup copy of an image.
// Like low ones, they don't open incidents, and unlike low ones they don't
// keep plugins from being copied off the site (see CountOpenSiteScanFindings).
const ScanSeverityInfo = "info"

// minorStraySuffixes are the backup and editor swap file suffixes a stray
// file can have and still be informational.
var minorStraySuffixes = []string{".bak", ".old", ".orig", ".swp", ".swo"}

// harmlessBackupExts are the kinds of files whose backups are
// informational: media, documents, styles, scripts and fonts a site serves
// anyway, which hold no code that runs on the server and no secrets.
var harmlessBackupExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true, ".ico": true, ".bmp": true, ".tif": true, ".tiff": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
	".mp3": true, ".mp4": true, ".m4a": true, ".mov": true, ".webm": true, ".ogg": true, ".wav": true,
	".css": true, ".scss": true, ".js": true, ".map": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".po": true, ".mo": true, ".pot": true,
}

// IsMinorStrayFile reports whether a stray file at p is informational: a
// backup or swap file (.bak, .old, .orig, .swp, .swo) of a kind of file in
// harmlessBackupExts, such as logo.png.bak or .style.css.swp. Backups of
// anything else stay low: the web server serves them as text, and they may
// be code (index.php.old), config with secrets (wp-config.php.bak,
// .htpasswd.bak, config.inc.old), a database dump or a whole archive of the
// site (backup.tar.gz.old).
func IsMinorStrayFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	stripped := false
	for {
		trimmed := base
		for _, suffix := range minorStraySuffixes {
			trimmed = strings.TrimSuffix(trimmed, suffix)
		}
		if trimmed == base {
			break
		}
		base, stripped = trimmed, true
	}
	return stripped && harmlessBackupExts[path.Ext(base)]
}
