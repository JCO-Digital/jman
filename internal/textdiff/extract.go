// Package textdiff compares a page's visible text before and after an
// update. A page is reduced to lines of visible text (Extract), lines are
// compared in normalized form (Normalize), lines that change between two
// fetches without an update are noise (Noise), and Diff reports what is
// left. See docs/update-text-diff.md.
package textdiff

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// skipTags hold no visible text.
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true,
	"svg": true, "iframe": true, "object": true, "head": true,
}

// skipClasses are containers that visitors never see but whose text
// changes on every request. Gravity Forms' honeypot field gets a random
// label (Name, Phone, LinkedIn, ...) on each render. Only hidden content
// belongs here: visible random content is for noise filtering and mutes.
var skipClasses = map[string]bool{
	"gform_validation_container": true,
}

// blockTags start and end a line.
var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"body": true, "br": true, "button": true, "caption": true, "dd": true,
	"details": true, "dialog": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "header": true, "hr": true, "label": true, "legend": true,
	"li": true, "main": true, "nav": true, "ol": true, "option": true,
	"p": true, "pre": true, "section": true, "select": true, "summary": true,
	"table": true, "td": true, "textarea": true, "th": true, "tr": true,
	"ul": true,
}

// textAttrs are attributes whose values are shown to visitors or read by
// screen readers. Each value becomes its own line, prefixed with the
// attribute name in brackets.
var textAttrs = []string{"alt", "title", "aria-label", "placeholder"}

// Extract parses an HTML document and returns its visible text as lines in
// document order: one line per block element, plus one per text
// attribute, with whitespace collapsed and empty lines dropped.
func Extract(r io.Reader) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	e := extractor{}
	e.walk(doc)
	e.flush()
	return e.lines, nil
}

type extractor struct {
	lines []string
	cur   strings.Builder
}

func (e *extractor) flush() {
	e.add(e.cur.String())
	e.cur.Reset()
}

func (e *extractor) add(s string) {
	if s = collapse(s); s != "" {
		e.lines = append(e.lines, s)
	}
}

func (e *extractor) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		e.cur.WriteString(n.Data)
		e.cur.WriteByte(' ')
		return
	case html.CommentNode:
		return
	case html.ElementNode:
		if skipTags[n.Data] || hasSkipClass(n) {
			return
		}
		block := blockTags[n.Data]
		if block {
			e.flush()
		}
		for _, want := range textAttrs {
			if v := attr(n, want); collapse(v) != "" {
				e.lines = append(e.lines, "["+want+"] "+collapse(v))
			}
		}
		if n.Data == "input" {
			if t := attr(n, "type"); t == "submit" || t == "button" {
				if v := collapse(attr(n, "value")); v != "" {
					e.lines = append(e.lines, "[value] "+v)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			e.walk(c)
		}
		if block {
			e.flush()
		}
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.walk(c)
	}
}

func hasSkipClass(n *html.Node) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if skipClasses[c] {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
