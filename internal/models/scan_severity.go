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

// IsMinorStrayFile reports whether a stray file at p is informational: a
// backup or swap file of something that isn't code or config. A copy of a
// PHP file or of wp-config (wp-config.php.bak, .index.php.swp) stays low,
// since the web server serves it as text, with the source and any secrets
// it has. Database dumps, archives, .env files and .git directories stay
// low too.
func IsMinorStrayFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	if strings.Contains(base, ".php") || strings.Contains(base, ".phtml") || strings.Contains(base, "wp-config") ||
		strings.Contains(base, ".env") || strings.Contains(base, ".sql") || strings.Contains(base, ".htaccess") ||
		strings.Contains(base, ".user.ini") {
		return false
	}
	for _, suffix := range minorStraySuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}
