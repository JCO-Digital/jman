package wpcli

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "upload.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

const pluginMain = "<?php\n/**\n * Plugin Name: My Plugin\n * Version: 1.0\n */\n"

func TestPluginZipSlug(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		filename string
		want     string
	}{
		{"top-level directory", map[string]string{"my-plugin/my-plugin.php": pluginMain, "my-plugin/readme.txt": ""}, "x.zip", "my-plugin"},
		{"directory with __MACOSX", map[string]string{"my-plugin/main.php": pluginMain, "__MACOSX/my-plugin/._main.php": ""}, "x.zip", "my-plugin"},
		{"root main file", map[string]string{"index.php": "<?php // Silence", "cool-thing.php": pluginMain, "inc/a.php": ""}, "cool-thing-2.1.0.zip", "cool-thing"},
		{"root without header", map[string]string{"a.php": "<?php", "readme.txt": ""}, "Cool Thing v2.1.zip", "cool-thing"},
		{"two top-level directories", map[string]string{"a/a.php": "", "b/b.php": ""}, "my_plugin.zip", "my_plugin"},
		{"version-only name", map[string]string{"a.php": ""}, "1.2.3.zip", "1.2.3"},
		{"nothing usable", map[string]string{"a.php": ""}, "!!!.zip", "plugin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PluginZipSlug(writeZip(t, tt.files), tt.filename); got != tt.want {
				t.Errorf("PluginZipSlug() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPluginZipSlugUnreadableZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "abc.zip")
	if err := os.WriteFile(path, []byte("PK\x03\x04"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := PluginZipSlug(path, "/some/dir/my-plugin-1.2.zip"); got != "my-plugin" {
		t.Errorf("PluginZipSlug() = %q, want my-plugin", got)
	}
}

func TestStagePluginZip(t *testing.T) {
	src := writeZip(t, map[string]string{"cool-thing.php": pluginMain})
	staged, cleanup, err := StagePluginZip(src, "upload.zip")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(staged) != "cool-thing.zip" {
		t.Errorf("staged as %s, want cool-thing.zip", staged)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(staged)); !os.IsNotExist(err) {
		t.Errorf("staging directory not removed (err %v)", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source removed: %v", err)
	}
}

func TestInspectPluginZip(t *testing.T) {
	main := "<?php\n/**\n * Plugin Name: Paid Thing Pro\n * Description: x\n * Version:     4.2.1\n */\n"
	tests := []struct {
		name  string
		files map[string]string
		want  PluginZipInfo
	}{
		{"nested", map[string]string{"paid-thing/readme.txt": "Version: 9", "paid-thing/paid-thing.php": main, "paid-thing/inc/other.php": "<?php\n// Plugin Name: Not me\n// Version: 0.1\n"},
			PluginZipInfo{Slug: "paid-thing", Name: "Paid Thing Pro", Version: "4.2.1"}},
		{"root", map[string]string{"paid-thing.php": main}, PluginZipInfo{Slug: "paid-thing", Name: "Paid Thing Pro", Version: "4.2.1"}},
		{"no header", map[string]string{"paid-thing/a.php": "<?php"}, PluginZipInfo{Slug: "paid-thing"}},
		{"single-line header", map[string]string{"p/p.php": "<?php\n/* Plugin Name: P */\n/* Version: 1.0 */"}, PluginZipInfo{Slug: "p", Name: "P", Version: "1.0"}},
	}
	for _, tt := range tests {
		if got := InspectPluginZip(writeZip(t, tt.files), "upload.zip"); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
