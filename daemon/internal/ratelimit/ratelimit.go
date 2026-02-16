// Package ratelimit provides an in-memory sliding-window rate limiter.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter tracks request attempts per key (typically IP address) using a
// sliding window. After maxAttempts failures within the window, subsequent
// requests are rejected until the window slides past.
type Limiter struct {
	mu          sync.Mutex
	attempts    map[string][]time.Time
	maxAttempts int
	window      time.Duration
	now         func() time.Time
}

// Option configures a Limiter.
type Option func(*Limiter)

// WithWindow sets the sliding window duration.
func WithWindow(d time.Duration) Option {
	return func(l *Limiter) { l.window = d }
}

// WithMax sets the maximum number of attempts within the window.
func WithMax(n int) Option {
	return func(l *Limiter) { l.maxAttempts = n }
}

// WithNow overrides the time source (for testing).
func WithNow(fn func() time.Time) Option {
	return func(l *Limiter) { l.now = fn }
}

// New creates a rate limiter. Defaults: 5 attempts per 1 minute window.
func New(opts ...Option) *Limiter {
	l := &Limiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: 5,
		window:      1 * time.Minute,
		now:         time.Now,
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Allow checks whether the given key is within the rate limit.
// Returns true if the request is allowed, false if it should be rejected.
// Each call to Allow that returns true is recorded as an attempt.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)

	// Prune old attempts.
	entries := l.attempts[key]
	start := 0
	for start < len(entries) && entries[start].Before(cutoff) {
		start++
	}
	entries = entries[start:]

	if len(entries) >= l.maxAttempts {
		l.attempts[key] = entries
		return false
	}

	l.attempts[key] = append(entries, now)
	return true
}

// Record records a failed attempt for the given key without checking the limit.
// Use this when you want to count failures separately from Allow.
func (l *Limiter) Record(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)

	entries := l.attempts[key]
	start := 0
	for start < len(entries) && entries[start].Before(cutoff) {
		start++
	}
	l.attempts[key] = append(entries[start:], now)
}

// Reset removes all tracked attempts for a key.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
