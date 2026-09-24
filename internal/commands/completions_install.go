package commands

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

type completionTarget struct {
	path string
	data []byte
}

// InstallShellCompletions generates and installs shell completion scripts for
// bash, zsh, and fish. It attempts to run binaryPath to generate the completions
// (so any new flags/commands in the updated binary are reflected), falling
// back to in-memory generation via rootCmd if executing binaryPath fails.
// Returns the list of destination paths that were successfully written.
func InstallShellCompletions(binaryPath string) ([]string, error) {
	bashData, zshData, fishData, err := generateCompletions(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to generate completions: %w", err)
	}

	targets := getCompletionTargets(binaryPath, bashData, zshData, fishData)

	var installed []string
	var lastErr error

	for _, target := range targets {
		if err := os.MkdirAll(filepath.Dir(target.path), 0755); err != nil {
			lastErr = err
			continue
		}
		if err := os.WriteFile(target.path, target.data, 0644); err != nil {
			lastErr = err
			continue
		}
		installed = append(installed, target.path)
	}

	if len(installed) == 0 && lastErr != nil {
		return nil, fmt.Errorf("failed to write completion files: %w", lastErr)
	}

	return installed, nil
}

func generateCompletions(binaryPath string) (bash, zsh, fish []byte, err error) {
	bash = tryRunCompletion(binaryPath, "bash")
	if len(bash) == 0 {
		var buf bytes.Buffer
		if err := rootCmd.GenBashCompletion(&buf); err != nil {
			return nil, nil, nil, fmt.Errorf("generating bash completion: %w", err)
		}
		bash = buf.Bytes()
	}

	zsh = tryRunCompletion(binaryPath, "zsh")
	if len(zsh) == 0 {
		var buf bytes.Buffer
		if err := rootCmd.GenZshCompletion(&buf); err != nil {
			return nil, nil, nil, fmt.Errorf("generating zsh completion: %w", err)
		}
		zsh = buf.Bytes()
	}

	fish = tryRunCompletion(binaryPath, "fish")
	if len(fish) == 0 {
		var buf bytes.Buffer
		if err := rootCmd.GenFishCompletion(&buf, true); err != nil {
			return nil, nil, nil, fmt.Errorf("generating fish completion: %w", err)
		}
		fish = buf.Bytes()
	}

	return bash, zsh, fish, nil
}

func tryRunCompletion(binaryPath, shell string) []byte {
	if binaryPath == "" {
		return nil
	}
	cmd := exec.Command(binaryPath, "completion", shell)
	cmd.Env = append(os.Environ(), "JMAN_TOKENSPINUP=placeholder")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func getCompletionTargets(binaryPath string, bash, zsh, fish []byte) []completionTarget {
	var targets []completionTarget

	// Check if this is a system-wide installation (e.g. /usr/local/bin or /usr/bin)
	// and whether the system completion directories are writable.
	if strings.HasPrefix(binaryPath, "/usr/local/bin/") && isDirWritable("/usr/local/share") {
		targets = append(targets,
			completionTarget{path: "/usr/local/share/bash-completion/completions/jman", data: bash},
			completionTarget{path: "/usr/local/share/zsh/site-functions/_jman", data: zsh},
			completionTarget{path: "/usr/local/share/fish/vendor_completions.d/jman.fish", data: fish},
		)
		return targets
	}

	if strings.HasPrefix(binaryPath, "/usr/bin/") && isDirWritable("/usr/share") {
		targets = append(targets,
			completionTarget{path: "/usr/share/bash-completion/completions/jman", data: bash},
			completionTarget{path: "/usr/share/zsh/site-functions/_jman", data: zsh},
			completionTarget{path: "/usr/share/fish/vendor_completions.d/jman.fish", data: fish},
		)
		return targets
	}

	// User-level installation (default and most common for jman)
	// Bash
	targets = append(targets, completionTarget{
		path: filepath.Join(xdg.DataHome, "bash-completion", "completions", "jman"),
		data: bash,
	})

	// Zsh
	targets = append(targets, completionTarget{
		path: filepath.Join(xdg.DataHome, "zsh", "site-functions", "_jman"),
		data: zsh,
	})
	if home, err := os.UserHomeDir(); err == nil {
		zfuncDir := filepath.Join(home, ".zfunc")
		if info, err := os.Stat(zfuncDir); err == nil && info.IsDir() {
			targets = append(targets, completionTarget{
				path: filepath.Join(zfuncDir, "_jman"),
				data: zsh,
			})
		}
	}

	// Fish
	targets = append(targets,
		completionTarget{
			path: filepath.Join(xdg.ConfigHome, "fish", "completions", "jman.fish"),
			data: fish,
		},
		completionTarget{
			path: filepath.Join(xdg.DataHome, "fish", "vendor_completions.d", "jman.fish"),
			data: fish,
		},
	)

	return targets
}

func isDirWritable(dir string) bool {
	for d := dir; d != "" && d != "/" && d != "."; d = filepath.Dir(d) {
		info, err := os.Stat(d)
		if err == nil {
			if !info.IsDir() {
				return false
			}
			testFile := filepath.Join(d, fmt.Sprintf(".jman-write-test-%d", os.Getpid()))
			if err := os.WriteFile(testFile, []byte(""), 0600); err != nil {
				return false
			}
			_ = os.Remove(testFile)
			return true
		}
	}
	return false
}
