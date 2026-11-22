package scan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// PageCodePrefix starts the paths of page code: HTML and scripts that
// themes and plugins keep in the database and print on every page, where a
// skimmer needs no file at all. One client site's sat in Divi's body code field.
const PageCodePrefix = "db/"

// IsPageCodePath reports whether p is page code read from the database.
func IsPageCodePath(p string) bool {
	return strings.HasPrefix(p, PageCodePrefix)
}

const (
	// maxPageCodeEntries and maxPageCodeBytes bound what one site's page
	// code adds to a scan.
	maxPageCodeEntries = 200
	maxPageCodeBytes   = 256 << 10
)

// PageCodeEval is the PHP wp-cli runs to print a site's page code as a
// JSON object of path (under PageCodePrefix) to content:
//   - Divi's Theme Options → Integration fields (et_divi),
//   - Text and Custom HTML widgets (widget_text, widget_custom_html),
//   - WPCode's (Insert Headers and Footers) global header, body and footer.
const PageCodeEval = `$out = array();
$d = get_option('et_divi');
if (is_array($d)) {
	foreach (array('divi_integration_head', 'divi_integration_body', 'divi_integration_single_top', 'divi_integration_single_bottom') as $k) {
		if (isset($d[$k]) && is_string($d[$k]) && trim($d[$k]) !== '') { $out['et_divi/' . $k] = $d[$k]; }
	}
}
foreach (array('widget_text' => 'text', 'widget_custom_html' => 'content') as $opt => $field) {
	$w = get_option($opt);
	if (!is_array($w)) { continue; }
	foreach ($w as $n => $inst) {
		if (is_array($inst) && isset($inst[$field]) && is_string($inst[$field]) && trim($inst[$field]) !== '') { $out[$opt . '/' . $n] = $inst[$field]; }
	}
}
foreach (array('ihaf_insert_header', 'ihaf_insert_body', 'ihaf_insert_footer') as $opt) {
	$v = get_option($opt);
	if (is_string($v) && trim($v) !== '') { $out[$opt] = $v; }
}
echo json_encode((object) $out);`

// pageCodePattern is script code skimmers use and tracking snippets don't.
type pageCodePattern struct {
	name string
	find func([]byte) []int
}

var (
	atobRe          = regexp.MustCompile(`(?i)\batob\s*\(`)
	createElementRe = regexp.MustCompile(`(?i)createElement`)
	scriptWordRe    = regexp.MustCompile(`(?i)['"]script['"]`)
	paddedScriptRe  = regexp.MustCompile(`(?i)(?:[ \t]*\r?\n){20,}[ \t]*<script`)
	charCodeRe      = regexp.MustCompile(`(?i)String\.fromCharCode\s*\(\s*(?:\d+\s*,\s*){8,}`)
	evalCallRe      = regexp.MustCompile(`(?:^|[^\w.$])(?:eval\s*\(|new\s+Function\s*\()`)
	unescapeWriteRe = regexp.MustCompile(`(?i)document\.write\s*\(\s*(?:unescape|atob|decodeURIComponent)\s*\(`)
	cardFieldRe     = regexp.MustCompile(`(?i)card.?number|cc-?num|\bcvv\b|\bcvc\b|cc-?exp`)
	exfilRe         = regexp.MustCompile(`(?i)\bfetch\s*\(|XMLHttpRequest|sendBeacon|new\s+Image\s*\(`)
)

// pageCodePatterns are the rules for page code.
var pageCodePatterns = []pageCodePattern{
	// A script element built with a URL or code decoded at run time:
	// One client site's loader took the skimmer's URL from atob().
	{"encoded-script-loader", func(b []byte) []int {
		if !createElementRe.Match(b) || !scriptWordRe.Match(b) {
			return nil
		}
		return atobRe.FindIndex(b)
	}},
	// A script pushed out of sight below a screenful of blank lines.
	{"padded-script", func(b []byte) []int {
		loc := paddedScriptRe.FindIndex(b)
		if loc == nil {
			return nil
		}
		return []int{loc[1] - len("<script"), loc[1]}
	}},
	{"charcode-script", func(b []byte) []int { return charCodeRe.FindIndex(b) }},
	{"eval-code", func(b []byte) []int { return evalCallRe.FindIndex(b) }},
	{"encoded-write", func(b []byte) []int { return unescapeWriteRe.FindIndex(b) }},
	// Reads card fields and sends something off: a skimmer written inline.
	{"card-exfiltration", func(b []byte) []int {
		if !exfilRe.Match(b) {
			return nil
		}
		return cardFieldRe.FindIndex(b)
	}},
}

// matchPageCode returns the names of the page code rules body matches and
// a detail quoting where each matched.
func matchPageCode(body []byte) ([]string, string) {
	var names, lines []string
	for _, p := range pageCodePatterns {
		loc := p.find(body)
		if loc == nil {
			continue
		}
		start := loc[0]
		excerpt := string(body[start:min(len(body), start+maxMatchExcerpt)])
		excerpt = strings.ToValidUTF8(strings.Join(strings.Fields(excerpt), " "), "�")
		names = append(names, p.name)
		lines = append(lines, fmt.Sprintf("%s (line %d): %s", p.name, bytes.Count(body[:start], []byte("\n"))+1, excerpt))
	}
	return names, strings.Join(lines, "\n")
}

// PageCodeEntries turns PageCodeEval's output into scan entries owned by
// uid: hashed, with their start as the snippet and the page code rules
// they match. PHP notices printed before the JSON are skipped.
func PageCodeEntries(out string, uid uint32) ([]Entry, error) {
	out = strings.TrimSpace(out)
	if i := strings.LastIndex(out, "\n{"); i >= 0 {
		out = out[i+1:]
	}
	var code map[string]string
	if err := json.Unmarshal([]byte(out), &code); err != nil {
		return nil, fmt.Errorf("failed to parse the page code: %w", err)
	}
	keys := make([]string, 0, len(code))
	for k := range code {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > maxPageCodeEntries {
		keys = keys[:maxPageCodeEntries]
	}
	entries := make([]Entry, 0, len(keys))
	for _, k := range keys {
		body := []byte(code[k])
		size := int64(len(body))
		sum := sha256.Sum256(body)
		if len(body) > maxPageCodeBytes {
			body = body[:maxPageCodeBytes]
		}
		e := Entry{
			Path:   PageCodePrefix + k,
			UID:    uid,
			Size:   size,
			SHA256: hex.EncodeToString(sum[:]),
			Head:   strings.ToValidUTF8(string(body[:min(len(body), headBytes)]), "�"),
		}
		e.Content, e.ContentDetail = matchPageCode(body)
		entries = append(entries, e)
	}
	return entries, nil
}

// SeedPageCodeEntries adds page code to a baseline that predates reading
// it, so the first read after an upgrade doesn't report every site's
// widgets as changed. The page code rules still apply: they describe a
// state.
func SeedPageCodeEntries(prev *State, cur []Entry) {
	if prev == nil || prev.PageCodeChecked {
		return
	}
	for _, e := range cur {
		if !IsPageCodePath(e.Path) {
			continue
		}
		e.Content = nil
		prev.Entries[e.Path] = e
	}
}

// CarryPageCode copies the previous baseline's page code into cur, for a
// scan that couldn't read it, so it doesn't count as removed.
func CarryPageCode(prev *State, cur []Entry) []Entry {
	if prev == nil {
		return cur
	}
	for p, e := range prev.Entries {
		if IsPageCodePath(p) {
			cur = append(cur, e)
		}
	}
	return cur
}
