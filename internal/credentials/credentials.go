// Package credentials stores the jman CLI's API token, one per API URL. It
// uses the OS keyring when one is available, and otherwise a file readable
// only by the user.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/zalando/go-keyring"
)

// keyringService is the service name tokens are stored under in the OS
// keyring; the API URL is the account.
const keyringService = "jman"

// fileName is the fallback file in the config directory.
const fileName = "credentials.json"

// ErrNotFound is returned when there is no token for an API URL.
var ErrNotFound = errors.New("not logged in")

type fileData struct {
	// Tokens maps API URLs to CLI tokens.
	Tokens map[string]string `json:"tokens"`
}

// Get returns the token stored for apiURL, or ErrNotFound.
func Get(apiURL string) (string, error) {
	if token, err := keyring.Get(keyringService, apiURL); err == nil {
		return token, nil
	}
	data, err := readFile()
	if err != nil {
		return "", err
	}
	if token := data.Tokens[apiURL]; token != "" {
		return token, nil
	}
	return "", ErrNotFound
}

// Set stores token for apiURL, in the OS keyring if possible and in the
// fallback file otherwise. It reports where the token was stored.
func Set(apiURL, token string) (string, error) {
	if err := keyring.Set(keyringService, apiURL, token); err == nil {
		// Don't leave an older copy behind in the file.
		_ = removeFromFile(apiURL)
		return "the system keyring", nil
	}
	data, err := readFile()
	if err != nil {
		return "", err
	}
	data.Tokens[apiURL] = token
	if err := writeFile(data); err != nil {
		return "", err
	}
	return filePath(), nil
}

// Delete removes the token for apiURL from both the keyring and the file.
func Delete(apiURL string) error {
	// Errors only mean there was no keyring entry or no keyring at all.
	_ = keyring.Delete(keyringService, apiURL)
	return removeFromFile(apiURL)
}

func filePath() string {
	return filepath.Join(config.RunData.ConfigDir, fileName)
}

func readFile() (*fileData, error) {
	data := &fileData{Tokens: map[string]string{}}
	b, err := os.ReadFile(filePath())
	if os.IsNotExist(err) {
		return data, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filePath(), err)
	}
	if err := json.Unmarshal(b, data); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filePath(), err)
	}
	if data.Tokens == nil {
		data.Tokens = map[string]string{}
	}
	return data, nil
}

func writeFile(data *fileData) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	path := filePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

func removeFromFile(apiURL string) error {
	data, err := readFile()
	if err != nil {
		return err
	}
	if _, ok := data.Tokens[apiURL]; !ok {
		return nil
	}
	delete(data.Tokens, apiURL)
	if len(data.Tokens) == 0 {
		if err := os.Remove(filePath()); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", filePath(), err)
		}
		return nil
	}
	return writeFile(data)
}
