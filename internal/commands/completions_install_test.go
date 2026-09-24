package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

func TestInstallShellCompletions_UserDirs(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	configDir := filepath.Join(dir, "config")

	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	xdg.Reload()
	t.Cleanup(func() {
		xdg.Reload()
	})

	installed, err := InstallShellCompletions("")
	if err != nil {
		t.Fatalf("InstallShellCompletions failed: %v", err)
	}

	if len(installed) == 0 {
		t.Fatal("expected installed completions, got none")
	}

	// Verify bash completion
	bashPath := filepath.Join(dataDir, "bash-completion", "completions", "jman")
	bashData, err := os.ReadFile(bashPath)
	if err != nil {
		t.Errorf("failed to read bash completion at %s: %v", bashPath, err)
	} else if !strings.Contains(string(bashData), "jman") {
		t.Errorf("bash completion does not mention jman: %s", string(bashData[:min(100, len(bashData))]))
	}

	// Verify zsh completion
	zshPath := filepath.Join(dataDir, "zsh", "site-functions", "_jman")
	zshData, err := os.ReadFile(zshPath)
	if err != nil {
		t.Errorf("failed to read zsh completion at %s: %v", zshPath, err)
	} else if !strings.Contains(string(zshData), "#compdef jman") {
		t.Errorf("zsh completion missing #compdef jman: %s", string(zshData[:min(100, len(zshData))]))
	}

	// Verify fish completions
	fishConfigPath := filepath.Join(configDir, "fish", "completions", "jman.fish")
	fishData, err := os.ReadFile(fishConfigPath)
	if err != nil {
		t.Errorf("failed to read fish completion at %s: %v", fishConfigPath, err)
	} else if !strings.Contains(string(fishData), "jman") {
		t.Errorf("fish completion does not mention jman: %s", string(fishData[:min(100, len(fishData))]))
	}

	fishVendorPath := filepath.Join(dataDir, "fish", "vendor_completions.d", "jman.fish")
	if _, err := os.Stat(fishVendorPath); err != nil {
		t.Errorf("fish vendor completion does not exist at %s: %v", fishVendorPath, err)
	}
}

func TestInstallShellCompletions_Zfunc(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	configDir := filepath.Join(dir, "config")
	homeDir := filepath.Join(dir, "home")
	zfuncDir := filepath.Join(homeDir, ".zfunc")
	if err := os.MkdirAll(zfuncDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("HOME", homeDir)
	xdg.Reload()
	t.Cleanup(func() {
		xdg.Reload()
	})

	installed, err := InstallShellCompletions("")
	if err != nil {
		t.Fatalf("InstallShellCompletions failed: %v", err)
	}

	zfuncFile := filepath.Join(zfuncDir, "_jman")
	found := false
	for _, p := range installed {
		if p == zfuncFile {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %s in installed list: %v", zfuncFile, installed)
	}
	if _, err := os.Stat(zfuncFile); err != nil {
		t.Errorf("file %s was not written: %v", zfuncFile, err)
	}
}

func TestGenerateCompletions(t *testing.T) {
	bash, zsh, fish, err := generateCompletions("")
	if err != nil {
		t.Fatalf("generateCompletions failed: %v", err)
	}

	if !bytes.Contains(bash, []byte("jman")) {
		t.Errorf("bash completion content unexpected")
	}
	if !bytes.Contains(zsh, []byte("#compdef jman")) {
		t.Errorf("zsh completion content unexpected")
	}
	if !bytes.Contains(fish, []byte("jman")) {
		t.Errorf("fish completion content unexpected")
	}
}
