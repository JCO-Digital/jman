package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxLoginAttempts   = 5
	loginLockoutWindow = 15 * time.Minute
	cleanupInterval    = 5 * time.Minute
)

type loginAttemptRecord struct {
	Attempts     int
	FirstAttempt time.Time
}

// LoginRateLimiter provides in-memory per-IP rate limiting for the login endpoint.
type LoginRateLimiter struct {
	BehindProxy bool
	entries     sync.Map
	stop        chan struct{}
}

// NewLoginRateLimiter creates a new LoginRateLimiter. If behindProxy is true,
// the ClientIP helper will trust X-Real-IP and X-Forwarded-For headers set by
// a reverse proxy such as nginx.
func NewLoginRateLimiter(behindProxy bool) *LoginRateLimiter {
	rl := &LoginRateLimiter{
		BehindProxy: behindProxy,
		stop:        make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// ClientIP extracts the client's IP address from an HTTP request. When
// BehindProxy is true it checks proxy headers in the following order:
//  1. X-Real-IP (standard nginx header set via proxy_set_header X-Real-IP $remote_addr)
//  2. X-Forwarded-For (takes the first/leftmost IP, which is the original client)
//  3. Falls back to r.RemoteAddr with the port stripped
//
// When BehindProxy is false, only r.RemoteAddr is used.
func (rl *LoginRateLimiter) ClientIP(r *http.Request) string {
	if rl.BehindProxy {
		// Prefer X-Real-IP (set explicitly by nginx).
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return realIP
		}

		// Fall back to X-Forwarded-For; take the leftmost (original client) IP.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.SplitN(xff, ",", 2)
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
	}

	// Fall back to RemoteAddr, stripping the port.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port (e.g. Unix socket); use as-is.
		return r.RemoteAddr
	}
	return ip
}

// Allow returns true if the given key (typically an IP address) has fewer than
// 5 failed attempts in the current 15-minute window. If the window has
// expired, the counter is reset and the request is allowed.
func (rl *LoginRateLimiter) Allow(key string) bool {
	val, ok := rl.entries.Load(key)
	if !ok {
		return true
	}

	record := val.(loginAttemptRecord)

	// If the lockout window has expired, reset and allow.
	if time.Since(record.FirstAttempt) > loginLockoutWindow {
		rl.entries.Delete(key)
		return true
	}

	return record.Attempts < maxLoginAttempts
}

// RecordFailure increments the failure counter for the given key. If this is
// the first failure, FirstAttempt is set to the current time.
func (rl *LoginRateLimiter) RecordFailure(key string) {
	val, ok := rl.entries.Load(key)
	if !ok {
		rl.entries.Store(key, loginAttemptRecord{
			Attempts:     1,
			FirstAttempt: time.Now(),
		})
		return
	}

	record := val.(loginAttemptRecord)

	// If the window has expired, start a new tracking period.
	if time.Since(record.FirstAttempt) > loginLockoutWindow {
		rl.entries.Store(key, loginAttemptRecord{
			Attempts:     1,
			FirstAttempt: time.Now(),
		})
		return
	}

	record.Attempts++
	rl.entries.Store(key, record)
}

// Reset deletes the entry for the given key. This should be called on
// successful login.
func (rl *LoginRateLimiter) Reset(key string) {
	rl.entries.Delete(key)
}

// Stop terminates the background cleanup goroutine. Call this when the rate
// limiter is no longer needed (e.g. during graceful shutdown).
func (rl *LoginRateLimiter) Stop() {
	close(rl.stop)
}

// cleanupLoop runs periodically to evict expired entries from the map,
// preventing unbounded memory growth.
func (rl *LoginRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.evictExpired()
		case <-rl.stop:
			return
		}
	}
}

// evictExpired removes all entries whose lockout window has expired.
func (rl *LoginRateLimiter) evictExpired() {
	now := time.Now()
	rl.entries.Range(func(key, value any) bool {
		record := value.(loginAttemptRecord)
		if now.Sub(record.FirstAttempt) > loginLockoutWindow {
			rl.entries.Delete(key)
		}
		return true
	})
}
