package wpcli

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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
