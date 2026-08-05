package cache

import (
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
)

func setupCacheTest(t *testing.T) {
	t.Helper()

	cacheDir, err := os.MkdirTemp("", "jman-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp cache dir: %v", err)
	}
	dataDir, err := os.MkdirTemp("", "jman-data-test-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}

	oldCacheDir := config.RunData.CacheDir
	oldDataDir := config.RunData.DataDir
	config.RunData.CacheDir = cacheDir
	config.RunData.DataDir = dataDir

	t.Cleanup(func() {
		os.RemoveAll(cacheDir)
		os.RemoveAll(dataDir)
		config.RunData.CacheDir = oldCacheDir
		config.RunData.DataDir = oldDataDir
	})
}

func TestWriteJSONCacheRejectsPathTraversal(t *testing.T) {
	setupCacheTest(t)

	err := WriteJSONCache("../../etc/evil", map[string]string{"pwned": "true"})
	if err == nil {
		t.Fatal("expected error for path-traversing filename, got nil")
	}

	// Confirm nothing was written outside the cache dir.
	if _, statErr := os.Stat("/etc/evil.json"); !os.IsNotExist(statErr) {
		t.Fatalf("traversal write should not have succeeded, stat err: %v", statErr)
	}
}

func TestReadJSONCacheRejectsPathTraversal(t *testing.T) {
	setupCacheTest(t)

	var dest map[string]string
	err := ReadJSONCache("../../etc/passwd", &dest, DefaultTTL)
	if err == nil {
		t.Fatal("expected error for path-traversing filename, got nil")
	}
}

func TestWriteJSONCacheAllowsNormalNestedFilename(t *testing.T) {
	setupCacheTest(t)

	if err := WriteJSONCache("vulnerabilities/some-plugin", map[string]string{"ok": "true"}); err != nil {
		t.Fatalf("expected normal nested filename to succeed, got: %v", err)
	}
}

func TestWriteJSONCacheAllowsCoreVersionNestedFilename(t *testing.T) {
	setupCacheTest(t)

	if err := WriteJSONCache("vulnerabilities/core/6.6.1", map[string]string{"ok": "true"}); err != nil {
		t.Fatalf("expected core version nested filename to succeed, got: %v", err)
	}
}
