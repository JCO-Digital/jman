package utils

import (
	"html"
	"regexp"
	"strings"
)

// htmlTagRegexp matches basic HTML tags for removal in CleanHTML.
var htmlTagRegexp = regexp.MustCompile(`<[^>]*>`)
var unifyHyphens = regexp.MustCompile(`[-–—]+`)

// CleanHTML removes HTML tags, decodes HTML entities, and trims surrounding whitespace.
// This normalizes text from external feeds (like WordPress.org or vulnerability databases)
// for CLI or Slack output.
func CleanHTML(s string) string {
	s = htmlTagRegexp.ReplaceAllString(s, "")
	s = unifyHyphens.ReplaceAllString(s, "-")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}

// Show the first part of the string up to " - ".
func ShowFirstPart(s string) string {
	s = unifyHyphens.ReplaceAllString(s, "-")
	parts := strings.SplitN(s, " - ", 2)
	return parts[0]
}
