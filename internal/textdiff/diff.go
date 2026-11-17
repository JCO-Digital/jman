package textdiff

// Line kinds in a Hunk.
const (
	KindContext = "context"
	KindRemoved = "removed"
	KindAdded   = "added"
)

// ContextLines is how many unchanged lines a hunk shows around changes.
const ContextLines = 2

// maxCells bounds the LCS table (lines before × lines after, after common
// prefix and suffix are removed). Beyond it, the differing middle is
// reported as removed and re-added rather than diffed line by line.
const maxCells = 4_000_000

// Line is one line of a hunk. Text is the line as it appeared on the page.
type Line struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Hunk is a run of changed lines with context around them.
type Hunk struct {
	Lines []Line `json:"lines"`
}

// Changes counts a hunk's removed and added lines.
func (h Hunk) Changes() int {
	n := 0
	for _, l := range h.Lines {
		if l.Kind != KindContext {
			n++
		}
	}
	return n
}

// Hide reports whether a changed line should be left out of a diff. kind
// is KindRemoved or KindAdded, norm the line's normalized form.
type Hide func(kind, norm string) bool

// Noise returns the normalized lines that differ between two fetches of a
// page taken without an update in between. They change on their own, so
// a diff should ignore them.
func Noise(a, b []string) map[string]bool {
	na, nb := normalizeAll(a), normalizeAll(b)
	noise := map[string]bool{}
	for _, o := range diffOps(na, nb) {
		switch o.kind {
		case '-':
			noise[na[o.a]] = true
		case '+':
			noise[nb[o.b]] = true
		}
	}
	return noise
}

// Diff compares two fetches of a page line by line, in normalized form,
// and returns the changes as hunks with ContextLines of context. Changed
// lines for which hide returns true are left out, and so are hunks left
// with no changes. hide may be nil.
func Diff(before, after []string, hide Hide) []Hunk {
	nb, na := normalizeAll(before), normalizeAll(after)
	all := diffOps(nb, na)

	ops := all[:0:0]
	for _, o := range all {
		if hide != nil {
			if o.kind == '-' && hide(KindRemoved, nb[o.a]) {
				continue
			}
			if o.kind == '+' && hide(KindAdded, na[o.b]) {
				continue
			}
		}
		ops = append(ops, o)
	}

	line := func(o op) Line {
		switch o.kind {
		case '-':
			return Line{Kind: KindRemoved, Text: before[o.a]}
		case '+':
			return Line{Kind: KindAdded, Text: after[o.b]}
		}
		return Line{Kind: KindContext, Text: after[o.b]}
	}

	var hunks []Hunk
	hunkEnd := 0 // where the previous hunk ended in ops
	for i := 0; i < len(ops); {
		if ops[i].kind == '=' {
			i++
			continue
		}
		// A change at i: start a hunk with up to ContextLines before it,
		// and extend it while the next change is close enough that the
		// context around both would touch.
		start := max(i-ContextLines, hunkEnd)
		end := i
		for end < len(ops) {
			if ops[end].kind != '=' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == '=' {
				run++
			}
			if run < len(ops) && run-end <= 2*ContextLines {
				end = run
				continue
			}
			end = min(end+ContextLines, len(ops))
			break
		}
		h := Hunk{}
		for _, o := range ops[start:end] {
			h.Lines = append(h.Lines, line(o))
		}
		hunks = append(hunks, h)
		hunkEnd = end
		i = end
	}
	return hunks
}

type op struct {
	kind byte // '=', '-' or '+'
	a, b int  // index in a (for '=' and '-') and b (for '=' and '+')
}

// diffOps returns an edit script turning a into b: the longest common
// subsequence stays ('='), the rest of a is removed ('-') and the rest of
// b added ('+'). Removals come before additions within a change.
func diffOps(a, b []string) []op {
	var ops []op
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		ops = append(ops, op{'=', pre, pre})
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]

	if len(ma)*len(mb) > maxCells {
		for i := range ma {
			ops = append(ops, op{'-', pre + i, 0})
		}
		for j := range mb {
			ops = append(ops, op{'+', 0, pre + j})
		}
	} else {
		// lcs[i][j] is the LCS length of ma[i:] and mb[j:].
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
		i, j := 0, 0
		for i < len(ma) || j < len(mb) {
			switch {
			case i < len(ma) && j < len(mb) && ma[i] == mb[j]:
				ops = append(ops, op{'=', pre + i, pre + j})
				i++
				j++
			case j == len(mb) || (i < len(ma) && lcs[i+1][j] >= lcs[i][j+1]):
				ops = append(ops, op{'-', pre + i, 0})
				i++
			default:
				ops = append(ops, op{'+', 0, pre + j})
				j++
			}
		}
	}

	for k := 0; k < suf; k++ {
		ops = append(ops, op{'=', len(a) - suf + k, len(b) - suf + k})
	}
	return ops
}
