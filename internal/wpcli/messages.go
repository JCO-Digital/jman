package wpcli

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// phpDiagnostic matches a PHP notice, warning or deprecation that wp-cli
// passes through from the site's code ("Warning: foo in /path/x.php on
// line 12"), which wp-cli's own messages never end with.
var phpDiagnostic = regexp.MustCompile(` in \S+ on line \d+\.?$`)

// maxMessageLen caps a short message, and maxLineLen each line of it: one
// of WordPress's warnings lists every file it couldn't copy. The full
// output is kept with the job.
const (
	maxMessageLen = 500
	maxLineLen    = 200
)

// Messages returns wp-cli's own errors and warnings from stderr, joined
// with "; ", for a short message to show: "Warning: Download failed.
// Unauthorized; Error: No plugins updated (1 failed)". PHP's notices,
// warnings and deprecations from the site's code are left out, as they
// are the noise that buries the reason. It returns "" when there is none.
func Messages(stderr string) string {
	var msgs []string
	for line := range strings.SplitSeq(stderr, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Error:") && !strings.HasPrefix(line, "Warning:") {
			continue
		}
		if phpDiagnostic.MatchString(line) {
			continue
		}
		line = truncate(line, maxLineLen)
		if !slices.Contains(msgs, line) {
			msgs = append(msgs, line)
		}
	}
	return truncate(strings.Join(msgs, "; "), maxMessageLen)
}

// truncate cuts s to at most n bytes, on a character boundary, marking
// the cut with "…".
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

// shortError is the short message of a failed call: wp-cli's own errors
// and warnings, or err (RunWP's first error line, or the exit status) when
// it printed none.
func shortError(stderr string, err error) string {
	if msg := Messages(stderr); msg != "" {
		return msg
	}
	return err.Error()
}
