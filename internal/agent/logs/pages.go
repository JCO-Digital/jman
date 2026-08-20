package logs

import "strings"

// excludedPagePrefixes are page paths that are never useful in a "top
// pages" list — WordPress core system paths and static assets rather than
// an actual visitor page view — so they're dropped before ever occupying
// one of the limited top-N slots. This is an explicit list rather than a
// blanket "/wp-" prefix match, so a real post/page whose slug happens to
// start with "wp-" isn't accidentally hidden.
var excludedPagePrefixes = []string{
	"/wp-admin",
	"/wp-content",
	"/wp-includes",
	"/wp-json",
	"/wp-login.php",
	"/wp-cron.php",
	"/wp-load.php",
	"/wp-mail.php",
	"/wp-signup.php",
	"/wp-activate.php",
	"/wp-trackback.php",
	"/wp-comments-post.php",
	"/wp-links-opml.php",
	"/xmlrpc.php",
}

// isExcludedPage reports whether path (already stripped of its query
// string, see PathWithoutQuery) matches one of excludedPagePrefixes.
func isExcludedPage(path string) bool {
	for _, prefix := range excludedPagePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
