package utils

import (
	"html"
	"regexp"
	"strings"
)

// htmlTagRegexp matches basic HTML tags for removal in CleanHTML.
var htmlTagRegexp = regexp.MustCompile(`<[^>]*>`)
var unifyHyphens = regexp.MustCompile(`[-–—]+`)
var slugRegex = regexp.MustCompile(`^[a-z0-9-]+$`)
var versionRegex = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,3}$`)
var ansiRegexp = regexp.MustCompile(`\x1B\[[0-?]*[ -/]*[@-~]`)

// IsValidSlug checks if a plugin slug is valid according to WordPress standards.
func IsValidSlug(slug string) bool {
	return slugRegex.MatchString(slug)
}

// IsValidVersion checks if a string looks like a plain dotted version number (e.g. "6.6.1").
func IsValidVersion(version string) bool {
	return versionRegex.MatchString(version)
}

// CleanHTML removes HTML tags, decodes HTML entities, and trims surrounding whitespace.
// This normalizes text from external feeds (like WordPress.org or vulnerability databases)
// for CLI or Slack output.
// StripANSI removes ANSI escape codes from a string.
func StripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

func CleanHTML(s string) string {
	s = htmlTagRegexp.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = unifyHyphens.ReplaceAllString(s, "-")
	return strings.TrimSpace(s)
}

// Show the first part of the string up to " - ".
func ShowFirstPart(s string) string {
	s = unifyHyphens.ReplaceAllString(s, "-")
	parts := strings.SplitN(s, " - ", 2)
	return parts[0]
}
