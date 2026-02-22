package lifecycle

import (
	"fmt"
	"math"
	"time"
)

// BackoffPolicy defines the parameters for exponential backoff retry strategy.
type BackoffPolicy struct {
	// InitialDelay is the delay before the first retry attempt.
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retry attempts.
	MaxDelay time.Duration
	// Multiplier is the factor by which delay increases after each attempt.
	Multiplier float64
	// MaxAttempts is the maximum number of retry attempts before giving up.
	MaxAttempts int
	// SuccessThreshold is the minimum uptime duration to consider a start successful
	// and reset the backoff state.
	SuccessThreshold time.Duration
}

// DefaultBackoffPolicy returns a sensible default backoff policy for agent restarts.
func DefaultBackoffPolicy() BackoffPolicy {
	return BackoffPolicy{
		InitialDelay:     1 * time.Second,
		MaxDelay:         5 * time.Minute,
		Multiplier:       2.0,
		MaxAttempts:      10,
		SuccessThreshold: 30 * time.Second,
	}
}

// BackoffState tracks the current retry state for a single agent.
type BackoffState struct {
	// Attempt is the current retry attempt number (0 = first attempt).
	Attempt int
	// NextRetryTime is the earliest time when the next retry should be attempted.
	NextRetryTime time.Time
	// LastStartTime is when the agent was last successfully started.
	LastStartTime time.Time
	// CrashLooping indicates the agent has exceeded max retry attempts.
	CrashLooping bool
}

// CalculateNextDelay calculates the next retry delay based on the current attempt.
// Returns the delay duration and whether the max attempts limit has been reached.
func (p BackoffPolicy) CalculateNextDelay(attempt int) (time.Duration, bool) {
	if attempt >= p.MaxAttempts {
		return 0, true // Max attempts exceeded
	}

	// Calculate exponential backoff: initialDelay * multiplier^attempt
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt))

	// Cap at max delay
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	return time.Duration(delay), false
}

// ShouldRetry determines if a retry should be attempted based on the current state.
// Returns true if retry is allowed, false if in crash loop or cooldown.
func (p BackoffPolicy) ShouldRetry(state BackoffState, now time.Time) (bool, string) {
	if state.CrashLooping {
		return false, fmt.Sprintf("crash-loop: exceeded max retry attempts (%d)", p.MaxAttempts)
	}

	if !state.NextRetryTime.IsZero() && now.Before(state.NextRetryTime) {
		remaining := state.NextRetryTime.Sub(now)
		return false, fmt.Sprintf("exponential backoff cooldown: %s remaining (attempt %d/%d)",
			remaining.Round(time.Second), state.Attempt, p.MaxAttempts)
	}

	return true, ""
}

// RecordCrash updates the backoff state after a crash, incrementing the attempt
// and calculating the next retry time.
func (p BackoffPolicy) RecordCrash(state BackoffState, now time.Time) BackoffState {
	// Check if the agent ran long enough to be considered successful
	if !state.LastStartTime.IsZero() {
		uptime := now.Sub(state.LastStartTime)
		if uptime >= p.SuccessThreshold {
			// Agent ran successfully for threshold duration - reset backoff
			return BackoffState{
				Attempt:       0,
				NextRetryTime: now, // Can retry immediately
				LastStartTime: time.Time{},
				CrashLooping:  false,
			}
		}
	}

	// Calculate delay based on current attempt (number of previous failures)
	delay, _ := p.CalculateNextDelay(state.Attempt)

	// Increment attempt for next retry
	nextAttempt := state.Attempt + 1

	// Check if next attempt would exceed max attempts
	_, maxExceeded := p.CalculateNextDelay(nextAttempt)

	return BackoffState{
		Attempt:       nextAttempt,
		NextRetryTime: now.Add(delay),
		LastStartTime: state.LastStartTime,
		CrashLooping:  maxExceeded,
	}
}

// RecordStart updates the backoff state to record a successful start.
func (p BackoffPolicy) RecordStart(state BackoffState, now time.Time) BackoffState {
	return BackoffState{
		Attempt:       state.Attempt,
		NextRetryTime: state.NextRetryTime,
		LastStartTime: now,
		CrashLooping:  state.CrashLooping,
	}
}

// Reset clears the backoff state, typically called after a successful run.
func (p BackoffPolicy) Reset() BackoffState {
	return BackoffState{
		Attempt:       0,
		NextRetryTime: time.Time{},
		LastStartTime: time.Time{},
		CrashLooping:  false,
	}
}
