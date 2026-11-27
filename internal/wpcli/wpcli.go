package wpcli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/knock"
	"github.com/JCO-Digital/jman/internal/models"
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
	// PremiumUpdates registers premium plugins' admin-only update checks
	// (PremiumUpdatesArg); for plugin lists and updates with plugins loaded.
	PremiumUpdates bool
	// EnglishMessages keeps the WordPress messages FailureCause reads in
	// English (EnglishMessagesArg); for updates and installs.
	EnglishMessages bool
	IncludeThemes   bool
	Timeout         time.Duration
	// Record, if set, gets the call and everything it printed appended,
	// for the update attempt log and failed jobs.
	Record *[]models.WPCLICall
}

type RunResult struct {
	Output string
	Error  string
}

// RunWP executes a wp-cli command. If ssh is provided, it runs via SSH.
// It uses variadic arguments to avoid shell injection and quoting issues.
// With opts.Record set, the call is recorded there.
func RunWP(opts CliOptions, args ...string) (RunResult, error) {
	start := time.Now()
	res, err := runWP(opts, args...)
	if opts.Record != nil {
		call := models.WPCLICall{
			Args:       slices.Clone(args),
			Stdout:     capOutput(res.Output),
			Stderr:     capOutput(res.Error),
			DurationMS: time.Since(start).Milliseconds(),
		}
		if err != nil {
			call.Error = err.Error()
		}
		*opts.Record = append(*opts.Record, call)
	}
	return res, err
}

// maxRecordedOutput caps each stream of a recorded call. A plugin that
// prints a notice for every file can fill megabytes; the error is nearly
// always at the end, so most of what's kept is the tail.
const maxRecordedOutput = 256 << 10

func capOutput(s string) string {
	if len(s) <= maxRecordedOutput {
		return s
	}
	head := maxRecordedOutput / 8
	tail := maxRecordedOutput - head
	return s[:head] + fmt.Sprintf("\n[… %d bytes left out …]\n", len(s)-head-tail) + s[len(s)-tail:]
}

func runWP(opts CliOptions, args ...string) (RunResult, error) {
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
	} else if opts.PremiumUpdates {
		fullArgs = append(fullArgs, PremiumUpdatesArg())
	}
	if opts.EnglishMessages {
		fullArgs = append(fullArgs, EnglishMessagesArg())
	}
	if !opts.IncludeThemes {
		fullArgs = append(fullArgs, "--skip-themes")
	}

	fullArgs = append(fullArgs, args...)

	// WP-CLI quotes each argument once for the remote shell; a gateway
	// that parses the command a second time needs another layer. --ssh
	// itself is for the local wp and stays as is.
	if opts.SSH != "" && reparsesCommand(opts.SSH) {
		for i := 1; i < len(fullArgs); i++ {
			fullArgs[i] = shellQuoteArg(fullArgs[i])
		}
	}

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

// shellQuoteArg quotes s for safe inclusion in a POSIX shell command line.
// This is needed because the ssh client joins all trailing arguments with a
// single space and hands the result to the remote user's shell for parsing
// — passing args as separate exec.Command elements (as we do for local
// commands) does not protect against shell metacharacters on the remote end.
func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// reparsesCommand reports whether an SSH host parses the remote command
// twice. WP Engine's SSH gateway (*.ssh.wpengine.net) splits the command
// into words, joins them back with spaces and runs the result with
// `bash -c`, so one layer of quoting is gone before bash sees it: an
// argument holding spaces, parentheses or quotes (such as the --exec PHP
// of PremiumUpdatesArg) becomes a bash syntax error. Commands sent to such
// a host need every argument quoted once more.
func reparsesCommand(spec string) bool {
	host := strings.ToLower(parseSSHSpec(spec).host)
	return host == "ssh.wpengine.net" || strings.HasSuffix(host, ".ssh.wpengine.net")
}

// remoteCommand shell-quotes args and joins them into the command string
// ssh hands to the remote shell, quoting twice for hosts that parse it
// twice (reparsesCommand).
func remoteCommand(spec string, args []string) string {
	twice := reparsesCommand(spec)
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuoteArg(arg)
		if twice {
			quoted[i] = shellQuoteArg(quoted[i])
		}
	}
	return strings.Join(quoted, " ")
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

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// ssh doesn't accept WP-CLI's "host:port" form, so pass the port as -p.
	target := parseSSHSpec(ssh)
	sshArgs := append(target.sshPortArgs("-p"), target.dest, remoteCommand(ssh, args))
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

// UploadFile copies a local file to a remote path over SSH, by piping it
// to `cat` on the host. scp isn't used: since OpenSSH 9.0 it copies over
// SFTP, which hosts such as WP Engine's SSH gateway don't offer ("subsystem
// request failed"), while a plain remote command works wherever wp-cli does.
func UploadFile(ssh, localPath, remotePath string) error {
	if ssh == "" {
		return fmt.Errorf("ssh connection string is required for upload")
	}

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", localPath, err)
	}
	defer f.Close()

	knock.KnockIfNeeded(ssh)
	if err := EnsureHostKey(ssh); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	target := parseSSHSpec(ssh)
	sshArgs := append(target.sshPortArgs("-p"), target.dest, uploadCommand(ssh, remotePath))
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	cmd.Stdin = f

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to upload file over ssh: %w (stderr: %s)", err, errBuf.String())
	}

	return nil
}

// uploadCommand is the remote command that writes stdin to remotePath. The
// redirection has to reach the remote shell unquoted, so only the path goes
// through remoteCommand's quoting.
func uploadCommand(ssh, remotePath string) string {
	return "cat > " + remoteCommand(ssh, []string{remotePath})
}

// StagingDir returns a new directory name on a site's host for files a job
// copies there before wp-cli uses them, such as a plugin ZIP. It's /tmp,
// except on hosts whose SSH connections each get a fresh /tmp (WP Engine's
// gateway, see reparsesCommand), where a file copied in one connection is
// gone in the next; there it's WordPress's own wp-content/upgrade, which is
// in the web root, so the name has a random part nobody can guess.
func StagingDir(site models.CliSite, name string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	name += "-" + hex.EncodeToString(b[:])
	if reparsesCommand(site.SSH) && path.IsAbs(site.Path) {
		return path.Join(site.Path, "wp-content", "upgrade", name)
	}
	return path.Join("/tmp", name)
}
