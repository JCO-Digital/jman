package textdiff

import (
	"regexp"
	"strings"
)

var (
	digits   = regexp.MustCompile(`\d+`)
	dateWord = regexp.MustCompile(`(?i)\b(?:` +
		// English months and weekdays, full and short.
		`january|february|march|april|may|june|july|august|september|october|november|december|` +
		`jan|feb|mar|apr|jun|jul|aug|sep|sept|oct|nov|dec|` +
		`monday|tuesday|wednesday|thursday|friday|saturday|sunday|mon|tue|wed|thu|fri|sat|sun|` +
		// Finnish months and weekdays, any inflection.
		`(?:tammi|helmi|maalis|huhti|touko|kesä|heinä|elo|syys|loka|marras|joulu)kuu\p{L}*|` +
		`(?:maanantai|tiistai|keskiviikko|torstai|perjantai|lauantai|sunnuntai)\p{L}*|` +
		// Swedish months (the ones not already matched above) and weekdays.
		`januari|februari|mars|maj|juni|juli|augusti|oktober|` +
		`(?:måndag|tisdag|onsdag|torsdag|fredag|lördag|söndag)\p{L}*` +
		`)\b`)
)

// Normalize returns the form of a line that diffs and mutes compare:
// whitespace collapsed, every run of digits replaced with "#" and month and
// weekday names (English, Finnish, Swedish) replaced with "<d>", so dates,
// times, counts and prices don't count as changes.
func Normalize(line string) string {
	line = strings.Join(strings.Fields(line), " ")
	line = digits.ReplaceAllString(line, "#")
	return dateWord.ReplaceAllString(line, "<d>")
}

func normalizeAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = Normalize(l)
	}
	return out
}
