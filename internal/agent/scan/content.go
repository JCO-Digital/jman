package scan

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// maxContentBytes is how much of a PHP file the content rules search.
const maxContentBytes = 2 << 20

// maxMatchExcerpt caps the matched text a finding quotes.
const maxMatchExcerpt = 120

// contentPattern is code that malware uses and legitimate code next to
// never does. hints are lowercase strings the file must contain for the
// pattern to be worth running, to keep most files to a cheap search.
type contentPattern struct {
	name     string
	severity string
	hints    []string
	re       *regexp.Regexp
	// find, when set, is used instead of re: it returns where the pattern
	// first matches, or nil.
	find func([]byte) []int
}

// contentPatterns are the built-in content rules, after the ones PHP
// Malware Finder and similar scanners use, narrowed to what's rare in
// plugins and themes.
var contentPatterns = []contentPattern{
	{
		// Runs code hidden in an encoded string.
		name: "eval-decoded", severity: "high", hints: []string{"eval"},
		re: regexp.MustCompile(`(?i)\beval\s*\(\s*(?:@\s*)?(?:base64_decode|gzinflate|gzuncompress|gzdecode|str_rot13|strrev|hex2bin|rawurldecode|urldecode|convert_uudecode)\s*\(`),
	},
	{
		// Runs what the request sends.
		name: "exec-request", severity: "high", hints: []string{"$_"},
		re: regexp.MustCompile(`(?i)\b(?:eval|assert|system|exec|shell_exec|passthru|popen|proc_open|pcntl_exec|create_function|call_user_func(?:_array)?)\s*\(\s*(?:@\s*)?(?:stripslashes\s*\(\s*|base64_decode\s*\(\s*)?\$_(?:GET|POST|REQUEST|COOKIE|SERVER|FILES)\b`),
	},
	{
		// Calls a function the request names: $_POST['f']($_POST['a']).
		name: "request-call", severity: "high", hints: []string{"$_"},
		re: regexp.MustCompile(`(?i)\$_(?:GET|POST|REQUEST|COOKIE)\s*\[[^\]\n]{1,64}\]\s*\(`),
	},
	{
		// preg_replace's /e modifier evaluates the replacement (PHP < 7).
		name: "preg-replace-eval", severity: "high", hints: []string{"preg_replace"},
		re: regexp.MustCompile(`(?i)\bpreg_replace\s*\(\s*['"]([^\w\s\\])[^'"\n]{0,300}([^\w\s\\])[a-df-z]*e[a-z]*['"]\s*,`),
	},
	{
		// Includes code hidden in an image or text file, such as the
		// classic favicon_*.ico.
		name: "include-non-php", severity: "high", hints: []string{"include", "require"},
		re: regexp.MustCompile(`(?i)\b(?:include|require)(?:_once)?\s*\(?\s*['"][^'"\n]+\.(?:ico|png|gif|jpe?g|bmp|txt|svg)['"]`),
	},
	{
		// Names of well-known web shells.
		name: "webshell", severity: "high", hints: []string{"filesman", "b374k", "c99shell", "r57shell", "wso", "indoxploit", "alfa"},
		re: regexp.MustCompile(`(?i)\b(?:FilesMan|b374k|c99shell|r57shell|WSO\s?[0-9][0-9.]*|IndoXploit|AlfaShell|ALFA_DATA)\b`),
	},
	{
		// Code spelled out in hex escapes ("\x65\x76\x61\x6c") or chr()
		// chains that, decoded, calls what a shell calls. Libraries use
		// both for binary data, which doesn't decode to code.
		name: "encoded-call", severity: "high", hints: []string{`\x`, "chr"},
		find: findEncodedCall,
	},
	{
		// A base64 string that decodes to PHP code or such calls. Images
		// and fonts decode to binary.
		name: "encoded-payload", severity: "high", hints: nil,
		find: findEncodedPayload,
	},
}

// dangerousRe matches what decoded code a shell hides: calls that run
// code or commands, request input, or a PHP opening tag.
var dangerousRe = regexp.MustCompile(`(?i)\b(?:eval|assert|base64_decode|gzinflate|gzuncompress|str_rot13|system|shell_exec|passthru|exec|popen|proc_open|create_function)\s*\(|\$_(?:post|get|request|cookie)\b|<\?php`)

var (
	hexRunRe   = regexp.MustCompile(`(?:\\x[0-9a-fA-F]{2}){4,}`)
	hexByteRe  = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)
	chrChainRe = regexp.MustCompile(`(?i)(?:chr\s*\(\s*\d{1,3}\s*\)\s*\.\s*){3,}chr\s*\(\s*\d{1,3}\s*\)`)
	chrNumRe   = regexp.MustCompile(`\d{1,3}`)
)

// maxDecodedLine caps the line around a hex run that's decoded.
const maxDecodedLine = 4096

// findEncodedCall returns where the first run of hex escapes or chr() chain
// whose decoded line calls something dangerous starts and ends. A hex run
// is decoded with the rest of its line, as "ev\x61l" mixes both.
func findEncodedCall(b []byte) []int {
	for _, loc := range hexRunRe.FindAllIndex(b, -1) {
		start := max(bytes.LastIndexByte(b[:loc[0]], '\n')+1, loc[0]-maxDecodedLine/2)
		end := loc[1] + bytes.IndexByte(b[loc[1]:], '\n')
		if end < loc[1] || end-start > maxDecodedLine {
			end = min(len(b), start+maxDecodedLine)
		}
		line := b[start:end]
		decoded := hexByteRe.ReplaceAllFunc(line, func(m []byte) []byte {
			v, _ := strconv.ParseUint(string(m[2:]), 16, 8)
			return []byte{byte(v)}
		})
		// Only what decoding produced counts, not the line's own code.
		for _, m := range dangerousRe.FindAll(decoded, -1) {
			if !bytes.Contains(line, m) {
				return loc
			}
		}
	}
	for _, loc := range chrChainRe.FindAllIndex(b, -1) {
		var decoded []byte
		for _, n := range chrNumRe.FindAll(b[loc[0]:loc[1]], -1) {
			v, _ := strconv.Atoi(string(n))
			decoded = append(decoded, byte(v))
		}
		if dangerousRe.Match(decoded) {
			return loc
		}
	}
	return nil
}

// minBase64Run is how long a run of base64 characters is worth decoding.
const minBase64Run = 100

func isBase64Char(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/'
}

// findEncodedPayload returns where the first base64 string that decodes to
// dangerous code starts and ends.
func findEncodedPayload(b []byte) []int {
	for i := 0; i < len(b); {
		if !isBase64Char(b[i]) {
			i++
			continue
		}
		j := i
		for j < len(b) && isBase64Char(b[j]) {
			j++
		}
		if j-i >= minBase64Run {
			run := b[i:j]
			decoded := make([]byte, base64.StdEncoding.DecodedLen(len(run)))
			// Decode whole 4-character groups; padding may follow the run.
			n, _ := base64.StdEncoding.Decode(decoded, run[:len(run)/4*4])
			if dangerousRe.Match(decoded[:n]) {
				return []int{i, j}
			}
		}
		i = j
	}
	return nil
}

// contentMatch is a content rule that matched a file.
type contentMatch struct {
	pattern *contentPattern
	line    int
	excerpt string
}

// matchContent returns the content rules that match a PHP file's content,
// each with where it first matched.
func matchContent(body []byte) []contentMatch {
	if len(body) == 0 {
		return nil
	}
	lower := bytes.ToLower(body)
	var matches []contentMatch
	for i := range contentPatterns {
		p := &contentPatterns[i]
		if len(p.hints) > 0 && !containsAny(lower, p.hints) {
			continue
		}
		var loc []int
		if p.find != nil {
			loc = p.find(body)
		} else {
			loc = p.re.FindIndex(body)
		}
		if loc == nil {
			continue
		}
		excerpt := string(body[loc[0]:min(loc[1], loc[0]+maxMatchExcerpt)])
		matches = append(matches, contentMatch{
			pattern: p,
			line:    bytes.Count(body[:loc[0]], []byte("\n")) + 1,
			excerpt: strings.ToValidUTF8(excerpt, "�"),
		})
	}
	return matches
}

func containsAny(lower []byte, hints []string) bool {
	for _, h := range hints {
		if bytes.Contains(lower, []byte(h)) {
			return true
		}
	}
	return false
}

// contentFlags returns the names of the matched rules, which an entry
// keeps, and a finding detail quoting where each matched.
func contentFlags(matches []contentMatch) ([]string, string) {
	names := make([]string, len(matches))
	lines := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.pattern.name
		lines[i] = fmt.Sprintf("%s (line %d): %s", m.pattern.name, m.line, m.excerpt)
	}
	return names, strings.Join(lines, "\n")
}

// contentSeverity returns the worst severity of the named content rules.
func contentSeverity(names []string) string {
	rank := map[string]int{"low": 1, "medium": 2, "high": 3}
	worst := "low"
	for _, name := range names {
		for _, p := range contentPatterns {
			if p.name == name && rank[p.severity] > rank[worst] {
				worst = p.severity
			}
		}
	}
	return worst
}

// isContentArea reports whether the nightly content scan reads p: PHP
// files in wp-content and the site directory itself. Core's own
// directories are covered by its checksums.
func isContentArea(p string) bool {
	return IsPHPPath(p) && (strings.HasPrefix(p, "wp-content/") || !strings.Contains(p, "/"))
}
