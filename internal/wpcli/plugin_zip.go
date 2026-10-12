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
	r, err := zip.OpenReader(path)
	if err != nil {
		return ""
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
	if len(tops) == 1 && nested {
		for top := range tops {
			return sanitizeSlug(top)
		}
	}

	for _, f := range files {
		name := f.Name
		if strings.Contains(name, "/") || !strings.EqualFold(filepath.Ext(name), ".php") {
			continue
		}
		if hasPluginHeader(f) {
			return sanitizeSlug(strings.TrimSuffix(name, filepath.Ext(name)))
		}
	}
	return ""
}

func hasPluginHeader(f *zip.File) bool {
	rc, err := f.Open()
	if err != nil {
		return false
	}
	defer rc.Close()
	head, err := io.ReadAll(io.LimitReader(rc, pluginHeaderBytes))
	if err != nil {
		return false
	}
	return pluginHeaderRegex.Match(head)
}

func sanitizeSlug(s string) string {
	s = slugUnsafeRunRegex.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-.")
}
