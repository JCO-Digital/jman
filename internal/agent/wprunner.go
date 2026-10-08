package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// jman-agent runs as root, but never runs WordPress code as root: every
// wp-cli call runs as the Unix user that owns the site's directory, with
// that user's primary group, no supplementary groups and a minimal
// environment (in particular, not the agent's own, which holds its API
// token). A site whose owner can't be determined safely is skipped, never
// run with more privileges. This contains a compromised plugin to the
// account that already owns its files.

// safePath is the PATH wp-cli runs with, and where the wp binary is looked
// up: system directories only, never the agent's own PATH.
const safePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// wpCommandTimeout bounds one wp-cli call. The process group is killed
// when it runs out, so PHP children can't outlive it.
const wpCommandTimeout = 2 * time.Minute

// maxWPOutputBytes caps what a wp-cli call may print; anything beyond is
// discarded and the call fails.
const maxWPOutputBytes = 4 << 20

// wpNiceness is the scheduling priority wp-cli runs at, so collection
// doesn't compete with the sites' own traffic.
const wpNiceness = 10

// siteIdentity is the Unix account wp-cli runs as for one site.
type siteIdentity struct {
	UID      uint32
	GID      uint32
	Username string
	Home     string
}

// wpRunner runs wp-cli commands for sites, each as its site's owner.
type wpRunner struct {
	wpPath string
}

// newWPRunner finds a wp binary it's safe to run: in a system directory,
// owned by root and not writable by anyone else (otherwise a site user
// could replace the binary every other site's collection runs). It also
// requires the agent to run as root, which dropping privileges needs.
func newWPRunner() (*wpRunner, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("jman-agent isn't running as root, so it can't run wp-cli as each site's owner")
	}
	for _, dir := range filepath.SplitList(safePath) {
		candidate := filepath.Join(dir, "wp")
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %s: %w", candidate, err)
		}
		if err := checkRootOwned(resolved); err != nil {
			return nil, err
		}
		return &wpRunner{wpPath: resolved}, nil
	}
	return nil, fmt.Errorf("wp-cli not found in %s", safePath)
}

// checkRootOwned fails unless path is a regular file owned by root and not
// writable by group or others.
func checkRootOwned(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("can't read the owner of %s", path)
	}
	switch {
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file", path)
	case st.Uid != 0:
		return fmt.Errorf("refusing to run %s: it's owned by uid %d, not root", path, st.Uid)
	case info.Mode().Perm()&0o022 != 0:
		return fmt.Errorf("refusing to run %s: it's writable by group or others (%04o)", path, info.Mode().Perm())
	}
	return nil
}

// lookupUser and lookupUserID are seams for tests.
var (
	lookupUser   = user.Lookup
	lookupUserID = user.LookupId
)

// resolveSiteIdentity returns the account that owns sitePath. It refuses a
// path that is a symlink or owned by root, an owner without an account or
// whose primary group is root, and an owner that differs from the site
// user jman-api names (when that user exists locally), since any of those
// suggests the path isn't the site's own.
func resolveSiteIdentity(sitePath, manifestUser string) (siteIdentity, error) {
	info, err := os.Lstat(sitePath)
	if err != nil {
		return siteIdentity{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return siteIdentity{}, fmt.Errorf("refusing to run wp-cli: %s is a symlink", sitePath)
	}
	if !info.IsDir() {
		return siteIdentity{}, fmt.Errorf("refusing to run wp-cli: %s is not a directory", sitePath)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return siteIdentity{}, fmt.Errorf("can't read the owner of %s", sitePath)
	}
	return identityForUID(st.Uid, sitePath, manifestUser)
}

func identityForUID(uid uint32, sitePath, manifestUser string) (siteIdentity, error) {
	if uid == 0 {
		return siteIdentity{}, fmt.Errorf("refusing to run wp-cli: %s is owned by root", sitePath)
	}
	u, err := lookupUserID(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return siteIdentity{}, fmt.Errorf("refusing to run wp-cli: the owner of %s (uid %d) has no account: %w", sitePath, uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return siteIdentity{}, fmt.Errorf("invalid primary group %q of %s", u.Gid, u.Username)
	}
	if gid == 0 {
		return siteIdentity{}, fmt.Errorf("refusing to run wp-cli: the owner of %s (%s) has root as its primary group", sitePath, u.Username)
	}
	if manifestUser != "" && manifestUser != u.Username {
		if mu, err := lookupUser(manifestUser); err == nil && mu.Uid != u.Uid {
			return siteIdentity{}, fmt.Errorf("refusing to run wp-cli: %s is owned by %s, but the site's user is %s", sitePath, u.Username, manifestUser)
		}
	}
	home := u.HomeDir
	if home == "" {
		home = sitePath
	}
	return siteIdentity{UID: uid, GID: uint32(gid), Username: u.Username, Home: home}, nil
}

// errOutputTooLarge is returned when wp-cli prints more than
// maxWPOutputBytes.
var errOutputTooLarge = errors.New("wp-cli output exceeded the size limit")

// cappedBuffer collects up to max bytes and records whether more arrived.
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room < len(p) {
		c.truncated = true
		if room > 0 {
			c.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return c.buf.Write(p)
}

// run executes wp-cli for the site at sitePath as id, with other plugins
// and themes skipped (so a broken plugin can't break collection, and site
// code beyond WordPress core doesn't run), and returns its stdout.
func (r *wpRunner) run(ctx context.Context, id siteIdentity, sitePath string, args ...string) (string, error) {
	if id.UID == 0 || id.GID == 0 {
		// resolveSiteIdentity never returns these; this guards the
		// invariant against future callers.
		return "", fmt.Errorf("refusing to run wp-cli as root")
	}

	full := append([]string{"--path=" + sitePath, "--skip-plugins", "--skip-themes"}, args...)
	cmd := exec.Command(r.wpPath, full...)
	cmd.Dir = sitePath
	cmd.Env = []string{
		"PATH=" + safePath,
		"HOME=" + id.Home,
		"USER=" + id.Username,
		"LOGNAME=" + id.Username,
		"LANG=C.UTF-8",
		"WP_CLI_DISABLE_AUTO_CHECK_UPDATE=1",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: id.UID, Gid: id.GID, Groups: []uint32{}},
		// Its own process group, so a timeout kills PHP children too.
		Setpgid: true,
	}
	stdout := &cappedBuffer{max: maxWPOutputBytes}
	stderr := &cappedBuffer{max: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start wp-cli: %w", err)
	}
	_ = syscall.Setpriority(syscall.PRIO_PGRP, cmd.Process.Pid, wpNiceness)

	ctx, cancel := context.WithTimeout(ctx, wpCommandTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return "", fmt.Errorf("wp-cli %v timed out after %s", args, wpCommandTimeout)
	}

	if stdout.truncated {
		return "", errOutputTooLarge
	}
	if err != nil {
		msg := firstErrorLine(stderr.buf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("wp-cli %v failed: %s", args, msg)
	}
	return stdout.buf.String(), nil
}

// firstErrorLine returns wp-cli's "Error: …" or PHP fatal error line from
// stderr, or its last non-empty line.
func firstErrorLine(stderr string) string {
	var last string
	for line := range strings.SplitSeq(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "Error:") || strings.Contains(trimmed, "Fatal error") {
			return trimmed
		}
		last = trimmed
	}
	return last
}
