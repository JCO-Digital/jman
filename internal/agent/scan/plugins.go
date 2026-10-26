package scan

import (
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

// PluginChecksums are the SHA-256 checksums of a plugin release's PHP
// files, keyed by path relative to the plugin's directory, as
// wordpress.org publishes them. A file can have more than one.
type PluginChecksums struct {
	Slug    string              `json:"slug"`
	Version string              `json:"version"`
	SHA256  map[string][]string `json:"sha256"`
}

// maxPluginModified is how many of a plugin's PHP files may differ from
// their checksums before the walk takes it for an update in progress and
// leaves the plugin out.
const maxPluginModified = 50

// maxPluginUnknown is how many PHP files a plugin may have that its
// wordpress.org release doesn't before the walk takes it for another build
// with the same slug (a premium version adding a pro/ directory, say) and
// leaves it out. An attacker drops a few files, not dozens.
const maxPluginUnknown = 20

// pluginsDir is where the plugins checked against checksums live.
const pluginsDir = "wp-content/plugins/"

// maxPluginHeaderBytes is how much of a plugin's PHP files is searched for
// its header, as WordPress reads it (get_file_data reads 8 KB).
const maxPluginHeaderBytes = 8 << 10

var (
	pluginNameRe    = regexp.MustCompile(`(?mi)^(?:[ \t]*<\?php)?[ \t/*#@]*Plugin Name:(.*)$`)
	pluginVersionRe = regexp.MustCompile(`(?mi)^(?:[ \t]*<\?php)?[ \t/*#@]*Version:(.*)$`)
	updateURIRe     = regexp.MustCompile(`(?mi)^(?:[ \t]*<\?php)?[ \t/*#@]*Update URI:(.*)$`)
)

// headerValue returns a plugin header's value, trimmed as WordPress does.
func headerValue(m [][]byte) string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(string(m[1])), "*/"))
}

// updatesFromWPOrg reports whether a plugin's Update URI header leaves its
// updates to wordpress.org: unset, or pointing there. Premium builds that
// reuse a wordpress.org slug set it to their own site, or "false".
func updatesFromWPOrg(uri string) bool {
	if uri == "" {
		return true
	}
	host := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(uri), "https://"), "http://")
	host, _, _ = strings.Cut(host, "/")
	return host == "w.org" || host == "wordpress.org"
}

// readPluginVersions returns the version of each plugin in a directory of
// wp-content/plugins, by slug, read from the header of its main file the
// way WordPress finds it: a PHP file directly in the plugin's directory
// with a "Plugin Name:" header. Plugins without a version are left out, and
// so are the ones whose Update URI says they don't come from wordpress.org,
// which are listed in elsewhere.
func readPluginVersions(root string) (versions map[string]string, elsewhere []string) {
	dirs, err := os.ReadDir(root + "/" + pluginsDir)
	if err != nil {
		return nil, nil
	}
	versions = map[string]string{}
	for _, d := range dirs {
		if !d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			continue
		}
		files, err := os.ReadDir(root + "/" + pluginsDir + d.Name())
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.Type().IsRegular() || !strings.HasSuffix(strings.ToLower(f.Name()), ".php") {
				continue
			}
			b, ok := readSmallFile(root+"/"+pluginsDir+d.Name()+"/"+f.Name(), maxPluginHeaderBytes)
			if !ok || pluginNameRe.Find(b) == nil {
				continue
			}
			if uri := headerValue(updateURIRe.FindSubmatch(b)); !updatesFromWPOrg(uri) {
				elsewhere = append(elsewhere, d.Name()+" (Update URI: "+uri+")")
			} else if v := headerValue(pluginVersionRe.FindSubmatch(b)); v != "" {
				versions[d.Name()] = v
			}
			break
		}
	}
	return versions, elsewhere
}

// pluginOf returns the slug of the plugin directory p is in, and its path
// within it, or "" for a path outside one.
func pluginOf(p string) (string, string) {
	rest, ok := strings.CutPrefix(p, pluginsDir)
	if !ok {
		return "", ""
	}
	slug, inner, ok := strings.Cut(rest, "/")
	if !ok || inner == "" {
		return "", ""
	}
	return slug, inner
}

// pluginRule returns the plugin rule a PHP file in a checked plugin
// breaks, if any: RulePluginModified for a file of the release that
// differs from its checksum, RulePluginUnknown for a PHP file the release
// doesn't have. As in uploads, empty files and data files that exit
// first aren't flagged until that changes. e must have been read with
// readTracked.
func pluginRule(inner string, sums *PluginChecksums, e Entry) string {
	if want, known := sums.SHA256[inner]; known {
		if e.SHA256 == "" || !slices.Contains(want, e.SHA256) {
			return RulePluginModified
		}
		return ""
	}
	if e.Unreadable || (e.Size > 0 && !isGuarded(e.Head)) {
		return RulePluginUnknown
	}
	return ""
}

// IsPHPPath reports whether a path names a PHP file.
func IsPHPPath(p string) bool {
	return isPHP(path.Base(p))
}
