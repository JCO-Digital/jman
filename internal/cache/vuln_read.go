package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

// The Read* functions below read the vulnerability cache that the refresh
// scheduler fills (RunFullRefresh through GetCachedVulnerabilities and
// GetCachedCoreVulnerabilities). They never fetch, whatever the file's
// age: request handlers and update jobs use them so that a request can't
// wait on wpvulnerability.net, and stale data is shown rather than
// dropped when a fetch fails.
//
// Decoded files are kept in memory and decoded again only when the file's
// modification time or size changes, so the many readers of one refresh's
// data share one decode per file. The returned responses are shared
// between callers and must not be modified.

type parsedVulnFile struct {
	modTime time.Time
	size    int64
	value   any
}

var (
	parsedVulnMu    sync.Mutex
	parsedVulnFiles = map[string]parsedVulnFile{}
)

// readParsedCache returns the decoded cache file filename, decoding it
// with decode only when the file changed since the last read.
func readParsedCache(filename string, decode func([]byte) (any, error)) (any, error) {
	path := getCacheFilePath(filename)
	if err := ensureWithinDir(path, config.RunData.CacheDir); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	parsedVulnMu.Lock()
	cached, ok := parsedVulnFiles[path]
	parsedVulnMu.Unlock()
	if ok && cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		return cached.value, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	value, err := decode(data)
	if err != nil {
		return nil, err
	}

	parsedVulnMu.Lock()
	parsedVulnFiles[path] = parsedVulnFile{modTime: info.ModTime(), size: info.Size(), value: value}
	parsedVulnMu.Unlock()
	return value, nil
}

// ReadCachedVulnerabilities returns the cached vulnerability data of a
// plugin, or an error if there is none or the feed reported an error for
// it. See the note above on sharing.
func ReadCachedVulnerabilities(slug string) (*models.VulnResponse, error) {
	v, err := readParsedCache("vulnerabilities/"+slug, func(data []byte) (any, error) {
		var resp models.VulnResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
		models.SanitizeVulnData(resp.Data)
		return &resp, nil
	})
	if err != nil {
		return nil, err
	}
	resp := v.(*models.VulnResponse)
	if resp.Error != 0 {
		return nil, fmt.Errorf("cached vulnerability data for %s holds a feed error", slug)
	}
	return resp, nil
}

// ReadCachedCoreVulnerabilities returns the cached vulnerability data of a
// WordPress core version, or an error if there is none or the feed
// reported an error for it. See the note above on sharing.
func ReadCachedCoreVulnerabilities(coreVersion string) (*models.CoreVulnResponse, error) {
	v, err := readParsedCache("vulnerabilities/core/"+coreVersion, func(data []byte) (any, error) {
		var resp models.CoreVulnResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
		normalizeCoreVulnResponse(&resp)
		return &resp, nil
	})
	if err != nil {
		return nil, err
	}
	resp := v.(*models.CoreVulnResponse)
	if resp.Error != 0 {
		return nil, fmt.Errorf("cached vulnerability data for WordPress %s holds a feed error", coreVersion)
	}
	return resp, nil
}

// IsSpecialPlugin reports whether slug is installed only as a must-use
// plugin or drop-in, which have no vulnerability data.
func IsSpecialPlugin(slug string) (bool, error) {
	return isSpecialPlugin(slug, nil)
}
