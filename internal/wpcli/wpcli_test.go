package wpcli

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/knock"
)

type mockKnocker struct {
	knocked []knock.KnockPort
	hosts   []string
}

func (m *mockKnocker) KnockOne(host string, kp knock.KnockPort, timeout time.Duration) error {
	m.hosts = append(m.hosts, host)
	m.knocked = append(m.knocked, kp)
	return nil
}

func TestWpcliKnockIntegration(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	mock := &mockKnocker{}
	knock.ActiveKnocker = mock
	defer func() {
		knock.ActiveKnocker = knock.DefaultKnocker{}
		config.Cfg.KnockdPorts = ""
		config.Cfg.KnockdTimeout = 0
	}()

	config.Cfg.KnockdPorts = "7000,8000:udp,9000"
	config.Cfg.KnockdTimeout = 60

	// Trigger via KnockIfNeeded directly or via SSH strings
	knock.KnockIfNeeded("user@wp-server.example.com")

	if len(mock.knocked) != 3 {
		t.Fatalf("expected 3 knocks, got %d", len(mock.knocked))
	}
	if len(mock.hosts) != 3 || mock.hosts[0] != "wp-server.example.com" {
		t.Fatalf("expected host wp-server.example.com, got %v", mock.hosts)
	}

	// Repeated call should skip
	mock.knocked = nil
	mock.hosts = nil
	knock.KnockIfNeeded("user@wp-server.example.com")
	if len(mock.knocked) != 0 {
		t.Fatalf("expected 0 knocks on second call, got %d", len(mock.knocked))
	}
}
