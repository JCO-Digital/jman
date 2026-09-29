package wpcli

import (
	"strings"
	"testing"
)

func TestParseSSHSpec(t *testing.T) {
	tests := []struct {
		spec       string
		dest, host string
		port       int
		known      string
	}{
		{"user@host.example.com", "user@host.example.com", "host.example.com", 0, "host.example.com"},
		{"mysite@mysite.ssh.wpengine.net:2222", "mysite@mysite.ssh.wpengine.net", "mysite.ssh.wpengine.net", 2222, "[mysite.ssh.wpengine.net]:2222"},
		{"user@host:22", "user@host", "host", 22, "host"},
		{"alias", "alias", "alias", 0, "alias"},
		{"alias:2200", "alias", "alias", 2200, "[alias]:2200"},
		{"user@host:notaport", "user@host:notaport", "host:notaport", 0, "host:notaport"},
	}
	for _, tt := range tests {
		got := parseSSHSpec(tt.spec)
		if got.dest != tt.dest || got.host != tt.host || got.port != tt.port {
			t.Errorf("parseSSHSpec(%q) = %+v, want dest=%q host=%q port=%d", tt.spec, got, tt.dest, tt.host, tt.port)
		}
		if k := got.knownHostsName(); k != tt.known {
			t.Errorf("parseSSHSpec(%q).knownHostsName() = %q, want %q", tt.spec, k, tt.known)
		}
	}

	if args := parseSSHSpec("u@h:2222").sshPortArgs("-P"); strings.Join(args, " ") != "-P 2222" {
		t.Errorf("sshPortArgs = %v, want [-P 2222]", args)
	}
	if args := parseSSHSpec("u@h").sshPortArgs("-p"); args != nil {
		t.Errorf("sshPortArgs for default port = %v, want none", args)
	}
}
