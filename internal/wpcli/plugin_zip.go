package wpcli

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	// WordPress reads plugin headers from the first 8 KB of a file.
	pluginHeaderRegex  = regexp.MustCompile(`(?mi)^[ \t/*#@]*Plugin Name:[ \t]*\S`)
	zipVersionRegex    = regexp.MustCompile(`[-_.]v?\d+(\.\d+)*$`)
	slugUnsafeRunRegex = regexp.MustCompile(`[^a-z0-9_.-]+`)
)

const pluginHeaderBytes = 8 << 10

// PluginZipSlug guesses the directory name a plugin installed from the ZIP
// at path should get. WordPress uses the ZIP's single top-level directory
// if it has one, and otherwise names the directory after the ZIP file, so
// a ZIP with files at its root should be installed from "<slug>.zip". The
// slug is the top-level directory, else the main plugin file's name (the
// PHP file with a Plugin Name header), else filename without any version
// suffix.
func PluginZipSlug(path, filename string) string {
	if slug := slugFromZip(path); slug != "" {
		return slug
	}
	name := sanitizeSlug(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if slug := zipVersionRegex.ReplaceAllString(name, ""); strings.IndexFunc(slug, unicode.IsLetter) >= 0 {
		return slug
	}
	if name != "" {
		return name
	}
	return "plugin"
}

// StagePluginZip copies the plugin ZIP at path to a temporary directory as
// "<slug>.zip" (see PluginZipSlug) for installing on a local site. cleanup
// removes the copy.
func StagePluginZip(path, filename string) (staged string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "jman-plugin-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	staged = filepath.Join(dir, PluginZipSlug(path, filename)+".zip")
	if err := copyFile(path, staged); err != nil {
		cleanup()
		return "", nil, err
	}
	return staged, cleanup, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func slugFromZip(path string) string {
	return inspectZip(path).Slug
}

// PluginZipInfo describes the plugin in a ZIP.
type PluginZipInfo struct {
	// Slug is the directory the plugin is installed as (see PluginZipSlug).
	Slug string `json:"slug"`
	// Name and Version are the main plugin file's headers; empty if the
	// ZIP has no file with a Plugin Name header where WordPress looks.
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InspectPluginZip reads the slug, name and version of the plugin in the
// ZIP at path, uploaded as filename.
func InspectPluginZip(path, filename string) PluginZipInfo {
	info := inspectZip(path)
	info.Slug = PluginZipSlug(path, filename)
	return info
}

// inspectZip finds the plugin's directory (the ZIP's single top-level
// directory, if it has one) and its main file, the PHP file directly in
// that directory (or at the ZIP's root) with a Plugin Name header, as
// WordPress does. Slug is the directory, else the main file's name.
func inspectZip(path string) PluginZipInfo {
	r, err := zip.OpenReader(path)
	if err != nil {
		return PluginZipInfo{}
	}
	defer r.Close()

	// Mirror WordPress: __MACOSX entries are skipped when unzipping.
	var files []*zip.File
	tops := map[string]bool{}
	nested := false
	for _, f := range r.File {
		name := strings.TrimPrefix(filepath.ToSlash(f.Name), "/")
		if name == "" || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		files = append(files, f)
		top, rest, found := strings.Cut(name, "/")
		tops[top] = true
		if found {
			nested = nested || rest != "" || f.FileInfo().IsDir()
		}
	}
	dir := ""
	if len(tops) == 1 && nested {
		for top := range tops {
			dir = top
		}
	}

	var info PluginZipInfo
	if dir != "" {
		info.Slug = sanitizeSlug(dir)
	}
	for _, f := range files {
		name := strings.TrimPrefix(filepath.ToSlash(f.Name), "/")
		if dir != "" {
			var ok bool
			if name, ok = strings.CutPrefix(name, dir+"/"); !ok {
				continue
			}
		}
		if strings.Contains(name, "/") || !strings.EqualFold(filepath.Ext(name), ".php") {
			continue
		}
		head := pluginHeader(f)
		if head == nil {
			continue
		}
		if info.Slug == "" {
			info.Slug = sanitizeSlug(strings.TrimSuffix(name, filepath.Ext(name)))
		}
		info.Name = headerField(head, "Plugin Name")
		info.Version = headerField(head, "Version")
		break
	}
	return info
}

// pluginHeader returns the start of f if it has a Plugin Name header.
func pluginHeader(f *zip.File) []byte {
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	head, err := io.ReadAll(io.LimitReader(rc, pluginHeaderBytes))
	if err != nil || !pluginHeaderRegex.Match(head) {
		return nil
	}
	return head
}

// headerField reads a plugin header field the way WordPress's
// get_file_data does.
func headerField(head []byte, field string) string {
	re := regexp.MustCompile(`(?mi)^[ \t/*#@]*` + regexp.QuoteMeta(field) + `:(.*)$`)
	m := re.FindSubmatch(head)
	if m == nil {
		return ""
	}
	v := strings.TrimSpace(string(m[1]))
	v = strings.TrimSpace(strings.TrimSuffix(v, "*/"))
	return strings.TrimSpace(strings.TrimSuffix(v, "?>"))
}

func sanitizeSlug(s string) string {
	s = slugUnsafeRunRegex.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-.")
}
