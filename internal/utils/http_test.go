package utils

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	t.Run("with positive timeout", func(t *testing.T) {
		timeout := 5 * time.Second
		client := NewHTTPClient(timeout)
		if client.Timeout != timeout {
			t.Errorf("Expected timeout %v, got %v", timeout, client.Timeout)
		}
	})

	t.Run("with zero timeout", func(t *testing.T) {
		client := NewHTTPClient(0)
		if client.Timeout != DefaultTimeout {
			t.Errorf("Expected default timeout %v, got %v", DefaultTimeout, client.Timeout)
		}
	})

	t.Run("with negative timeout", func(t *testing.T) {
		client := NewHTTPClient(-1 * time.Second)
		if client.Timeout != DefaultTimeout {
			t.Errorf("Expected default timeout %v, got %v", DefaultTimeout, client.Timeout)
		}
	})
}

func TestSetStandardHeaders(t *testing.T) {
	t.Run("sets default user agent", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		SetStandardHeaders(req)

		got := req.Header.Get("User-Agent")
		if got != UserAgent {
			t.Errorf("Expected User-Agent %q, got %q", UserAgent, got)
		}
	})

	t.Run("does not override existing user agent", func(t *testing.T) {
		customUA := "CustomAgent/1.0"
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		req.Header.Set("User-Agent", customUA)
		SetStandardHeaders(req)

		got := req.Header.Get("User-Agent")
		if got != customUA {
			t.Errorf("Expected User-Agent to remain %q, got %q", customUA, got)
		}
	})
}

// countingServer returns a test server and a func reporting how many
// connections it has accepted.
func countingServer(t *testing.T) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	conns := 0
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			mu.Lock()
			conns++
			mu.Unlock()
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, func() int {
		mu.Lock()
		defer mu.Unlock()
		return conns
	}
}

func TestNewFreshConnHTTPClientOpensConnectionPerRequest(t *testing.T) {
	for _, tc := range []struct {
		name      string
		client    *http.Client
		wantConns int
	}{
		{"fresh", NewFreshConnHTTPClient(time.Second), 3},
		// The default client reuses one connection, which is what kept
		// uptime checks on a moved site's old server.
		{"default", NewHTTPClient(time.Second), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, conns := countingServer(t)
			for range 3 {
				resp, err := tc.client.Get(srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			if got := conns(); got != tc.wantConns {
				t.Errorf("3 requests opened %d connections, want %d", got, tc.wantConns)
			}
		})
	}
}
