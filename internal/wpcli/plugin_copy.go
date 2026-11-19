package wpcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/knock"
)

// pluginCopyTimeout bounds copying a plugin's files off a site.
const pluginCopyTimeout = 5 * time.Minute

// PluginDir returns the absolute path of an installed plugin's directory on
// a site. It fails for single-file plugins, which have no directory of
// their own.
func PluginDir(siteID, ssh, sitePath, slug string) (string, error) {
	res, err := RunWP(CliOptions{SiteID: siteID, SSH: ssh, Path: sitePath}, "plugin", "path", slug, "--dir")
	if err != nil {
		return "", fmt.Errorf("failed to find the plugin's directory: %w", err)
	}
	dir := strings.TrimRight(strings.TrimSpace(res.Output), "/")
	if strings.ContainsAny(dir, "\r\n") || !path.IsAbs(dir) {
		return "", fmt.Errorf("WP-CLI returned an unexpected plugin path %q", dir)
	}
	if path.Base(dir) != slug {
		return "", fmt.Errorf("plugin %s has no directory of its own (it's a single-file plugin)", slug)
	}
	return dir, nil
}

// TarPluginDir streams a tar archive of the plugin directory dir (see
// PluginDir) to consume, over SSH, or locally if ssh is empty. Entries are
// named "<slug>/...". Version control directories and *.log files are left
// out. If consume returns an error, tar is stopped and that error returned.
func TarPluginDir(ssh, dir string, consume func(io.Reader) error) error {
	args := []string{"tar", "--exclude-vcs", "--exclude=*.log", "-C", path.Dir(dir), "-cf", "-", "--", path.Base(dir)}
	return streamCommand(ssh, args, pluginCopyTimeout, consume)
}

// streamCommand runs args over SSH (or locally if ssh is empty) and hands
// its stdout to consume.
func streamCommand(ssh string, args []string, timeout time.Duration, consume func(io.Reader) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if ssh == "" {
		cmd = exec.CommandContext(ctx, args[0], args[1:]...)
	} else {
		knock.KnockIfNeeded(ssh)
		if err := EnsureHostKey(ssh); err != nil {
			return err
		}
		target := parseSSHSpec(ssh)
		cmd = exec.CommandContext(ctx, "ssh", append(target.sshPortArgs("-p"), target.dest, remoteCommand(ssh, args))...)
	}
	cmd.WaitDelay = pipeWaitDelay
	var errBuf bytes.Buffer
	cmd.Stderr = &limitedBuffer{buf: &errBuf, max: 4096}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	consumeErr := consume(stdout)
	if consumeErr != nil {
		_ = cmd.Process.Kill()
	} else {
		// Drain what consume didn't read, so the command can exit.
		_, _ = io.Copy(io.Discard, stdout)
	}
	waitErr := cmd.Wait()

	switch {
	case consumeErr != nil:
		return consumeErr
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("%s timed out after %s", args[0], timeout)
	case waitErr != nil:
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return fmt.Errorf("%s failed: %s", args[0], firstLine(msg))
		}
		return fmt.Errorf("%s failed: %w", args[0], waitErr)
	}
	return nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// limitedBuffer keeps the first max bytes written to it.
type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
