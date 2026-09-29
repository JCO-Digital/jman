package wpcli

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// fakeSSH puts fake ssh and ssh-keygen executables first on PATH. ssh-keygen
// reports a host as known when its name is listed in knownHosts; ssh appends
// its arguments to a log and prints stderr.
func fakeSSH(t *testing.T, knownHosts []string, stderr string) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "ssh.log")

	known := strings.Join(knownHosts, "\n")
	scripts := map[string]string{
		"ssh-keygen": "#!/bin/sh\n" +
			"# args: -F <name> [-f file]\n" +
			"printf '%s\\n' '" + known + "' | grep -qxF -- \"$2\" && echo \"$2 ssh-ed25519 AAAA\" && exit 0\n" +
			"exit 1\n",
		"ssh": "#!/bin/sh\n" +
			"echo \"$*\" >> '" + logPath + "'\n" +
			"printf '%s' '" + stderr + "' >&2\n" +
			"exit 255\n",
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatalf("failed to write fake %s: %v", name, err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	resetHostKeyChecks()
	t.Cleanup(resetHostKeyChecks)
	return logPath
}

func sshCalls(t *testing.T, logPath string) []string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("failed to read ssh log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestEnsureHostKey_UnknownHostIsAcceptedOnce(t *testing.T) {
	logPath := fakeSSH(t, nil, "Warning: Permanently added '[mysite.ssh.wpengine.net]:2222' (ED25519) to the list of known hosts.\r\nPermission denied (publickey).\r\n")

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := EnsureHostKey("mysite@mysite.ssh.wpengine.net:2222"); err != nil {
				t.Errorf("EnsureHostKey error = %v", err)
			}
		}()
	}
	wg.Wait()

	calls := sshCalls(t, logPath)
	if len(calls) != 1 {
		t.Fatalf("expected exactly one ssh connection for concurrent callers, got %d: %v", len(calls), calls)
	}
	for _, want := range []string{"StrictHostKeyChecking=accept-new", "BatchMode=yes", "-p 2222", "mysite@mysite.ssh.wpengine.net exit"} {
		if !strings.Contains(calls[0], want) {
			t.Errorf("ssh call %q is missing %q", calls[0], want)
		}
	}
}

func TestEnsureHostKey_KnownHostSkipsConnection(t *testing.T) {
	logPath := fakeSSH(t, []string{"[mysite.ssh.wpengine.net]:2222"}, "")

	if err := EnsureHostKey("mysite@mysite.ssh.wpengine.net:2222"); err != nil {
		t.Fatalf("EnsureHostKey error = %v", err)
	}
	if calls := sshCalls(t, logPath); len(calls) != 0 {
		t.Fatalf("expected no ssh connection for a known host, got %v", calls)
	}
}

func TestEnsureHostKey_ChangedKeyIsRefused(t *testing.T) {
	fakeSSH(t, nil, "@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\r\n@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\r\nHost key verification failed.\r\n")

	err := EnsureHostKey("user@changed.example.com")
	if err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("expected a not-trusted error for a changed host key, got %v", err)
	}
}

func resetHostKeyChecks() {
	hostKeyChecks.Range(func(k, _ any) bool {
		hostKeyChecks.Delete(k)
		return true
	})
}
