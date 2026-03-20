package wpcli

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/JCO-Digital/jman/internal/verb"
)

type RunResult struct {
	Output string
	Error  string
}

// RunWP executes a wp-cli command. If ssh is provided, it runs via SSH.
// It uses variadic arguments to avoid shell injection and quoting issues.
func RunWP(ssh, path string, skip bool, args ...string) (RunResult, error) {
	if _, err := exec.LookPath("wp"); err != nil {
		return RunResult{}, fmt.Errorf("wp-cli executable not found in PATH")
	}

	var fullArgs []string
	if ssh != "" {
		fullArgs = append(fullArgs, fmt.Sprintf("--ssh=%s", ssh))
	}
	if path != "" {
		fullArgs = append(fullArgs, fmt.Sprintf("--path=%s", path))
	}

	if skip {
		fullArgs = append(fullArgs, "--skip-plugins", "--skip-themes")
	}

	fullArgs = append(fullArgs, args...)

	cmd := exec.Command("wp", fullArgs...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := RunResult{
		Output: outBuf.String(),
		Error:  errBuf.String(),
	}

	verb.Printf(verb.Debug, "Command output:\n%s\n\nError output:\n%s", res.Output, res.Error)

	// If cmd.Run() returned an error (non-zero exit code), look for the first error line.
	if err != nil {
		if res.Error != "" {
			for line := range strings.SplitSeq(res.Error, "\n") {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				if strings.HasPrefix(trimmed, "Error:") || strings.HasPrefix(trimmed, "Fatal error:") {
					return res, fmt.Errorf("%s", trimmed)
				}
			}
		}
	}

	return res, nil
}

// AddUser creates a new user on the target WordPress site.
func AddUser(ssh, path, username, email, role string) (string, error) {
	res, err := RunWP(ssh, path, true, "user", "create", username, email, "--role="+role)
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
	res, err := RunWP(ssh, path, true, "user", "reset-password", username, "--porcelain")
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
	_, err := RunWP(ssh, path, true, "config", "set", "--raw", "DISALLOW_FILE_MODS", valStr)
	return err
}
