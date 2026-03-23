package wpcli

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/JCO-Digital/jman/internal/verb"
)

type CliOptions struct {
	SSH            string
	Path           string
	IncludePlugins bool
	IncludeThemes  bool
}

type RunResult struct {
	Output string
	Error  string
}

// RunWP executes a wp-cli command. If ssh is provided, it runs via SSH.
// It uses variadic arguments to avoid shell injection and quoting issues.
func RunWP(opts CliOptions, args ...string) (RunResult, error) {
	if _, err := exec.LookPath("wp"); err != nil {
		return RunResult{}, fmt.Errorf("wp-cli executable not found in PATH")
	}

	var fullArgs []string
	if opts.SSH != "" {
		fullArgs = append(fullArgs, fmt.Sprintf("--ssh=%s", opts.SSH))
	}
	if opts.Path != "" {
		fullArgs = append(fullArgs, fmt.Sprintf("--path=%s", opts.Path))
	}

	if !opts.IncludePlugins {
		fullArgs = append(fullArgs, "--skip-plugins")
	}
	if !opts.IncludeThemes {
		fullArgs = append(fullArgs, "--skip-themes")
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
		return res, err
	}

	return res, nil
}

// AddUser creates a new user on the target WordPress site.
func AddUser(ssh, path, username, email, role string) (string, error) {
	res, err := RunWP(CliOptions{SSH: ssh, Path: path}, "user", "create", username, email, "--role="+role)
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
	res, err := RunWP(CliOptions{SSH: ssh, Path: path}, "user", "reset-password", username, "--porcelain")
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
	_, err := RunWP(CliOptions{SSH: ssh, Path: path}, "config", "set", "--raw", "DISALLOW_FILE_MODS", valStr)
	return err
}
