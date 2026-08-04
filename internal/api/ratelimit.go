package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
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
	BehindProxy    bool
	trustedProxies []*net.IPNet
	mu             sync.Mutex
	entries        map[string]loginAttemptRecord
	stop           chan struct{}
}

// NewLoginRateLimiter creates a new LoginRateLimiter. If behindProxy is true,
// the ClientIP helper will trust X-Real-IP and X-Forwarded-For headers, but
// only for requests whose immediate peer address (RemoteAddr) falls within
// config.Cfg.TrustedProxies — otherwise the headers are ignored, since any
// client can set them and an untrusted peer could spoof its rate-limit key.
func NewLoginRateLimiter(behindProxy bool) *LoginRateLimiter {
	rl := &LoginRateLimiter{
		BehindProxy:    behindProxy,
		trustedProxies: parseTrustedProxies(config.Cfg.TrustedProxies),
		entries:        make(map[string]loginAttemptRecord),
		stop:           make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// parseTrustedProxies parses a list of IPs or CIDRs into IPNets, skipping
// any entries that fail to parse. Bare IPs are treated as single-host CIDRs.
func parseTrustedProxies(proxies []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, p := range proxies {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			if ip := net.ParseIP(p); ip != nil && ip.To4() != nil {
				p += "/32"
			} else {
				p += "/128"
			}
		}
		if _, ipNet, err := net.ParseCIDR(p); err == nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}

// isTrustedProxy reports whether ip falls within one of the configured
// trusted proxy networks.
func (rl *LoginRateLimiter) isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range rl.trustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP extracts the client's IP address from an HTTP request. When
// BehindProxy is true AND the immediate peer (r.RemoteAddr) is in the
// configured trusted-proxy list, it checks proxy headers in the following
// order:
//  1. X-Real-IP (standard nginx header set via proxy_set_header X-Real-IP $remote_addr)
//  2. X-Forwarded-For (takes the first/leftmost IP, which is the original client)
//  3. Falls back to r.RemoteAddr with the port stripped
//
// If the peer is not a trusted proxy, these headers are attacker-controlled
// and are ignored — otherwise any client could set an arbitrary
// X-Forwarded-For value to get a fresh rate-limit bucket on every request.
func (rl *LoginRateLimiter) ClientIP(r *http.Request) string {
	peerIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port (e.g. Unix socket); use as-is.
		peerIP = r.RemoteAddr
	}

	if rl.BehindProxy && rl.isTrustedProxy(net.ParseIP(peerIP)) {
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

	return peerIP
}

// Allow returns true if the given key (typically an IP address) has fewer than
// 5 failed attempts in the current 15-minute window. If the window has
// expired, the counter is reset and the request is allowed.
func (rl *LoginRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	record, ok := rl.entries[key]
	if !ok {
		return true
	}

	// If the lockout window has expired, reset and allow.
	if time.Since(record.FirstAttempt) > loginLockoutWindow {
		delete(rl.entries, key)
		return true
	}

	return record.Attempts < maxLoginAttempts
}

// RecordFailure increments the failure counter for the given key. If this is
// the first failure, FirstAttempt is set to the current time.
func (rl *LoginRateLimiter) RecordFailure(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	record, ok := rl.entries[key]
	if !ok {
		rl.entries[key] = loginAttemptRecord{
			Attempts:     1,
			FirstAttempt: time.Now(),
		}
		return
	}

	// If the window has expired, start a new tracking period.
	if time.Since(record.FirstAttempt) > loginLockoutWindow {
		rl.entries[key] = loginAttemptRecord{
			Attempts:     1,
			FirstAttempt: time.Now(),
		}
		return
	}

	record.Attempts++
	rl.entries[key] = record
}

// Reset deletes the entry for the given key. This should be called on
// successful login.
func (rl *LoginRateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.entries, key)
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
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, record := range rl.entries {
		if now.Sub(record.FirstAttempt) > loginLockoutWindow {
			delete(rl.entries, key)
		}
	}
}
