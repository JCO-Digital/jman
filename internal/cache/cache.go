package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
)

const (
	// DefaultTTL is the default cache expiry for normal operations (48 hours).
	DefaultTTL = 48 * time.Hour
	// FetchTTL is the default cache expiry for the fetch command (30 minutes).
	FetchTTL = 30 * time.Minute
)

func getCacheFilePath(filename string) string {
	return filepath.Join(config.RunData.CacheDir, filename+".json")
}

func GetDataFilePath(filename string) string {
	return filepath.Join(config.RunData.DataDir, filename+".json")
}

// ensureWithinDir returns an error if path (once cleaned/made absolute)
// does not resolve to a location inside baseDir. filenames built from
// external input (e.g. a plugin slug) could otherwise contain ".." segments
// and escape the intended cache/data directory.
func ensureWithinDir(path, baseDir string) error {
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory: %w", err)
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return fmt.Errorf("refusing to access path outside of %s", baseDir)
	}
	return nil
}

// ReadJSONCache reads a JSON file from the cache directory and unmarshals it into dest.
// If ttl > 0 and the file is older than ttl, it returns an error.
// If ttl is 0, the cache is considered expired (useful for force refreshing).
// If ttl is -1, the cache never expires.
func ReadJSONCache(filename string, dest any, ttl time.Duration) error {
	filePath := getCacheFilePath(filename)
	if err := ensureWithinDir(filePath, config.RunData.CacheDir); err != nil {
		return err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cache file %s does not exist", filename)
		}
		return err
	}

	if ttl == 0 {
		return fmt.Errorf("cache file %s is forced to expire", filename)
	}

	if ttl > 0 {
		if time.Since(info.ModTime()) > ttl {
			return fmt.Errorf("cache file %s is expired", filename)
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// WriteJSONCache marshals data into JSON and writes it to the cache directory.
func WriteJSONCache(filename string, data any) error {
	filePath := getCacheFilePath(filename)
	if err := ensureWithinDir(filePath, config.RunData.CacheDir); err != nil {
		return err
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, bytes, 0644)
}

// ReadJSONData reads a JSON file from the data directory.
// Unlike the cache directory, data files do not expire.
func ReadJSONData(filename string, dest any) error {
	filePath := GetDataFilePath(filename)
	if err := ensureWithinDir(filePath, config.RunData.DataDir); err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// WriteJSONData marshals data into JSON and writes it to the data directory.
func WriteJSONData(filename string, data any) error {
	filePath := GetDataFilePath(filename)
	if err := ensureWithinDir(filePath, config.RunData.DataDir); err != nil {
		return err
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, bytes, 0644)
}
