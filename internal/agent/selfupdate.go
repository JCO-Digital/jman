package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/JCO-Digital/jman/internal/update"
	"github.com/JCO-Digital/jman/internal/verb"
)

// CheckAndSelfUpdate checks GitHub Releases for a newer jman-agent build,
// and if one is available, downloads it (verifying the same Ed25519
// signature scheme jman/jman-api/jman-monitor use), atomically replaces the
// running executable, and re-execs in place via syscall.Exec — hot-swapping
// the process image with no dependency on systemd or any other supervisor
// noticing the file changed. On success this function does not return.
func CheckAndSelfUpdate(currentVersion string) error {
	checkVersion := currentVersion
	if checkVersion == "dev" {
		checkVersion = "v0.0.0"
	}

	latest, releaseURL, sigURL, available, err := update.CheckForUpdate(checkVersion, "jman-agent")
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}
	if !available {
		return nil
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}
	selfPath, err = filepath.EvalSymlinks(selfPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	verb.LogPrintf(verb.Normal, "Updating jman-agent %s -> %s...", currentVersion, latest)
	if err := update.DownloadAndReplace(releaseURL, sigURL, selfPath, "jman-agent"); err != nil {
		return fmt.Errorf("self-update failed: %w", err)
	}

	verb.LogPrintf(verb.Normal, "Re-executing updated jman-agent binary...")
	if err := syscall.Exec(selfPath, os.Args, os.Environ()); err != nil {
		return fmt.Errorf("update installed but failed to re-exec (restart jman-agent manually): %w", err)
	}
	return nil // unreachable on success — syscall.Exec replaces this process image
}
