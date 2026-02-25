package wpcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
)

type RunResult struct {
	Output string
	Error  string
}

// RunWP executes a wp-cli command via SSH.
func RunWP(ssh, path, command string, skip bool) (RunResult, error) {
	if _, err := exec.LookPath("wp"); err != nil {
		return RunResult{}, fmt.Errorf("wp-cli executable not found in PATH")
	}

	skipArgs := ""
	if skip {
		skipArgs = "--skip-plugins --skip-themes "
	}

	fullCmd := fmt.Sprintf("wp --ssh=%s --path=%s %s %s", ssh, path, skipArgs, command)
	cmd := exec.Command("sh", "-c", fullCmd)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	return RunResult{
		Output: outBuf.String(),
		Error:  errBuf.String(),
	}, err
}

// AddUser creates a new user on the target WordPress site.
func AddUser(ssh, path, username, email, role string) (string, error) {
	cmd := fmt.Sprintf("user create %s %s --role=%s", username, email, role)
	res, err := RunWP(ssh, path, cmd, true)
	if err != nil {
		return "", fmt.Errorf("failed to add user: %w (stderr: %s)", err, res.Error)
	}

	lines := strings.SplitSeq(res.Output, "\n")
	for line := range lines {
		if after, ok := strings.CutPrefix(line, "Password: "); ok {
			return strings.TrimSpace(after), nil
		}
	}
	return "", nil
}

// ResetUserPassword resets the password for a given user.
func ResetUserPassword(ssh, path, username string) (string, error) {
	cmd := fmt.Sprintf("user reset-password %s --porcelain", username)
	res, err := RunWP(ssh, path, cmd, true)
	if err != nil {
		return "", fmt.Errorf("failed to reset password: %w (stderr: %s)", err, res.Error)
	}
	return strings.TrimSpace(res.Output), nil
}

// AddPlugin installs and optionally activates a plugin.
func AddPlugin(ssh, path, plugin string, activate bool) (bool, error) {
	activateFlag := ""
	if activate {
		activateFlag = "--activate"
	}
	cmd := fmt.Sprintf("plugin install %s %s", plugin, activateFlag)
	res, err := RunWP(ssh, path, cmd, true)
	if err != nil {
		if strings.Contains(res.Error, "Plugin not found.") {
			return false, fmt.Errorf("plugin not found")
		} else if strings.Contains(res.Error, "Destination folder already exists.") {
			return false, fmt.Errorf("plugin already installed")
		}
		return false, fmt.Errorf("failed to install plugin: %w (stderr: %s)", err, res.Error)
	}
	return strings.Contains(res.Output, "Success:"), nil
}

// SetDisallowFileMods updates the DISALLOW_FILE_MODS constant in wp-config.php.
func SetDisallowFileMods(ssh, path string, value bool) error {
	valStr := "false"
	if value {
		valStr = "true"
	}
	cmd := fmt.Sprintf("config set --raw DISALLOW_FILE_MODS %s", valStr)
	_, err := RunWP(ssh, path, cmd, true)
	return err
}

// GetPlugins returns a list of installed plugins on the target site.
func GetPlugins(site models.CliSite) ([]models.WPPlugin, error) {
	res, err := RunWP(site.SSH, site.Path, "plugin list --format=json", true)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, fmt.Errorf("not a WordPress site")
		}
		return nil, fmt.Errorf("unknown error: %w", err)
	}

	output := res.Output
	idx := strings.Index(output, "[")
	if idx != -1 {
		output = output[idx:]
	} else {
		return nil, fmt.Errorf("no valid JSON array found in output")
	}

	type rawPlugin struct {
		Name          string `json:"name"`
		Status        string `json:"status"`
		Version       string `json:"version"`
		UpdateVersion string `json:"update_version"`
		AutoUpdate    string `json:"auto_update"`
	}

	var raw []rawPlugin
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse plugins JSON: %w", err)
	}

	var plugins []models.WPPlugin
	for _, rp := range raw {
		plugins = append(plugins, models.WPPlugin{
			SiteID:     site.ID,
			Name:       rp.Name,
			Status:     rp.Status,
			Version:    rp.Version,
			Update:     rp.UpdateVersion,
			AutoUpdate: rp.AutoUpdate == "on",
		})
	}

	return plugins, nil
}
