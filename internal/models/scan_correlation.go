package models

import (
	"fmt"
	"sort"
	"strings"
)

// Scan rules by the kind of compromise they show, for correlation.
var (
	// scanLocationRules flag a PHP file where none belongs, or one that
	// differs from its release.
	scanLocationRules = map[string]bool{
		"uploads-php": true, "core-unknown-file": true, "core-modified": true,
		"plugin-modified": true, "plugin-unknown-file": true,
		"mu-plugin-change": true, "dropin-change": true,
	}
	// scanSignalKinds sorts the rules that count towards two kinds of
	// compromise on one site.
	scanSignalKinds = map[string]string{
		"uploads-php": "code", "core-unknown-file": "code", "core-modified": "code",
		"plugin-modified": "code", "plugin-unknown-file": "code", "mu-plugin-change": "code",
		"dropin-change": "code", "auto-prepend": "code", "suspicious-code": "code",
		"new-admin": "accounts", "new-app-password": "accounts",
		"persistence": "server", "home-binary": "server", "disguised-process": "server",
	}
)

// CorrelateScanFindings returns why a security incident's open findings
// together are worse than any one of them, or "": the same file flagged
// for where it is and for what it contains, or two kinds of compromise
// (code, accounts, the server) on one site. Only high and critical
// findings count, so what's informational or trusted (a staff admin, an
// editor's server) never escalates.
func CorrelateScanFindings(findings []StoredScanFinding) string {
	located := map[string]bool{}
	malicious := map[string]bool{}
	kinds := map[string]string{}
	for _, f := range findings {
		if f.Status != ScanFindingOpen || (f.Severity != "high" && f.Severity != "critical") {
			continue
		}
		if scanLocationRules[f.Rule] {
			located[f.Path] = true
		}
		if f.Rule == "suspicious-code" {
			malicious[f.Path] = true
		}
		if kind := scanSignalKinds[f.Rule]; kind != "" && kinds[kind] == "" {
			kinds[kind] = f.Rule + " " + f.Path
		}
	}
	var both []string
	for p := range malicious {
		if located[p] {
			both = append(both, p)
		}
	}
	if len(both) > 0 {
		sort.Strings(both)
		more := ""
		if len(both) > 1 {
			more = fmt.Sprintf(" and %d more", len(both)-1)
		}
		return fmt.Sprintf("malicious code in a file that shouldn't be there: %s%s", both[0], more)
	}
	if len(kinds) >= 2 {
		var names, examples []string
		for _, kind := range []string{"code", "accounts", "server"} {
			if example := kinds[kind]; example != "" {
				names = append(names, kind)
				examples = append(examples, example)
			}
		}
		return fmt.Sprintf("compromise of %s: %s", strings.Join(names, " and "), strings.Join(examples, "; "))
	}
	return ""
}
