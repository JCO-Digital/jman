package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// apiURLLine matches a top-level apiUrl assignment in config.toml.
var apiURLLine = regexp.MustCompile(`^\s*apiUrl\s*=`)

// SaveAPIURL writes apiUrl to config.toml and sets Cfg.APIURL. It edits the
// file in place rather than re-serializing it, so the rest of the file,
// comments and layout included, is kept. The file is created with mode
// 0600 if missing.
func SaveAPIURL(url string) error {
	path := filepath.Join(RunData.ConfigDir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	line := "apiUrl = " + strconv.Quote(url)
	if err := os.WriteFile(path, []byte(setTopLevelLine(string(data), apiURLLine, line)), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	Cfg.APIURL = url
	return nil
}

// setTopLevelLine replaces the top-level line of content matching re with
// line, or adds line at the end of the top-level section, before the first
// table header, where a key belongs in TOML.
func setTopLevelLine(content string, re *regexp.Regexp, line string) string {
	lines := strings.Split(content, "\n")
	insertAt := len(lines)
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "[") {
			insertAt = i
			break
		}
		if re.MatchString(l) {
			lines[i] = line
			return strings.Join(lines, "\n")
		}
	}

	// Insert after the last non-blank top-level line, keeping a blank line
	// before any table that follows.
	for insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}
	out := append([]string{}, lines[:insertAt]...)
	out = append(out, line)
	rest := lines[insertAt:]
	if len(rest) > 0 && strings.TrimSpace(rest[0]) != "" {
		out = append(out, "")
	}
	out = append(out, rest...)
	result := strings.Join(out, "\n")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result
}
