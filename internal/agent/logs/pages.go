package logs

import "strings"

// excludedPagePrefixes are page paths that are never useful in a "top
// pages" list — WordPress admin/API traffic rather than an actual visitor
// page view — so they're dropped before ever occupying one of the limited
// top-N slots.
var excludedPagePrefixes = []string{"/wp-admin", "/wp-json"}

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
