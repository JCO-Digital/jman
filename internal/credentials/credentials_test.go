package credentials

import (
	"errors"
	"os"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/zalando/go-keyring"
)

func useTempConfigDir(t *testing.T) {
	t.Helper()
	old := config.RunData
	config.RunData.ConfigDir = t.TempDir()
	t.Cleanup(func() { config.RunData = old })
}

func TestKeyring(t *testing.T) {
	useTempConfigDir(t)
	keyring.MockInit()

	where, err := Set("https://a.example.com", "jman_cli_a")
	if err != nil || where != "the system keyring" {
		t.Fatalf("Set = %q, %v", where, err)
	}
	if _, err := os.Stat(filePath()); !os.IsNotExist(err) {
		t.Error("token was also written to the file")
	}
	if got, err := Get("https://a.example.com"); err != nil || got != "jman_cli_a" {
		t.Errorf("Get = %q, %v", got, err)
	}
	if _, err := Get("https://b.example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get for another URL: %v", err)
	}
	if err := Delete("https://a.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get("https://a.example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: %v", err)
	}
}

func TestFileFallback(t *testing.T) {
	useTempConfigDir(t)
	keyring.MockInitWithError(errors.New("no keyring"))
	t.Cleanup(keyring.MockInit)

	where, err := Set("https://a.example.com", "jman_cli_a")
	if err != nil || where != filePath() {
		t.Fatalf("Set = %q, %v", where, err)
	}
	if _, err := Set("https://b.example.com", "jman_cli_b"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file mode: %v %v", info, err)
	}
	if got, _ := Get("https://b.example.com"); got != "jman_cli_b" {
		t.Errorf("Get = %q", got)
	}

	_ = Delete("https://a.example.com")
	if _, err := Get("https://a.example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: %v", err)
	}
	_ = Delete("https://b.example.com")
	if _, err := os.Stat(filePath()); !os.IsNotExist(err) {
		t.Error("expected the empty file to be removed")
	}
}
