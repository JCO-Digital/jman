package utils

import (
	"net/http"
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
