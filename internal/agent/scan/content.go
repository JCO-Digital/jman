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
	// inComments makes re match in comments too; other patterns skip
	// matches on comment lines, such as a doc comment's
	// "@var $_GET['action'] (string)".
	inComments bool
}

// contentPatterns are the built-in content rules, after the ones PHP
// Malware Finder and similar scanners use, narrowed to what's rare in
// plugins and themes.
var contentPatterns = []contentPattern{
	{
		// Runs code or a command hidden in an encoded string.
		name: "eval-decoded", severity: "high", hints: []string{"eval", "assert", "exec", "system", "passthru", "popen"},
		re: regexp.MustCompile(`(?i)(?:^|[^\w>:$\\])(?:eval|assert|shell_exec|system|passthru|exec|popen|proc_open)\s*\(\s*(?:@\s*)?(?:base64_decode|gzinflate|gzuncompress|gzdecode|str_rot13|strrev|hex2bin|rawurldecode|urldecode|convert_uudecode)\s*\(`),
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
		name: "webshell", severity: "high", hints: []string{"filesman", "b374k", "c99shell", "r57shell", "wso", "indoxploit", "alfa"}, inComments: true,
		re: regexp.MustCompile(`(?i)\b(?:FilesMan|b374k|c99shell|r57shell|WSO\s?[0-9][0-9.]*|IndoXploit|Alfa\s?Shell|ALFA_DATA)\b`),
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
	// The rules below look for two things in the same file, each common
	// enough alone. They come from backdoor kits found on managed sites
	// in 2026, which none of the rules above caught.
	{
		// Decodes request input and runs a variable:
		// $d = base64_decode($_POST['d']); shell_exec($d).
		name: "exec-decoded-input", severity: "high", hints: []string{"base64_decode"},
		find: func(b []byte) []int { return findBoth(b, decodedInputRe, execVarRe) },
	},
	{
		// Fetches code from a URL and evals it: eval("?>" . $remote).
		name: "eval-remote", severity: "high", hints: []string{"eval"},
		find: func(b []byte) []int { return findAfter(b, remoteFetchRe, evalVarRe, maxFetchDistance) },
	},
	{
		// Fetches code from a URL into a temporary file and includes it.
		name: "include-remote", severity: "high", hints: []string{"tmpfile", "tempnam", "sys_get_temp_dir"},
		find: func(b []byte) []int {
			if !tempFileRe.Match(b) {
				return nil
			}
			return findBoth(b, remoteFetchRe, includeVarRe)
		},
	},
	{
		// Checks request input against a password hash written into the
		// file: the door of a password-protected shell or a login bypass.
		// Plugins keep password hashes in the database.
		name: "hardcoded-credential", severity: "high", hints: []string{"password_verify", "hash_equals"},
		find: findHardcodedCredential,
	},
	{
		// Strings cut into pieces to hide them from searches:
		// 'goo'.'gle'.'bot', 'wp'.'-lo'.'ad.php'. Code doesn't need that.
		name: "split-strings", severity: "high", hints: []string{"'.'", "' . '", `"."`, `" . "`},
		re: regexp.MustCompile(`(?:'[^'\\\n]{1,4}'\s*\.\s*){8,}'[^'\\\n]{1,4}'|(?:"[^"\\\n$]{1,4}"\s*\.\s*){8,}"[^"\\\n$]{1,4}"`),
	},
}

var (
	// decodedInputRe matches base64_decode() of request input.
	decodedInputRe = regexp.MustCompile(`(?i)base64_decode\s*\(\s*(?:@\s*)?(?:isset\s*\(\s*)?\$_(?:GET|POST|REQUEST|COOKIE|SERVER)\b`)
	// execVarRe matches a function that runs code or commands called on a
	// variable, not as a method (PDO's ->exec($sql)).
	execVarRe = regexp.MustCompile(`(?i)(?:^|[^\w>:$\\])(?:eval|assert|shell_exec|system|passthru|exec|popen|proc_open)\s*\(\s*(?:@\s*)?(?:base64_decode\s*\(\s*)?\$[A-Za-z_]`)
	// evalVarRe matches eval() of a variable, possibly after a short
	// string such as "?>".
	evalVarRe = regexp.MustCompile(`(?i)(?:^|[^\w>:$\\])eval\s*\(\s*(?:@\s*)?(?:(?:"[^"\n]{0,16}"|'[^'\n]{0,16}'|\$\w+)\s*\.\s*)?\$\w`)
	// includeVarRe matches include or require of a variable.
	includeVarRe = regexp.MustCompile(`(?i)(?:^|[^\w>:$\\])(?:include|require)(?:_once)?\s*\(?\s*\$\w`)
	// remoteFetchRe matches fetching something over HTTP.
	remoteFetchRe = regexp.MustCompile(`(?i)\bcurl_exec\s*\(|\bwp_remote_(?:get|post|request|retrieve_body)\s*\(|\b(?:file_get_contents|fopen)\s*\(\s*['"]https?://`)
	tempFileRe    = regexp.MustCompile(`(?i)\b(?:tmpfile|tempnam|sys_get_temp_dir)\s*\(`)

	bcryptRe        = regexp.MustCompile(`\$2[aby]\$\d\d\$[./A-Za-z0-9]{53}`)
	hexDigestRe     = regexp.MustCompile(`['"](?:[0-9a-fA-F]{40}|[0-9a-fA-F]{32})['"]`)
	passwordCheckRe = regexp.MustCompile(`(?i)\bpassword_verify\s*\(`)
	hashEqualsRe    = regexp.MustCompile(`(?i)\bhash_equals\s*\(`)
	hashedInputRe   = regexp.MustCompile(`(?i)\b(?:sha1|md5)\s*\(\s*\$_(?:GET|POST|COOKIE|REQUEST)\b|\bhash\s*\(\s*['"]\w+['"]\s*,\s*\$_(?:GET|POST|COOKIE|REQUEST)\b`)
	requestInputRe  = regexp.MustCompile(`\$_(?:GET|POST|COOKIE|REQUEST)\b`)
)

// maxFetchDistance is how far before an eval() eval-remote looks for the
// fetch of what it runs.
const maxFetchDistance = 2048

// findAfter returns where second first matches within distance bytes after
// a match of first.
func findAfter(b []byte, first, second *regexp.Regexp, distance int) []int {
	for _, loc := range second.FindAllIndex(b, -1) {
		if first.Match(b[max(0, loc[0]-distance):loc[0]]) {
			return loc
		}
	}
	return nil
}

// findBoth returns where second first matches if first matches too.
func findBoth(b []byte, first, second *regexp.Regexp) []int {
	if !first.Match(b) {
		return nil
	}
	return second.FindIndex(b)
}

// findHardcodedCredential returns where a password hash written into the
// file is, when the file checks request input against it: a bcrypt hash
// with password_verify(), or a hex digest with hash_equals() of a hashed
// request value.
func findHardcodedCredential(b []byte) []int {
	if !requestInputRe.Match(b) {
		return nil
	}
	if loc := bcryptRe.FindIndex(b); loc != nil && passwordCheckRe.Match(b) {
		return loc
	}
	if loc := hexDigestRe.FindIndex(b); loc != nil && hashEqualsRe.Match(b) && hashedInputRe.Match(b) {
		return loc
	}
	return nil
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
		switch {
		case p.find != nil:
			loc = p.find(body)
		case p.inComments:
			loc = p.re.FindIndex(body)
		default:
			for _, l := range p.re.FindAllIndex(body, -1) {
				if !onCommentLine(body, l[0]) {
					loc = l
					break
				}
			}
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

// onCommentLine reports whether the line holding offset i starts as a
// comment: //, #, /* or a doc comment's *.
func onCommentLine(b []byte, i int) bool {
	start := bytes.LastIndexByte(b[:i], '\n') + 1
	line := bytes.TrimLeft(b[start:i], " \t")
	return bytes.HasPrefix(line, []byte("//")) || bytes.HasPrefix(line, []byte("#")) || bytes.HasPrefix(line, []byte("*")) || bytes.HasPrefix(line, []byte("/*"))
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
