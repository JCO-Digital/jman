package api

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

// TestNewAPISiteSSH checks that /api/sites reports the SSH target jman
// actually connects to, not the server's IP address or display name.
func TestNewAPISiteSSH(t *testing.T) {
	serverID := "srv"
	cases := []struct {
		name string
		site models.ManagedSite
		want string
	}{
		{
			name: "spinupwp site uses the server hostname",
			site: models.ManagedSite{Provider: "spinupwp", ProviderSiteID: "1", ServerID: &serverID, ServerName: "web1.example.net", SSHHost: "web1.example.net", SSHPort: 22, SSHUser: "bee"},
			want: "bee@web1.example.net",
		},
		{
			name: "manual site uses its own host, not the logical server name",
			site: models.ManagedSite{Provider: "wpengine", ServerID: &serverID, ServerName: "WP Engine", SSHHost: "acme.ssh.wpengine.net", SSHPort: 22, SSHUser: "acme"},
			want: "acme@acme.ssh.wpengine.net",
		},
		{
			name: "non-default port is included",
			site: models.ManagedSite{Provider: "manual", SSHHost: "h.example.org", SSHPort: 2222, SSHUser: "u"},
			want: "u@h.example.org:2222",
		},
		{
			name: "no SSH host",
			site: models.ManagedSite{Provider: "manual", SSHUser: "u"},
			want: "",
		},
	}
	for _, c := range cases {
		if got := newAPISite(c.site).SSH; got != c.want {
			t.Errorf("%s: ssh = %q, want %q", c.name, got, c.want)
		}
	}
}
