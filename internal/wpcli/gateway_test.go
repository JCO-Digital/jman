package wpcli

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// shellWords parses cmd as bash would and returns its words.
func shellWords(t *testing.T, cmd string) []string {
	t.Helper()
	out, err := exec.Command("bash", "-c", `printf '%s\0' `+cmd).Output()
	if err != nil {
		t.Fatalf("bash parse of %q: %v", cmd, err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

func TestReparsesCommand(t *testing.T) {
	for spec, want := range map[string]bool{
		"opuscapita@opuscapita.ssh.wpengine.net":      true,
		"opuscapita@OpusCapita.SSH.WPEngine.net:2222": true,
		"user@ssh.wpengine.net":                       true,
		"user@example.com":                            false,
		"user@wpengine.net.example.com":               false,
		"myalias":                                     false,
	} {
		if got := reparsesCommand(spec); got != want {
			t.Errorf("reparsesCommand(%q) = %v, want %v", spec, got, want)
		}
	}
}

// The command must come out as the original arguments after the remote
// shell's parse, and on WP Engine after the gateway's extra one: split,
// joined with spaces and parsed again.
func TestRemoteCommandSurvivesGateway(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	args := []string{"wp", "--path=/sites/x", PremiumUpdatesArg(), "plugin", "update", "it's", "a b", "$HOME", "`id`;"}

	if got := shellWords(t, remoteCommand("user@example.com", args)); !slices.Equal(got, args) {
		t.Errorf("plain host: got %q, want %q", got, args)
	}

	cmd := remoteCommand("user@user.ssh.wpengine.net", args)
	gateway := strings.Join(shellWords(t, cmd), " ")
	if got := shellWords(t, gateway); !slices.Equal(got, args) {
		t.Errorf("gateway host: got %q, want %q", got, args)
	}
}
