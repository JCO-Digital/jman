package wpcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/verb"
)

// sshTarget is a parsed WP-CLI style SSH spec: [user@]host[:port].
type sshTarget struct {
	dest string // "user@host" or "host", as ssh/scp expect it
	host string
	port int // 0 when not given
}

// parseSSHSpec splits a WP-CLI --ssh value ("user@host", "user@host:2222",
// or an ~/.ssh/config alias) into the pieces ssh and scp take separately:
// unlike WP-CLI, they don't accept a ":port" suffix on the destination.
func parseSSHSpec(spec string) sshTarget {
	spec = strings.TrimSpace(spec)
	user, hostPort := "", spec
	if at := strings.LastIndex(spec, "@"); at >= 0 {
		user, hostPort = spec[:at], spec[at+1:]
	}

	host, port := hostPort, 0
	if colon := strings.LastIndex(hostPort, ":"); colon >= 0 {
		if p, err := strconv.Atoi(hostPort[colon+1:]); err == nil && p > 0 && p < 65536 {
			host, port = hostPort[:colon], p
		}
	}

	dest := host
	if user != "" {
		dest = user + "@" + host
	}
	return sshTarget{dest: dest, host: host, port: port}
}

// knownHostsName is how ssh records the host in known_hosts: "[host]:port"
// for non-default ports.
func (t sshTarget) knownHostsName() string {
	if t.port > 0 && t.port != 22 {
		return fmt.Sprintf("[%s]:%d", t.host, t.port)
	}
	return t.host
}

// sshPortArgs returns the port option for ssh ("-p") or scp ("-P").
func (t sshTarget) sshPortArgs(flag string) []string {
	if t.port > 0 && t.port != 22 {
		return []string{flag, strconv.Itoa(t.port)}
	}
	return nil
}

// knownHostsFile overrides the known_hosts file used by EnsureHostKey (tests).
var knownHostsFile string

// hostKeyChecks records one host key check per SSH target per process, so
// concurrent callers share a single attempt and later calls are free.
var hostKeyChecks sync.Map // key: sshTarget.knownHostsName() -> *hostKeyCheck

type hostKeyCheck struct {
	once sync.Once
	err  error
}

// EnsureHostKey makes sure the host key of an SSH target is recorded in
// known_hosts before a non-interactive connection is attempted. WP-CLI
// spawns its own ssh for --ssh and gives no way to pass options, so a host
// that has never been connected to fails with "Host key verification
// failed" — which is every new site on hosts like WP Engine, where each site
// has its own SSH hostname.
//
// For an unknown host it connects once with StrictHostKeyChecking=accept-new
// (trust on first use): the key is recorded during the key exchange, before
// authentication, so it is stored even if logging in fails. A host whose
// key has changed is still refused, and reported as an error.
func EnsureHostKey(spec string) error {
	t := parseSSHSpec(spec)
	if t.host == "" {
		return nil
	}

	v, _ := hostKeyChecks.LoadOrStore(t.knownHostsName(), &hostKeyCheck{})
	check := v.(*hostKeyCheck)
	check.once.Do(func() {
		check.err = ensureHostKey(t)
	})
	return check.err
}

func ensureHostKey(t sshTarget) error {
	if hostKeyKnown(t) {
		return nil
	}

	args := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
	}
	if knownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+knownHostsFile)
	}
	args = append(args, t.sshPortArgs("-p")...)
	args = append(args, t.dest, "exit")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	// The exit status is irrelevant here (authentication may well fail or
	// the account may not allow commands); only the host key matters.
	_ = cmd.Run()

	stderr := errBuf.String()
	if strings.Contains(stderr, "REMOTE HOST IDENTIFICATION HAS CHANGED") || strings.Contains(stderr, "Host key verification failed") {
		return fmt.Errorf("host key for %s is not trusted (it may have changed); verify it and update known_hosts manually", t.knownHostsName())
	}

	verb.Printf(verb.Verbose, "Recorded SSH host key for %s\n", t.knownHostsName())
	return nil
}

// hostKeyKnown reports whether known_hosts already has a key for the target.
// ssh-keygen -F understands hashed known_hosts entries.
func hostKeyKnown(t sshTarget) bool {
	args := []string{"-F", t.knownHostsName()}
	if knownHostsFile != "" {
		args = append(args, "-f", knownHostsFile)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh-keygen", args...).Output()
	return err == nil && len(bytes.TrimSpace(out)) > 0
}
