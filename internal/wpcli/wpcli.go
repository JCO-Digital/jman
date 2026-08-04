package wpcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/verb"
)

type CliOptions struct {
	SiteID         int
	SSH            string
	Path           string
	IncludePlugins bool
	IncludeThemes  bool
	Timeout        time.Duration
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

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 1 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wp", fullArgs...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := RunResult{
		Output: outBuf.String(),
		Error:  errBuf.String(),
	}

	verb.Printf(verb.Debug, "Command output:\n%s\n\nError output:\n%s", res.Output, res.Error)

	if opts.SiteID != 0 {
		if err != nil {
			RecordFailure(opts.SiteID)
		} else {
			RecordSuccess(opts.SiteID)
		}
	}

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

// GetWPCommandDump returns a JSON string representing the WP-CLI command structure.
func GetWPCommandDump(ssh, path string) (string, error) {
	res, err := RunWP(CliOptions{SSH: ssh, Path: path}, "cli", "cmd-dump", "--format=json")
	if err != nil {
		return "", fmt.Errorf("failed to get wp-cli command dump: %w (stderr: %s)", err, res.Error)
	}
	return res.Output, nil
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

// shellQuoteArg quotes s for safe inclusion in a POSIX shell command line.
// This is needed because the ssh client joins all trailing arguments with a
// single space and hands the result to the remote user's shell for parsing
// — passing args as separate exec.Command elements (as we do for local
// commands) does not protect against shell metacharacters on the remote end.
func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// RunSSH executes an arbitrary command via SSH on the target server. Each
// argument is shell-quoted and joined into a single remote command string so
// that ssh (which otherwise concatenates trailing args with spaces before
// the remote shell parses them) can't be tricked into splitting on
// attacker-influenced shell metacharacters.
func RunSSH(ssh string, args ...string) (RunResult, error) {
	if ssh == "" {
		return RunResult{}, fmt.Errorf("ssh connection string is required")
	}

	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = shellQuoteArg(arg)
	}
	remoteCommand := strings.Join(quotedArgs, " ")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh", ssh, remoteCommand)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := RunResult{
		Output: outBuf.String(),
		Error:  errBuf.String(),
	}

	verb.Printf(verb.Debug, "SSH Command output:\n%s\n\nError output:\n%s", res.Output, res.Error)

	return res, err
}

// UploadFile transfers a local file to a remote path via SCP.
func UploadFile(ssh, localPath, remotePath string) error {
	if ssh == "" {
		return fmt.Errorf("ssh connection string is required for upload")
	}

	// Format scp destination: user@host:path
	destination := fmt.Sprintf("%s:%s", ssh, remotePath)
	cmd := exec.Command("scp", localPath, destination)

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to upload file via scp: %w (stderr: %s)", err, errBuf.String())
	}

	return nil
}
