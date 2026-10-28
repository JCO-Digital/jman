// Package jcore builds checksums for J&Co Digital's own plugins (jcore-*),
// which aren't on wordpress.org, from their releases on GitHub.
package jcore

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/utils"
)

// Where the releases are; variables so tests can point them elsewhere.
var (
	ReleaseURL = "https://github.com/JCO-Digital"
	SourceURL  = "https://codeload.github.com/JCO-Digital"
)

// maxArchiveBytes caps a downloaded release zip or source tarball.
const maxArchiveBytes = 128 << 20

// maxFileBytes caps a file hashed from an archive.
const maxFileBytes = 32 << 20

var slugRe = regexp.MustCompile(`^jcore-[a-z0-9-]+$`)

// IsJcoreSlug reports whether a plugin slug is one of J&Co Digital's, which
// publish their releases at github.com/JCO-Digital/<slug>.
func IsJcoreSlug(slug string) bool {
	return slugRe.MatchString(slug)
}

// GetPluginChecksums returns the SHA-256 checksums of a JCORE plugin
// release's files, keyed by path within the plugin, or nil when there's no
// such release. A release reaches sites two ways: its zip on the GitHub
// release (what the update client installs), where php-scoper has rewritten
// the plugin's references to the bundled update library, and the Composer
// dist repo, which has the tag's unscoped sources and the same build
// output. A file the tag has gets both hashes.
func GetPluginChecksums(slug, version string) (map[string][]string, error) {
	if !IsJcoreSlug(slug) {
		return nil, nil
	}
	tag := "v" + version
	zipped, err := download(fmt.Sprintf("%s/%s/releases/download/%s/%s.zip", ReleaseURL, slug, url.PathEscape(tag), slug))
	if zipped == nil || err != nil {
		return nil, err
	}
	sums, err := zipChecksums(zipped, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to read the %s %s release: %w", slug, version, err)
	}
	if len(sums) == 0 {
		return nil, nil
	}
	// The tag's sources only add hashes; without them the zip still checks
	// sites that installed it.
	if source, err := download(fmt.Sprintf("%s/%s/tar.gz/refs/tags/%s", SourceURL, slug, url.PathEscape(tag))); err == nil && source != nil {
		if err := addTarChecksums(sums, source); err != nil {
			return nil, fmt.Errorf("failed to read the %s %s sources: %w", slug, version, err)
		}
	}
	return sums, nil
}

// download returns a URL's body, or nil if it isn't found.
func download(u string) ([]byte, error) {
	client := utils.NewHTTPClient(60 * time.Second)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	utils.SetStandardHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch %s: status %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", u, err)
	}
	if len(b) > maxArchiveBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", u, maxArchiveBytes)
	}
	return b, nil
}

// zipChecksums hashes a release zip's files. Older releases zip the
// plugin's files at the top, newer ones in a folder named after the slug.
func zipChecksums(b []byte, slug string) (map[string][]string, error) {
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	prefix := slug + "/"
	nested := len(r.File) > 0
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, prefix) {
			nested = false
			break
		}
	}
	sums := map[string][]string{}
	for _, f := range r.File {
		name := f.Name
		if nested {
			name = strings.TrimPrefix(name, prefix)
		}
		if f.FileInfo().IsDir() || name == "" || f.UncompressedSize64 > maxFileBytes || !safePath(name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		sum, err := hash(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		sums[name] = []string{sum}
	}
	return sums, nil
}

// addTarChecksums adds the hashes of a GitHub source tarball's files (under
// a "<repo>-<tag>/" folder) to sums, for paths the release has.
func addTarChecksums(sums map[string][]string, b []byte) error {
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		_, name, ok := strings.Cut(h.Name, "/")
		if !ok || h.Typeflag != tar.TypeReg || h.Size > maxFileBytes || !safePath(name) {
			continue
		}
		known, inRelease := sums[name]
		if !inRelease {
			continue
		}
		sum, err := hash(tr)
		if err != nil {
			return err
		}
		if !slices.Contains(known, sum) {
			sums[name] = append(known, sum)
		}
	}
}

func hash(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(r, maxFileBytes)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// safePath reports whether an archive path stays inside the plugin.
func safePath(p string) bool {
	return !strings.HasPrefix(p, "/") && !slices.Contains(strings.Split(p, "/"), "..")
}
