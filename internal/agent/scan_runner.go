package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/JCO-Digital/jman/internal/agent/scan"
)

// A site's files are scanned by the agent's own binary, started again with
// --scan-site as the site's owner, like wp-cli (see wprunner.go). The scan
// only reads and hashes files, but it reads files an attacker may control,
// so it doesn't do that as root. The child prints what it found as JSON;
// comparing that with the root-owned baseline happens back in the agent.

// ScanSiteFlag is the flag that runs jman-agent as a scan child.
const ScanSiteFlag = "scan-site"

// scanTimeout bounds one site's scan.
const scanTimeout = 10 * time.Minute

// maxScanOutputBytes caps what a scan child may print.
const maxScanOutputBytes = 64 << 20

// runScan is a seam for tests.
var runScan = runScanChild

// maxScanInputBytes caps what the scan child reads from its stdin: the
// walk's options, chiefly the site's core and plugin checksums (a few MB
// for a site with many large plugins).
const maxScanInputBytes = 64 << 20

// runScanChild scans sitePath as id with a child process, passing it opts
// on its stdin.
func runScanChild(ctx context.Context, id siteIdentity, sitePath string, opts scan.Options) (*scan.Result, error) {
	if id.UID == 0 || id.GID == 0 {
		// resolveSiteIdentity never returns these; this guards the
		// invariant against future callers.
		return nil, fmt.Errorf("refusing to scan as root")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to find the agent binary: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return nil, fmt.Errorf("failed to resolve the agent binary: %w", err)
	}
	if err := checkRootOwned(exe); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()

	cmd := exec.Command(exe, "--"+ScanSiteFlag, sitePath)
	cmd.Dir = sitePath
	cmd.Env = []string{
		"PATH=" + safePath,
		"HOME=" + id.Home,
		"USER=" + id.Username,
		"LOGNAME=" + id.Username,
		"LANG=C.UTF-8",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: id.UID, Gid: id.GID, Groups: []uint32{}},
		Setpgid:    true,
	}
	stdout := &cappedBuffer{max: maxScanOutputBytes}
	stderr := &cappedBuffer{max: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	input, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the scan's options: %w", err)
	}
	cmd.Stdin = bytes.NewReader(input)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start the scan: %w", err)
	}
	_ = syscall.Setpriority(syscall.PRIO_PGRP, cmd.Process.Pid, wpNiceness)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return nil, fmt.Errorf("the scan timed out after %s", scanTimeout)
	}

	if stdout.truncated {
		return nil, fmt.Errorf("the scan's output exceeded the size limit")
	}
	if err != nil {
		msg := firstErrorLine(stderr.buf.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("the scan failed: %s", msg)
	}
	var res scan.Result
	if err := json.Unmarshal(stdout.buf.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("failed to parse the scan's output: %w", err)
	}
	return &res, nil
}

// RunScanChild is the --scan-site mode: it walks sitePath as the current
// user, with the options the agent writes to in, and prints the result as
// JSON to out. It refuses to run as root, so a scan can't end up running
// with the agent's privileges.
func RunScanChild(sitePath string, in io.Reader, out io.Writer) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("refusing to scan as root")
	}
	var opts scan.Options
	b, err := io.ReadAll(io.LimitReader(in, maxScanInputBytes))
	if err != nil {
		return fmt.Errorf("failed to read the scan's options: %w", err)
	}
	if len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, &opts); err != nil {
			return fmt.Errorf("failed to parse the scan's options: %w", err)
		}
	}
	res, err := scan.Walk(sitePath, uint32(os.Getuid()), opts)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(res)
}
