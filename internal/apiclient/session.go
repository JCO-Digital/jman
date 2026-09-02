package apiclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// sessionFile is the name of the cached-session file under the config
// directory, keyed by nothing else — jman assumes a single jman-api
// identity per local user, matching how users.toml itself is scoped.
const sessionFile = "api-session.json"

type session struct {
	BaseURL   string    `json:"base_url"`
	Username  string    `json:"username"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LoadSession reads a previously cached session from configDir, if one
// exists and its token hasn't expired and matches baseURL/username. Returns
// ("", zero-time, false) if there's nothing usable to load.
func LoadSession(configDir, baseURL, username string) (token string, expiresAt time.Time, ok bool) {
	data, err := os.ReadFile(filepath.Join(configDir, sessionFile))
	if err != nil {
		return "", time.Time{}, false
	}

	var s session
	if err := json.Unmarshal(data, &s); err != nil {
		return "", time.Time{}, false
	}

	if s.BaseURL != baseURL || s.Username != username {
		return "", time.Time{}, false
	}
	if time.Now().After(s.ExpiresAt) {
		return "", time.Time{}, false
	}

	return s.Token, s.ExpiresAt, true
}

// SaveSession writes the given session to configDir with 0600 permissions,
// so repeated `jman agent token ...` invocations within the token's
// lifetime don't need to re-prompt for a password.
func SaveSession(configDir, baseURL, username, token string, expiresAt time.Time) error {
	s := session{BaseURL: baseURL, Username: username, Token: token, ExpiresAt: expiresAt}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, sessionFile), data, 0600)
}
