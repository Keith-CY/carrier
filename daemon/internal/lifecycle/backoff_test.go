package lifecycle

import (
	"testing"
	"time"
)

func TestBackoffPolicy_CalculateNextDelay(t *testing.T) {
	policy := BackoffPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		MaxAttempts:  5,
	}

	tests := []struct {
		name          string
		attempt       int
		expectedDelay time.Duration
		expectedMaxed bool
	}{
		{
			name:          "first retry uses initial delay",
			attempt:       0,
			expectedDelay: 1 * time.Second,
			expectedMaxed: false,
		},
		{
			name:          "second retry doubles delay",
			attempt:       1,
			expectedDelay: 2 * time.Second,
			expectedMaxed: false,
		},
		{
			name:          "third retry quadruples delay",
			attempt:       2,
			expectedDelay: 4 * time.Second,
			expectedMaxed: false,
		},
		{
			name:          "delay increases exponentially",
			attempt:       3,
			expectedDelay: 8 * time.Second,
			expectedMaxed: false,
		},
		{
			name:          "delay capped at max delay",
			attempt:       4,
			expectedDelay: 16 * time.Second,
			expectedMaxed: false,
		},
		{
			name:          "max attempts exceeded",
			attempt:       5,
			expectedDelay: 0,
			expectedMaxed: true,
		},
		{
			name:          "well beyond max attempts",
			attempt:       10,
			expectedDelay: 0,
			expectedMaxed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay, maxed := policy.CalculateNextDelay(tt.attempt)
			if maxed != tt.expectedMaxed {
				t.Errorf("CalculateNextDelay(%d) maxed = %v, want %v", tt.attempt, maxed, tt.expectedMaxed)
			}
			if !maxed && delay != tt.expectedDelay {
				t.Errorf("CalculateNextDelay(%d) delay = %v, want %v", tt.attempt, delay, tt.expectedDelay)
			}
		})
	}
}

func TestBackoffPolicy_CalculateNextDelay_MaxDelayCap(t *testing.T) {
	policy := BackoffPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		MaxAttempts:  10,
	}

	// Attempt 4: 1s * 2^4 = 16s, should be capped to 10s
	delay, maxed := policy.CalculateNextDelay(4)
	if maxed {
		t.Errorf("CalculateNextDelay(4) should not exceed max attempts")
	}
	if delay != 10*time.Second {
		t.Errorf("CalculateNextDelay(4) delay = %v, want %v (capped at MaxDelay)", delay, 10*time.Second)
	}

	// Attempt 10: should exceed max attempts
	_, maxed = policy.CalculateNextDelay(10)
	if !maxed {
		t.Errorf("CalculateNextDelay(10) should exceed max attempts")
	}
}

func TestBackoffPolicy_ShouldRetry(t *testing.T) {
	policy := BackoffPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		MaxAttempts:  5,
	}

	now := time.Now()

	tests := []struct {
		name          string
		state         BackoffState
		now           time.Time
		shouldRetry   bool
		errorContains string
	}{
		{
			name: "initial state allows retry",
			state: BackoffState{
				Attempt:       0,
				NextRetryTime: time.Time{},
				CrashLooping:  false,
			},
			now:         now,
			shouldRetry: true,
		},
		{
			name: "crash looping blocks retry",
			state: BackoffState{
				Attempt:      5,
				CrashLooping: true,
			},
			now:           now,
			shouldRetry:   false,
			errorContains: "crash-loop",
		},
		{
			name: "cooldown in progress blocks retry",
			state: BackoffState{
				Attempt:       2,
				NextRetryTime: now.Add(10 * time.Second),
				CrashLooping:  false,
			},
			now:           now,
			shouldRetry:   false,
			errorContains: "backoff cooldown",
		},
		{
			name: "cooldown expired allows retry",
			state: BackoffState{
				Attempt:       2,
				NextRetryTime: now.Add(-1 * time.Second),
				CrashLooping:  false,
			},
			now:         now,
			shouldRetry: true,
		},
		{
			name: "exact cooldown expiry allows retry",
			state: BackoffState{
				Attempt:       2,
				NextRetryTime: now,
				CrashLooping:  false,
			},
			now:         now,
			shouldRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRetry, msg := policy.ShouldRetry(tt.state, tt.now)
			if shouldRetry != tt.shouldRetry {
				t.Errorf("ShouldRetry() = %v, want %v", shouldRetry, tt.shouldRetry)
			}
			if !shouldRetry && tt.errorContains != "" {
				if msg == "" {
					t.Errorf("ShouldRetry() message is empty, want to contain %q", tt.errorContains)
				}
			}
		})
	}
}

func TestBackoffPolicy_RecordCrash(t *testing.T) {
	policy := BackoffPolicy{
		InitialDelay:     1 * time.Second,
		MaxDelay:         30 * time.Second,
		Multiplier:       2.0,
		MaxAttempts:      5,
		SuccessThreshold: 30 * time.Second,
	}

	now := time.Now()

	tests := []struct {
		name              string
		state             BackoffState
		now               time.Time
		expectedAttempt   int
		expectedCrashLoop bool
		expectedNextRetry time.Duration // relative to now
	}{
		{
			name: "first crash increments attempt",
			state: BackoffState{
				Attempt:       0,
				LastStartTime: now.Add(-5 * time.Second), // crashed after 5s
			},
			now:               now,
			expectedAttempt:   1,
			expectedCrashLoop: false,
			expectedNextRetry: 1 * time.Second, // initial delay
		},
		{
			name: "second crash increases delay exponentially",
			state: BackoffState{
				Attempt:       1,
				LastStartTime: now.Add(-5 * time.Second),
			},
			now:               now,
			expectedAttempt:   2,
			expectedCrashLoop: false,
			expectedNextRetry: 2 * time.Second, // 1s * 2^1
		},
		{
			name: "third crash continues exponential increase",
			state: BackoffState{
				Attempt:       2,
				LastStartTime: now.Add(-5 * time.Second),
			},
			now:               now,
			expectedAttempt:   3,
			expectedCrashLoop: false,
			expectedNextRetry: 4 * time.Second, // 1s * 2^2
		},
		{
			name: "crash after max attempts triggers crash loop",
			state: BackoffState{
				Attempt:       4,
				LastStartTime: now.Add(-5 * time.Second),
			},
			now:               now,
			expectedAttempt:   5,
			expectedCrashLoop: true,
		},
		{
			name: "crash after success threshold resets backoff",
			state: BackoffState{
				Attempt:       3,
				LastStartTime: now.Add(-60 * time.Second), // ran for 60s > 30s threshold
			},
			now:               now,
			expectedAttempt:   0,
			expectedCrashLoop: false,
			expectedNextRetry: 0, // Can retry immediately
		},
		{
			name: "crash just before success threshold does not reset",
			state: BackoffState{
				Attempt:       3,
				LastStartTime: now.Add(-29 * time.Second), // ran for 29s < 30s threshold
			},
			now:               now,
			expectedAttempt:   4,
			expectedCrashLoop: false,
			expectedNextRetry: 8 * time.Second, // 1s * 2^3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newState := policy.RecordCrash(tt.state, tt.now)

			if newState.Attempt != tt.expectedAttempt {
				t.Errorf("RecordCrash() Attempt = %v, want %v", newState.Attempt, tt.expectedAttempt)
			}
			if newState.CrashLooping != tt.expectedCrashLoop {
				t.Errorf("RecordCrash() CrashLooping = %v, want %v", newState.CrashLooping, tt.expectedCrashLoop)
			}
			if tt.expectedNextRetry > 0 {
				expectedTime := tt.now.Add(tt.expectedNextRetry)
				if !newState.NextRetryTime.Equal(expectedTime) {
					t.Errorf("RecordCrash() NextRetryTime = %v, want %v", newState.NextRetryTime, expectedTime)
				}
			}
		})
	}
}

func TestBackoffPolicy_RecordStart(t *testing.T) {
	policy := DefaultBackoffPolicy()
	now := time.Now()

	state := BackoffState{
		Attempt:       2,
		NextRetryTime: now.Add(5 * time.Second),
		CrashLooping:  false,
	}

	newState := policy.RecordStart(state, now)

	if newState.LastStartTime != now {
		t.Errorf("RecordStart() LastStartTime = %v, want %v", newState.LastStartTime, now)
	}
	// Other fields should be preserved
	if newState.Attempt != state.Attempt {
		t.Errorf("RecordStart() Attempt = %v, want %v", newState.Attempt, state.Attempt)
	}
	if newState.NextRetryTime != state.NextRetryTime {
		t.Errorf("RecordStart() NextRetryTime = %v, want %v", newState.NextRetryTime, state.NextRetryTime)
	}
}

func TestBackoffPolicy_Reset(t *testing.T) {
	policy := DefaultBackoffPolicy()

	resetState := policy.Reset()

	if resetState.Attempt != 0 {
		t.Errorf("Reset() Attempt = %v, want 0", resetState.Attempt)
	}
	if !resetState.NextRetryTime.IsZero() {
		t.Errorf("Reset() NextRetryTime = %v, want zero time", resetState.NextRetryTime)
	}
	if !resetState.LastStartTime.IsZero() {
		t.Errorf("Reset() LastStartTime = %v, want zero time", resetState.LastStartTime)
	}
	if resetState.CrashLooping {
		t.Errorf("Reset() CrashLooping = true, want false")
	}
}

func TestDefaultBackoffPolicy(t *testing.T) {
	policy := DefaultBackoffPolicy()

	if policy.InitialDelay != 1*time.Second {
		t.Errorf("DefaultBackoffPolicy InitialDelay = %v, want 1s", policy.InitialDelay)
	}
	if policy.MaxDelay != 5*time.Minute {
		t.Errorf("DefaultBackoffPolicy MaxDelay = %v, want 5m", policy.MaxDelay)
	}
	if policy.Multiplier != 2.0 {
		t.Errorf("DefaultBackoffPolicy Multiplier = %v, want 2.0", policy.Multiplier)
	}
	if policy.MaxAttempts != 10 {
		t.Errorf("DefaultBackoffPolicy MaxAttempts = %v, want 10", policy.MaxAttempts)
	}
	if policy.SuccessThreshold != 30*time.Second {
		t.Errorf("DefaultBackoffPolicy SuccessThreshold = %v, want 30s", policy.SuccessThreshold)
	}
}

func TestBackoffPolicy_Integration(t *testing.T) {
	// Simulate a full crash-restart cycle
	policy := BackoffPolicy{
		InitialDelay:     1 * time.Second,
		MaxDelay:         10 * time.Second,
		Multiplier:       2.0,
		MaxAttempts:      3,
		SuccessThreshold: 5 * time.Second,
	}

	now := time.Now()
	state := BackoffState{}

	// First start
	state = policy.RecordStart(state, now)
	if state.LastStartTime != now {
		t.Errorf("First start: LastStartTime = %v, want %v", state.LastStartTime, now)
	}

	// Crash after 2 seconds (< success threshold)
	now = now.Add(2 * time.Second)
	state = policy.RecordCrash(state, now)
	if state.Attempt != 1 {
		t.Errorf("After first crash: Attempt = %v, want 1", state.Attempt)
	}
	if state.NextRetryTime.Sub(now) != 1*time.Second {
		t.Errorf("After first crash: delay = %v, want 1s", state.NextRetryTime.Sub(now))
	}

	// Try to start during cooldown
	shouldRetry, msg := policy.ShouldRetry(state, now)
	if shouldRetry {
		t.Errorf("Should not retry during cooldown")
	}
	if msg == "" {
		t.Errorf("Expected cooldown message")
	}

	// Wait for cooldown and start again
	now = state.NextRetryTime
	state = policy.RecordStart(state, now)

	// Crash after 3 seconds (still < success threshold)
	now = now.Add(3 * time.Second)
	state = policy.RecordCrash(state, now)
	if state.Attempt != 2 {
		t.Errorf("After second crash: Attempt = %v, want 2", state.Attempt)
	}
	if state.NextRetryTime.Sub(now) != 2*time.Second {
		t.Errorf("After second crash: delay = %v, want 2s", state.NextRetryTime.Sub(now))
	}

	// Wait and start again
	now = state.NextRetryTime
	state = policy.RecordStart(state, now)

	// Crash after 10 seconds (> success threshold) - should reset
	now = now.Add(10 * time.Second)
	state = policy.RecordCrash(state, now)
	if state.Attempt != 0 {
		t.Errorf("After successful run then crash: Attempt = %v, want 0 (reset)", state.Attempt)
	}
	if state.CrashLooping {
		t.Errorf("After successful run then crash: should not be crash looping")
	}
}
