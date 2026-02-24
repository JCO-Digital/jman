package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
)

// cacheTTL is the maximum age of a cache file (12 hours)
const cacheTTL = 6 * time.Hour

func getCacheFilePath(filename string) string {
	return filepath.Join(config.RunData.CacheDir, filename+".json")
}

func getDataFilePath(filename string) string {
	return filepath.Join(config.RunData.DataDir, filename+".json")
}

// ReadJSONCache reads a JSON file from the cache directory and unmarshals it into dest.
// If the file does not exist or is older than 12 hours, it returns an error.
func ReadJSONCache(filename string, dest any) error {
	filePath := getCacheFilePath(filename)

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cache file %s does not exist", filename)
		}
		return err
	}

	if time.Since(info.ModTime()) > cacheTTL {
		return fmt.Errorf("cache file %s is expired", filename)
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
