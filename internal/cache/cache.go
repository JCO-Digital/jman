package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
)

func getCacheFilePath(filename string) string {
	return filepath.Join(config.RunData.CacheDir, filename+".json")
}

func getDataFilePath(filename string) string {
	return filepath.Join(config.RunData.DataDir, filename+".json")
}

// ReadJSONCache reads a JSON file from the cache directory and unmarshals it into dest.
// If ttlHours > 0 and the file is older than ttlHours, it returns an error.
// If ttlHours is 0, the cache never expires.
func ReadJSONCache(filename string, dest any, ttlHours int) error {
	filePath := getCacheFilePath(filename)

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cache file %s does not exist", filename)
		}
		return err
	}

	if ttlHours > 0 {
		ttl := time.Duration(ttlHours) * time.Hour
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
	filePath := getDataFilePath(filename)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// WriteJSONData marshals data into JSON and writes it to the data directory.
func WriteJSONData(filename string, data any) error {
	filePath := getDataFilePath(filename)

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
