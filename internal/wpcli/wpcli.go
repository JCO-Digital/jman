package wpcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/knock"
	"github.com/JCO-Digital/jman/internal/verb"
)

// DefaultTimeout bounds a WP-CLI call that doesn't set its own timeout.
const DefaultTimeout = 1 * time.Minute

// WriteTimeout bounds WP-CLI calls that change a site (plugin and core
// installs, updates, removals). Killing one of those midway can leave a
// plugin half-installed or the site in maintenance mode, so they get far
// longer than reads.
const WriteTimeout = 10 * time.Minute

// ErrTimeout is wrapped by RunWP's error when the call was killed for
// exceeding its timeout.
var ErrTimeout = errors.New("wp-cli timed out")

// pipeWaitDelay is how long RunWP keeps waiting for output after a timeout
// has killed wp. wp runs ssh as a child that keeps the output pipes open,
// so without a bound a "timed out" call would only return once the remote
// command finished.
const pipeWaitDelay = 2 * time.Second

type CliOptions struct {
	SiteID         string // site UUID, for failure tracking
	SSH            string
	Path           string
	User           int
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
		knock.KnockIfNeeded(opts.SSH)
		if err := EnsureHostKey(opts.SSH); err != nil {
			return RunResult{}, err
		}
		fullArgs = append(fullArgs, fmt.Sprintf("--ssh=%s", opts.SSH))
	}
	if opts.Path != "" {
		fullArgs = append(fullArgs, fmt.Sprintf("--path=%s", opts.Path))
	}
	if opts.User != 0 {
		fullArgs = append(fullArgs, fmt.Sprintf("--user=%d", opts.User))
	}

	if !opts.IncludePlugins {
		fullArgs = append(fullArgs, "--skip-plugins")
	}
	if !opts.IncludeThemes {
		fullArgs = append(fullArgs, "--skip-themes")
	}

	fullArgs = append(fullArgs, args...)

	timeout := effectiveTimeout(opts)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wp", fullArgs...)
	// Only wp itself is killed on timeout, not its ssh child: a remote
	// update that is still running is left to finish rather than being cut
	// off mid-install. WaitDelay stops RunWP from waiting on that child.
	cmd.WaitDelay = pipeWaitDelay

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res := RunResult{
		Output: outBuf.String(),
		Error:  errBuf.String(),
	}

	verb.Printf(verb.Debug, "Command output:\n%s\n\nError output:\n%s", res.Output, res.Error)

	if opts.SiteID != "" {
		if err != nil {
			RecordFailure(opts.SiteID)
		} else {
			RecordSuccess(opts.SiteID)
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		return res, fmt.Errorf("%w after %s", ErrTimeout, timeout)
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

// effectiveTimeout is the call's own timeout (DefaultTimeout if unset),
// raised to the longest configured wpcliHostTimeouts entry matching its SSH
// host.
func effectiveTimeout(opts CliOptions) time.Duration {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	if opts.SSH == "" {
		return timeout
	}
	host := parseSSHSpec(opts.SSH).host
	for _, ht := range config.Cfg.WPCLIHostTimeouts {
		if ok, _ := path.Match(ht.Host, host); !ok {
			continue
		}
		if t := time.Duration(ht.Minutes) * time.Minute; t > timeout {
			timeout = t
		}
	}
	return timeout
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

	knock.KnockIfNeeded(ssh)
	if err := EnsureHostKey(ssh); err != nil {
		return RunResult{}, err
	}

	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = shellQuoteArg(arg)
	}
	remoteCommand := strings.Join(quotedArgs, " ")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// ssh doesn't accept WP-CLI's "host:port" form, so pass the port as -p.
	target := parseSSHSpec(ssh)
	sshArgs := append(target.sshPortArgs("-p"), target.dest, remoteCommand)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)

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

	knock.KnockIfNeeded(ssh)
	if err := EnsureHostKey(ssh); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Format scp destination: user@host:path, with the port passed as -P
	// (scp doesn't accept WP-CLI's "host:port" form).
	target := parseSSHSpec(ssh)
	destination := fmt.Sprintf("%s:%s", target.dest, remotePath)
	scpArgs := append(target.sshPortArgs("-P"), "--", localPath, destination)
	cmd := exec.CommandContext(ctx, "scp", scpArgs...)

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to upload file via scp: %w (stderr: %s)", err, errBuf.String())
	}

	return nil
}
