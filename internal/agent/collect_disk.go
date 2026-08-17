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
	"time"
)

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
