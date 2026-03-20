package api

import (
	"sync"
	"time"
)

const (
	maxLoginAttempts   = 5
	loginLockoutWindow = 15 * time.Minute
)

type loginAttemptRecord struct {
	Attempts     int
	FirstAttempt time.Time
}

// LoginRateLimiter provides in-memory per-username rate limiting for the login endpoint.
type LoginRateLimiter struct {
	entries sync.Map
}

// NewLoginRateLimiter creates a new LoginRateLimiter.
func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{}
}

// Allow returns true if the username has fewer than 5 failed attempts in the
// current 15-minute window. If the window has expired, the counter is reset
// and the request is allowed.
func (rl *LoginRateLimiter) Allow(username string) bool {
	val, ok := rl.entries.Load(username)
	if !ok {
		return true
	}

	record := val.(loginAttemptRecord)

	// If the lockout window has expired, reset and allow.
	if time.Since(record.FirstAttempt) > loginLockoutWindow {
		rl.entries.Delete(username)
		return true
	}

	return record.Attempts < maxLoginAttempts
}

// RecordFailure increments the failure counter for the given username. If this
// is the first failure, FirstAttempt is set to the current time.
func (rl *LoginRateLimiter) RecordFailure(username string) {
	val, ok := rl.entries.Load(username)
	if !ok {
		rl.entries.Store(username, loginAttemptRecord{
			Attempts:     1,
			FirstAttempt: time.Now(),
		})
		return
	}

	record := val.(loginAttemptRecord)

	// If the window has expired, start a new tracking period.
	if time.Since(record.FirstAttempt) > loginLockoutWindow {
		rl.entries.Store(username, loginAttemptRecord{
			Attempts:     1,
			FirstAttempt: time.Now(),
		})
		return
	}

	record.Attempts++
	rl.entries.Store(username, record)
}

// Reset deletes the entry for the given username. This should be called on
// successful login.
func (rl *LoginRateLimiter) Reset(username string) {
	rl.entries.Delete(username)
}
