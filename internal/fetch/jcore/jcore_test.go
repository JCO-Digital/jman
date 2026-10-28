package jcore

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	w.Close()
	return b.Bytes()
}

func tarOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	w := tar.NewWriter(gz)
	for name, content := range files {
		w.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg})
		w.Write([]byte(content))
	}
	w.Close()
	gz.Close()
	return b.Bytes()
}

func TestGetPluginChecksums(t *testing.T) {
	scoped := "<?php use JcoreUpdate_1_7_1\\Jcore\\Update\\Config\\UpdateConfig;"
	unscoped := "<?php use Jcore\\Update\\Config\\UpdateConfig;"
	nested := zipOf(t, map[string]string{
		"jcore-turva/jcore-turva.php":                      scoped,
		"jcore-turva/build/blocks-manifest.php":            "<?php return [];",
		"jcore-turva/vendor/jcodigital/jcore-update/a.php": "<?php // scoped library",
		"jcore-turva/../../escape.php":                     "<?php",
	})
	flat := zipOf(t, map[string]string{"jcore-maailma.php": "<?php // maailma", "content.php": "<?php // content"})
	source := tarOf(t, map[string]string{
		"jcore-turva-1.15.0/jcore-turva.php": unscoped,
		"jcore-turva-1.15.0/composer.json":   "{}",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rel/jcore-turva/releases/download/v1.15.0/jcore-turva.zip":
			w.Write(nested)
		case "/rel/jcore-maailma/releases/download/v1.6.6/jcore-maailma.zip":
			w.Write(flat)
		case "/src/jcore-turva/tar.gz/refs/tags/v1.15.0":
			w.Write(source)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	oldRel, oldSrc := ReleaseURL, SourceURL
	ReleaseURL, SourceURL = srv.URL+"/rel", srv.URL+"/src"
	t.Cleanup(func() { ReleaseURL, SourceURL = oldRel, oldSrc })

	sums, err := GetPluginChecksums("jcore-turva", "1.15.0")
	if err != nil {
		t.Fatal(err)
	}
	// The main file matches both the zip's scoped copy and the tag's.
	if got := sums["jcore-turva.php"]; !slices.Equal(got, []string{sum(scoped), sum(unscoped)}) {
		t.Errorf("jcore-turva.php = %q", got)
	}
	if got := sums["build/blocks-manifest.php"]; !slices.Equal(got, []string{sum("<?php return [];")}) {
		t.Errorf("build/blocks-manifest.php = %q", got)
	}
	if _, ok := sums["vendor/jcodigital/jcore-update/a.php"]; !ok {
		t.Error("the bundled library is missing")
	}
	if _, ok := sums["composer.json"]; ok {
		t.Error("a file only in the tag was added")
	}
	for p := range sums {
		if !safePath(p) {
			t.Errorf("unsafe path kept: %s", p)
		}
	}

	// A zip without a folder; a release that isn't there; another vendor's slug.
	if sums, err := GetPluginChecksums("jcore-maailma", "1.6.6"); err != nil || len(sums) != 2 || sums["content.php"] == nil {
		t.Errorf("flat zip = %v, %v", sums, err)
	}
	if sums, err := GetPluginChecksums("jcore-turva", "0.0.1"); err != nil || sums != nil {
		t.Errorf("missing release = %v, %v", sums, err)
	}
	if sums, err := GetPluginChecksums("akismet", "5.3"); err != nil || sums != nil {
		t.Errorf("other slug = %v, %v", sums, err)
	}
}
