package agent

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// diskUsageInterval is how often a site's disk usage is measured. Each
// measurement walks the whole site, uploads included; a report cycle
// runs every few minutes, and every 30 seconds while traffic logs catch
// up, which disk usage doesn't need.
const diskUsageInterval = time.Hour

// lastDiskUsage is when each site's disk usage was last measured, in
// memory: after a restart every site is measured on the first cycle.
var (
	lastDiskUsage   = map[string]time.Time{}
	lastDiskUsageMu sync.Mutex
)

// diskUsageDue reports whether a site's disk usage is due for measuring.
func diskUsageDue(siteID string, now time.Time) bool {
	lastDiskUsageMu.Lock()
	defer lastDiskUsageMu.Unlock()
	last, ok := lastDiskUsage[siteID]
	return !ok || now.Sub(last) >= diskUsageInterval
}

// markDiskUsage records that a site's disk usage was measured and
// reported at at.
func markDiskUsage(siteID string, at time.Time) {
	lastDiskUsageMu.Lock()
	lastDiskUsage[siteID] = at
	lastDiskUsageMu.Unlock()
}

// CollectDiskUsage returns the total size in bytes of the given directory,
// preferring `du -sb` (fast, accounts for actual block usage) and falling
// back to a manual directory walk if `du` isn't available on the server.
func CollectDiskUsage(path string) (int64, error) {
	if _, err := exec.LookPath("du"); err == nil {
		return duBytes(path)
	}
	return walkDirBytes(path)
}

func duBytes(path string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "du", "-sb", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("du failed: %w", err)
	}

	fields := strings.Fields(out.String())
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected du output: %q", out.String())
	}
	return strconv.ParseInt(fields[0], 10, 64)
}

func walkDirBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries (permission issues, races with the site's
			// own writes) rather than failing the whole measurement.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}
