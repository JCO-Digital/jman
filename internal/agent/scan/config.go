package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
)

// maxConfigBytes is how much of wp-config.php or the site root's .htaccess
// is kept, redacted, in the baseline to show a change as a diff.
const maxConfigBytes = 64 << 10

// maxConfigLines caps the lines compared, which keeps the diff cheap.
const maxConfigLines = 3000

// maxConfigDiff caps a config-change diff, which jman-api keeps as the
// finding's snippet (up to 2 KB).
const maxConfigDiff = 2000

// configContext is how many unchanged lines a diff shows around a change.
const configContext = 2

// redacted replaces a secret value in a config copy.
const redacted = "[redacted]"

// ConfigCopy is wp-config.php or the site root's .htaccess with its
// secrets taken out, kept in the baseline so a change can be shown as a
// diff. Neither it nor a diff of it holds a secret value: a value under a
// secret-looking name, and any random-looking string, is replaced, and
// only a hash of each secret is kept, to tell that it changed.
type ConfigCopy struct {
	Lines []string `json:"lines"`
	// Secrets are hashes of the lines that set each secret, by name.
	Secrets map[string]string `json:"secrets,omitempty"`
	// Truncated is set when the file is longer than what was kept.
	Truncated bool `json:"truncated,omitempty"`
}

var (
	// secretNameRe matches a name whose value is a secret.
	secretNameRe = regexp.MustCompile(`(?i)pass|pwd|key|salt|secret|token|auth|credential|private|licen[cs]e|api`)
	// configNameRes find a name a line sets, and so where its value
	// starts: a constant, a variable or array element, an array key, an
	// environment variable in PHP or .htaccess.
	defineRe = regexp.MustCompile(`(?i)\bdefine\s*\(\s*['"]([^'"]+)['"]\s*,`)
	assignRe = regexp.MustCompile(`\$([A-Za-z_]\w*)(?:\s*\[\s*['"]?([^'"\]]*)['"]?\s*\])*\s*(?:=[^=>]|\.=|\?\?=)`)
	arrowRe  = regexp.MustCompile(`['"]([^'"]+)['"]\s*=>`)
	putenvRe = regexp.MustCompile(`(?i)\bputenv\s*\(\s*(['"])([A-Za-z_]\w*)=`)
	setEnvRe = regexp.MustCompile(`(?i)^\s*SetEnv(?:If(?:NoCase)?)?\s+(\S+)\s+`)
	bareRe   = regexp.MustCompile(`\S{20,}`)
)

// configName is a name a line sets and where its value starts, or for a
// putenv() call, its value's span.
type configName struct {
	name       string
	start, end int // end is -1 when the value runs to the end of the line
}

// lineNames returns the names a config line sets.
func lineNames(line string) []configName {
	var names []configName
	for _, m := range defineRe.FindAllStringSubmatchIndex(line, -1) {
		names = append(names, configName{line[m[2]:m[3]], m[1], -1})
	}
	for _, m := range assignRe.FindAllStringSubmatchIndex(line, -1) {
		name := line[m[2]:m[3]]
		if m[4] >= 0 && m[5] > m[4] {
			name = line[m[4]:m[5]]
		}
		names = append(names, configName{name, m[1] - 1, -1})
	}
	for _, m := range arrowRe.FindAllStringSubmatchIndex(line, -1) {
		names = append(names, configName{line[m[2]:m[3]], m[1], -1})
	}
	for _, m := range putenvRe.FindAllStringSubmatchIndex(line, -1) {
		end := strings.IndexByte(line[m[1]:], line[m[2]])
		if end < 0 {
			end = len(line) - m[1]
		}
		names = append(names, configName{line[m[4]:m[5]], m[1], m[1] + end})
	}
	if m := setEnvRe.FindStringSubmatchIndex(line); m != nil {
		names = append(names, configName{line[m[2]:m[3]], m[1], -1})
	}
	return names
}

// literalSpans returns the spans of what the quoted strings in a line
// hold, without the quotes; one left open runs to the end of the line.
func literalSpans(line string) [][2]int {
	var spans [][2]int
	for i := 0; i < len(line); i++ {
		q := line[i]
		if q != '\'' && q != '"' {
			continue
		}
		j := i + 1
		for j < len(line) && line[j] != q {
			if line[j] == '\\' {
				j++
			}
			j++
		}
		j = min(j, len(line))
		spans = append(spans, [2]int{i + 1, j})
		i = j
	}
	return spans
}

// redactConfigLine returns a config line with its secret values replaced,
// and the secret names it sets. carry names a secret the line before set
// without a value, which is then on this line; next is such a name on
// this line, if any.
func redactConfigLine(line string, htaccess bool, carry string) (red string, secrets []string, next string) {
	type cut struct{ start, end int }
	var cuts []cut
	if carry != "" {
		secrets = append(secrets, carry)
		for _, s := range literalSpans(line) {
			cuts = append(cuts, cut{s[0], s[1]})
		}
	}
	for _, n := range lineNames(line) {
		if !secretNameRe.MatchString(n.name) {
			continue
		}
		secrets = append(secrets, n.name)
		if n.end >= 0 {
			cuts = append(cuts, cut{n.start, n.end})
			continue
		}
		if strings.TrimSpace(line[n.start:]) == "" {
			next = n.name
			continue
		}
		var found bool
		for _, s := range literalSpans(line[n.start:]) {
			cuts = append(cuts, cut{n.start + s[0], n.start + s[1]})
			found = true
		}
		if !found {
			// An unquoted value, as SetEnv takes: everything after the
			// name, but the end of a PHP statement.
			end := len(line)
			if !htaccess {
				end = n.start + len(strings.TrimRight(line[n.start:], " \t;)"))
			}
			if end > n.start {
				cuts = append(cuts, cut{n.start, end})
			}
		}
	}
	for _, s := range literalSpans(line) {
		if looksRandom(line[s[0]:s[1]]) {
			cuts = append(cuts, cut{s[0], s[1]})
		}
	}
	if htaccess {
		for _, m := range bareRe.FindAllStringIndex(line, -1) {
			if looksRandom(line[m[0]:m[1]]) {
				cuts = append(cuts, cut{m[0], m[1]})
			}
		}
	}
	if len(cuts) == 0 {
		return line, secrets, next
	}
	slices.SortFunc(cuts, func(a, b cut) int { return a.start - b.start })
	var b strings.Builder
	pos := 0
	for _, c := range cuts {
		if c.end <= pos || c.end <= c.start {
			continue
		}
		start := max(c.start, pos)
		b.WriteString(line[pos:start])
		b.WriteString(redacted)
		pos = c.end
	}
	b.WriteString(line[pos:])
	return b.String(), secrets, next
}

// looksRandom reports whether s looks like a key, salt or token rather
// than a word, a path or a URL: long, without spaces, and either hex or
// with characters spread out like random ones.
func looksRandom(s string) bool {
	if len(s) < 20 || strings.ContainsAny(s, " \t") || s == redacted {
		return false
	}
	hex, letters, digits := true, false, false
	counts := map[rune]int{}
	n := 0
	for _, r := range s {
		counts[r]++
		n++
		switch {
		case r >= '0' && r <= '9':
			digits = true
		case r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F':
			letters = true
		default:
			hex = false
		}
	}
	if hex && letters && digits && n >= 24 {
		return true
	}
	var entropy float64
	for _, c := range counts {
		p := float64(c) / float64(n)
		entropy -= p * math.Log2(p)
	}
	return entropy >= 4.2
}

// newConfigCopy returns the redacted copy of a config file's first
// maxConfigBytes; size is the whole file's.
func newConfigCopy(rel string, content []byte, size int64) *ConfigCopy {
	htaccess := rel == ".htaccess"
	text := strings.ToValidUTF8(string(content), "�")
	truncated := size > int64(len(content))
	if truncated {
		// Leave out the line the cut ends in the middle of.
		if i := strings.LastIndexByte(text, '\n'); i >= 0 {
			text = text[:i+1]
		}
	}
	text = strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > maxConfigLines {
		lines, truncated = lines[:maxConfigLines], true
	}
	c := &ConfigCopy{Lines: make([]string, len(lines)), Truncated: truncated}
	secretLines := map[string][]string{}
	carry := ""
	for i, line := range lines {
		var red string
		var names []string
		red, names, carry = redactConfigLine(line, htaccess, carry)
		c.Lines[i] = red
		for _, name := range names {
			secretLines[name] = append(secretLines[name], line)
		}
	}
	if len(secretLines) > 0 {
		c.Secrets = make(map[string]string, len(secretLines))
		for name, set := range secretLines {
			h := sha256.Sum256([]byte(name + "\x00" + strings.Join(set, "\n")))
			c.Secrets[name] = hex.EncodeToString(h[:])
		}
	}
	return c
}

// changedSecrets returns the secret names added, removed or set to
// another value between two copies.
func changedSecrets(old, cur *ConfigCopy) []string {
	var names []string
	for name, h := range cur.Secrets {
		if old.Secrets[name] != h {
			names = append(names, name)
		}
	}
	for name := range old.Secrets {
		if _, ok := cur.Secrets[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// configDiff returns a unified diff between two redacted copies, without
// the file headers, cut to maxConfigDiff bytes. old may be nil, for a
// new file.
func configDiff(old, cur *ConfigCopy) string {
	var a []string
	if old != nil {
		a = old.Lines
	}
	b := cur.Lines
	ops := diffLines(a, b)

	var out strings.Builder
	var lines []string
	// Group the changes into hunks with configContext lines around them.
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := max(0, i-configContext)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			// Unchanged lines: does another change follow closely?
			j := end
			for j < len(ops) && ops[j].kind == ' ' && j-end < 2*configContext {
				j++
			}
			if j < len(ops) && ops[j].kind != ' ' {
				end = j
				continue
			}
			break
		}
		stop := min(len(ops), end+configContext)
		lines = append(lines, fmt.Sprintf("@@ line %d @@", ops[start].line))
		for _, op := range ops[start:stop] {
			lines = append(lines, string(op.kind)+op.text)
		}
		i = stop
	}
	for n, line := range lines {
		more := fmt.Sprintf("… %d more lines", len(lines)-n)
		if out.Len()+len(line)+1 > maxConfigDiff-len(more) {
			out.WriteString(more)
			break
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// diffOp is one line of a diff: ' ' kept, '-' removed, '+' added. line is
// its number in the new file (for a removed line, where it was).
type diffOp struct {
	kind byte
	text string
	line int
}

// diffLines returns a line diff of a and b, by longest common
// subsequence after their common start and end.
func diffLines(a, b []string) []diffOp {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]

	// lcs[i][j] is the length of the common subsequence of ma[i:], mb[j:].
	lcs := make([][]int32, len(ma)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(mb)+1)
	}
	for i := len(ma) - 1; i >= 0; i-- {
		for j := len(mb) - 1; j >= 0; j-- {
			if ma[i] == mb[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	ops := make([]diffOp, 0, len(a)+len(b))
	line := 1
	for _, t := range b[:pre] {
		ops = append(ops, diffOp{' ', t, line})
		line++
	}
	i, j := 0, 0
	for i < len(ma) || j < len(mb) {
		switch {
		case i < len(ma) && j < len(mb) && ma[i] == mb[j]:
			ops = append(ops, diffOp{' ', mb[j], line})
			i, j, line = i+1, j+1, line+1
		case j < len(mb) && (i == len(ma) || lcs[i][j+1] >= lcs[i+1][j]):
			ops = append(ops, diffOp{'+', mb[j], line})
			j, line = j+1, line+1
		default:
			ops = append(ops, diffOp{'-', ma[i], line})
			i++
		}
	}
	for _, t := range b[len(b)-suf:] {
		ops = append(ops, diffOp{' ', t, line})
		line++
	}
	return ops
}

// configChange describes how a config file changed from old to e, as a
// detail to add to the finding and a diff for its snippet. There's no
// diff when the baseline has no copy of the file, as after an upgrade.
func configChange(old, e Entry, existed bool) (detail, diff string) {
	if e.Config == nil {
		return "", ""
	}
	var prev *ConfigCopy
	if existed {
		if prev = old.Config; prev == nil {
			return "", ""
		}
	} else {
		prev = &ConfigCopy{}
	}
	if names := changedSecrets(prev, e.Config); len(names) > 0 && existed {
		detail = "; secrets changed (values hidden): " + strings.Join(names, ", ")
	}
	diff = configDiff(prev, e.Config)
	if e.Config.Truncated || prev.Truncated {
		detail += fmt.Sprintf("; only the first %d KB compared", maxConfigBytes>>10)
	}
	return detail, diff
}
