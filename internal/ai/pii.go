package ai

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// personalDataPatterns find common kinds of personal data and secrets. They
// guard requests that leave zero data retention (batches); a match sends
// the request through the regular API instead. The agent is responsible for
// leaving personal data out; this catches mistakes.
var personalDataPatterns = []struct {
	name string
	re   *regexp.Regexp
	ok   func(match string) bool
}{
	{name: "email address", re: regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)},
	{name: "phone number", re: regexp.MustCompile(`(\+\d{1,3}[ -]?\(?\d{1,4}\)?([ -]?\d{2,4}){2,4})|\b0\d{1,2}[ -]\d{3}[ -]?\d{3,4}\b`)},
	// Finnish personal identity code: DDMMYY, century sign, number, check.
	{name: "personal identity code", re: regexp.MustCompile(`\b(0[1-9]|[12]\d|3[01])(0[1-9]|1[0-2])\d{2}[-+A-FU-Y]\d{3}[0-9A-Y]\b`)},
	{name: "IBAN", re: regexp.MustCompile(`\b[A-Z]{2}\d{2}( ?[A-Z0-9]{4}){3,7}( ?[A-Z0-9]{1,4})?\b`)},
	{name: "payment card number", re: regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`), ok: notLuhn},
	{name: "API key", re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{10,}|\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`)},
}

// findPersonalData returns the kinds of personal data found in the text of
// a request: its system prompt, messages and tool definitions.
func findPersonalData(params anthropic.MessageNewParams) []string {
	raw, err := json.Marshal(struct {
		System   any `json:"system"`
		Messages any `json:"messages"`
		Tools    any `json:"tools"`
	}{params.System, params.Messages, params.Tools})
	if err != nil {
		return []string{"unreadable request"}
	}
	// Undo JSON escaping so patterns see the text as written.
	var text strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			text.WriteString(x)
			text.WriteByte('\n')
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return []string{"unreadable request"}
	}
	walk(v)
	return findPersonalDataInText(text.String())
}

func findPersonalDataInText(text string) []string {
	found := map[string]bool{}
	for _, p := range personalDataPatterns {
		for _, m := range p.re.FindAllString(text, -1) {
			if p.ok != nil && p.ok(m) {
				continue
			}
			found[p.name] = true
			break
		}
	}
	out := make([]string, 0, len(found))
	for name := range found {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// notLuhn reports whether a digit run fails the Luhn check, so isn't a
// card number.
func notLuhn(s string) bool {
	sum, double := 0, false
	digits := 0
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			continue
		}
		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
		digits++
	}
	return digits < 13 || sum%10 != 0
}
