package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
)

// RuleSetVersion identifies the built-in rules. Bump it when a rule
// changes what it flags, so jman-api can tell a burst of findings after an
// upgrade from an attack.
const RuleSetVersion = "5"

// Engine is the engine name built-in findings carry.
const Engine = "builtin"

// Rules and their severities.
const (
	RuleUploadsPHP     = "uploads-php"
	RuleMuPlugin       = "mu-plugin-change"
	RuleDropin         = "dropin-change"
	RuleConfig         = "config-change"
	RuleAutoPrepend    = "auto-prepend"
	RuleForeignOwner   = "foreign-owner"
	RuleStrayFile      = "stray-file"
	RuleUnreadable     = "unreadable"
	RuleCoreModified   = "core-modified"
	RuleCoreUnknown    = "core-unknown-file"
	RulePluginModified = "plugin-modified"
	RulePluginUnknown  = "plugin-unknown-file"
)

var severities = map[string]string{
	RuleUploadsPHP:     "high",
	RuleMuPlugin:       "medium",
	RuleDropin:         "medium",
	RuleConfig:         "medium",
	RuleAutoPrepend:    "high",
	RuleForeignOwner:   "medium",
	RuleStrayFile:      "low",
	RuleUnreadable:     "low",
	RuleCoreModified:   "high",
	RuleCoreUnknown:    "high",
	RulePluginModified: "high",
	RulePluginUnknown:  "high",
}

// isCoreRule reports whether a checksum rule is one of core's.
func isCoreRule(rule string) bool {
	return rule == RuleCoreModified || rule == RuleCoreUnknown
}

// signature is what has to change for a tracked file to count as changed:
// its hash, or for a file that has none (a symlink, or one too large or
// unreadable to hash), its size, target and readability.
func signature(e Entry) string {
	if e.SHA256 != "" {
		return e.SHA256
	}
	return fmt.Sprintf("%d|%s|%t", e.Size, e.Symlink, e.Unreadable)
}

// Evaluate compares the entries of a walk with the previous baseline and
// returns what the rules flag. Only changes are flagged: a file that was
// already in the baseline, unchanged, isn't flagged again. ownerUID is the
// site directory's owner; userName names a uid for the findings.
func Evaluate(prev map[string]Entry, cur []Entry, ownerUID uint32, userName func(uint32) string) []models.ScanFinding {
	var findings []models.ScanFinding
	for _, e := range cur {
		old, existed := prev[e.Path]
		changed := !existed || signature(old) != signature(e)
		flag := func(rule, detail string) {
			f := models.ScanFinding{
				Engine:   Engine,
				Rule:     rule,
				Severity: severities[rule],
				Path:     e.Path,
				SHA256:   e.SHA256,
				Snippet:  e.Head,
				Detail:   detail,
			}
			if e.Symlink != "" {
				f.Snippet = ""
				f.Detail = strings.TrimPrefix(f.Detail+"; symlink to "+e.Symlink, "; ")
			}
			findings = append(findings, f)
		}
		change := "new"
		if existed {
			change = "changed"
		}

		if !e.Dir {
			switch {
			case isUploadsPHP(e.Path) && changed:
				// Plugins leave empty index.php files all over uploads, and
				// keep data in .php files that exit before anything else
				// (see isGuarded). Neither runs anything; each is flagged
				// once it does.
				if (e.Size > 0 && !isGuarded(e.Head)) || e.Symlink != "" || e.Unreadable {
					flag(RuleUploadsPHP, change+" PHP file in uploads")
				}
			case isMuPlugin(e.Path) && changed:
				flag(RuleMuPlugin, change+" file in mu-plugins")
			case isDropin(e.Path) && changed:
				flag(RuleDropin, change+" drop-in")
			case isConfig(e.Path) && changed:
				flag(RuleConfig, change+" "+e.Path)
			}
			if len(e.Directives) > 0 && changed {
				flag(RuleAutoPrepend, strings.Join(e.Directives, "\n"))
			}
			if e.Checksum != "" && (changed || old.Checksum != e.Checksum) {
				switch e.Checksum {
				case RuleCoreModified:
					flag(RuleCoreModified, change+" core file that differs from WordPress's checksum")
				case RuleCoreUnknown:
					flag(RuleCoreUnknown, change+" file that isn't part of WordPress core")
					if !isPHP(path.Base(e.Path)) {
						// Leftovers from old releases are more likely than
						// shells outside PHP files.
						findings[len(findings)-1].Severity = "medium"
					}
				case RulePluginModified:
					flag(RulePluginModified, change+" plugin file that differs from its checksum on wordpress.org")
				case RulePluginUnknown:
					flag(RulePluginUnknown, change+" PHP file that isn't part of the plugin's release on wordpress.org")
				}
			}
		}

		// A root-owned symlink is the server's own doing (SpinupWP links
		// wp-content/debug.log into the site's logs directory); the site's
		// owner can't create one.
		rootSymlink := e.Symlink != "" && e.UID == 0
		if e.UID != ownerUID && !rootSymlink && (!existed || old.UID != e.UID) {
			flag(RuleForeignOwner, fmt.Sprintf("owned by %s, not the site's owner %s", userName(e.UID), userName(ownerUID)))
		}
		if !existed && ((e.Dir && isStrayDir(e.Path)) || (!e.Dir && isStrayFile(e.Path))) {
			flag(RuleStrayFile, fmt.Sprintf("%d bytes", e.Size))
		}
		if e.Unreadable && (!existed || !old.Unreadable) {
			flag(RuleUnreadable, "the site's owner can't read it")
		}
	}
	return findings
}

// stateRules flag a state rather than a change, so they're worth
// reporting on a first scan too, when there's nothing to compare against.
// (The service's first scan has no checksums for core files yet.)
var stateRules = map[string]bool{RuleForeignOwner: true, RuleStrayFile: true, RuleUnreadable: true, RuleCoreModified: true, RuleCoreUnknown: true, RulePluginModified: true, RulePluginUnknown: true}

// EvaluateFirst returns what the rules flag on a scan without a baseline:
// only the state rules. The change rules stay quiet until there is a
// baseline, or everything already on the site would count as new.
func EvaluateFirst(cur []Entry, ownerUID uint32, userName func(uint32) string) []models.ScanFinding {
	var findings []models.ScanFinding
	for _, f := range Evaluate(nil, cur, ownerUID, userName) {
		if stateRules[f.Rule] {
			findings = append(findings, f)
		}
	}
	return findings
}

// Fingerprint hashes a baseline's entries, independent of their order.
func Fingerprint(entries []Entry) string {
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.Path + "\x00" + strconv.FormatUint(uint64(e.UID), 10) + "\x00" + strconv.FormatBool(e.Dir) + "\x00" + signature(e)
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, line := range lines {
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Details of cleared findings.
const (
	ClearedRemoved  = "removed"
	ClearedFixed    = "no longer flagged"
	ClearedReplaced = "replaced"
	ClearedOwner    = "now owned by the site's owner"
	ClearedReadable = "now readable"
)

// Cleared returns what the previous baseline flagged that the walk shows
// no longer holds. exists reports whether a path is still on disk: a file
// the walk stopped recording was removed, or no longer interesting (a
// foreign-owned file given back to the site's owner). complete is false
// when the walk hit its caps, so a missing path proves nothing.
func Cleared(prev map[string]Entry, cur []Entry, ownerUID uint32, complete bool, exists func(string) bool) []models.ScanCleared {
	var cleared []models.ScanCleared
	seen := make(map[string]bool, len(cur))
	for _, e := range cur {
		seen[e.Path] = true
		old, ok := prev[e.Path]
		if !ok {
			continue
		}
		if old.UID != ownerUID && e.UID == ownerUID {
			cleared = append(cleared, models.ScanCleared{Path: e.Path, Rule: RuleForeignOwner, Detail: ClearedOwner})
		}
		if old.Unreadable && !e.Unreadable {
			cleared = append(cleared, models.ScanCleared{Path: e.Path, Rule: RuleUnreadable, Detail: ClearedReadable})
		}
		if !old.Dir && signature(old) != signature(e) {
			for _, rule := range changeRules(old) {
				cleared = append(cleared, models.ScanCleared{Path: e.Path, Rule: rule, SHA256: old.SHA256, Detail: ClearedReplaced})
			}
		}
	}
	if !complete {
		return cleared
	}
	var gone []string
	for p := range prev {
		if !seen[p] {
			gone = append(gone, p)
		}
	}
	sort.Strings(gone)
	for _, p := range gone {
		detail := ClearedRemoved
		if exists(p) {
			detail = ClearedFixed
		}
		cleared = append(cleared, models.ScanCleared{Path: p, Detail: detail})
	}
	return cleared
}

// changeRules are the change rules whose findings for a file are about its
// content.
func changeRules(e Entry) []string {
	var rules []string
	switch {
	case isUploadsPHP(e.Path):
		rules = append(rules, RuleUploadsPHP)
	case isMuPlugin(e.Path):
		rules = append(rules, RuleMuPlugin)
	case isDropin(e.Path):
		rules = append(rules, RuleDropin)
	case isConfig(e.Path):
		rules = append(rules, RuleConfig)
	}
	if len(e.Directives) > 0 {
		rules = append(rules, RuleAutoPrepend)
	}
	if e.Checksum != "" {
		rules = append(rules, e.Checksum)
	}
	return rules
}
