package wpcli

import (
	"fmt"
	"strconv"
	"strings"
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
