package wpcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
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

// The upload command writes stdin to the path on a plain host and through
// the gateway's extra parse, whatever the path holds.
func TestUploadCommandSurvivesGateway(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	for _, spec := range []string{"user@example.com", "user@user.ssh.wpengine.net"} {
		dest := filepath.Join(t.TempDir(), "it's a $HOME `id`.zip")
		cmd := uploadCommand(spec, dest)
		if reparsesCommand(spec) {
			// The gateway splits the words (the redirection is one) and
			// joins them again.
			cmd = strings.Join(shellWords(t, strings.Replace(cmd, ">", `\>`, 1)), " ")
		}
		run := exec.Command("bash", "-c", cmd)
		run.Stdin = strings.NewReader("PK zip")
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v: %s", spec, err, out)
		}
		if got, err := os.ReadFile(dest); err != nil || string(got) != "PK zip" {
			t.Errorf("%s: file = %q, %v", spec, got, err)
		}
	}
}

// WP Engine gives each SSH connection its own /tmp, so files are staged in
// the site, under a name with a random part.
func TestStagingDir(t *testing.T) {
	wpe := models.CliSite{SSH: "x@x.ssh.wpengine.net", Path: "/sites/x"}
	plain := models.CliSite{SSH: "x@example.com", Path: "files"}
	pattern := map[models.CliSite]*regexp.Regexp{
		wpe:   regexp.MustCompile(`^/sites/x/wp-content/upgrade/jman-plugin-5-[0-9a-f]{16}$`),
		plain: regexp.MustCompile(`^/tmp/jman-plugin-5-[0-9a-f]{16}$`),
	}
	for site, re := range pattern {
		a, b := StagingDir(site, "jman-plugin-5"), StagingDir(site, "jman-plugin-5")
		if !re.MatchString(a) || a == b {
			t.Errorf("%s: StagingDir = %q, %q", site.SSH, a, b)
		}
	}
}
