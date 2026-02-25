package gateway

import (
	"fmt"
	"sync"
	"time"
)

// GatewayRateLimiter implements a sliding-window rate limiter with per-session
// and global tracking.
type GatewayRateLimiter struct {
	mu             sync.Mutex
	sessionWindows map[string][]time.Time
	globalWindow   []time.Time
	perSession     int
	global         int
	window         time.Duration
	now            func() time.Time
}

// RateLimitResult is the result of a rate limit check.
type RateLimitResult struct {
	Allowed   bool
	ErrorCode string
	Message   string
}

// NewGatewayRateLimiter creates a new rate limiter.
func NewGatewayRateLimiter(perSession, global int, window time.Duration, now func() time.Time) *GatewayRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &GatewayRateLimiter{
		sessionWindows: make(map[string][]time.Time),
		perSession:     perSession,
		global:         global,
		window:         window,
		now:            now,
	}
}

// Check checks the rate limit for a session key and records the request if allowed.
func (r *GatewayRateLimiter) Check(sessionKey string) RateLimitResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	cutoff := now.Add(-r.window)

	// Prune expired entries from all sessions
	for k, times := range r.sessionWindows {
		filtered := filterAfter(times, cutoff)
		if len(filtered) == 0 {
			delete(r.sessionWindows, k)
		} else {
			r.sessionWindows[k] = filtered
		}
	}

	// Prune global
	r.globalWindow = filterAfter(r.globalWindow, cutoff)

	// Check global limit
	if len(r.globalWindow) >= r.global {
		return RateLimitResult{
			Allowed:   false,
			ErrorCode: "E_RATE_LIMITED",
			Message:   fmt.Sprintf("global rate limit exceeded (%d req/%s)", r.global, r.window),
		}
	}

	// Check per-session limit
	sessionTimes := r.sessionWindows[sessionKey]
	if len(sessionTimes) >= r.perSession {
		return RateLimitResult{
			Allowed:   false,
			ErrorCode: "E_RATE_LIMITED",
			Message:   fmt.Sprintf("per-session rate limit exceeded (%d req/%s)", r.perSession, r.window),
		}
	}

	// Record
	r.sessionWindows[sessionKey] = append(sessionTimes, now)
	r.globalWindow = append(r.globalWindow, now)

	return RateLimitResult{Allowed: true}
}

// Cleanup removes expired entries. Returns count of removed session entries.
func (r *GatewayRateLimiter) Cleanup() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := r.now().Add(-r.window)
	removed := 0
	for k, times := range r.sessionWindows {
		filtered := filterAfter(times, cutoff)
		if len(filtered) == 0 {
			delete(r.sessionWindows, k)
			removed++
		} else {
			r.sessionWindows[k] = filtered
		}
	}
	r.globalWindow = filterAfter(r.globalWindow, cutoff)
	return removed
}

// Reset clears all state.
func (r *GatewayRateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionWindows = make(map[string][]time.Time)
	r.globalWindow = nil
}

func filterAfter(times []time.Time, cutoff time.Time) []time.Time {
	out := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}
