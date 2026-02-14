package lifecycle

import (
	"testing"
	"time"

	"carrier/daemon/internal/runtimecheck"
)

func TestTrimRestartHistory(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		history     []time.Time
		windowStart time.Time
		wantLen     int
	}{
		{
			name:        "empty history returns empty",
			history:     []time.Time{},
			windowStart: base,
			wantLen:     0,
		},
		{
			name:        "nil history returns nil",
			history:     nil,
			windowStart: base,
			wantLen:     0,
		},
		{
			name: "all entries before window are trimmed",
			history: []time.Time{
				base.Add(-3 * time.Hour),
				base.Add(-2 * time.Hour),
				base.Add(-1 * time.Hour),
			},
			windowStart: base,
			wantLen:     0,
		},
		{
			name: "no entries trimmed when all within window",
			history: []time.Time{
				base.Add(1 * time.Hour),
				base.Add(2 * time.Hour),
				base.Add(3 * time.Hour),
			},
			windowStart: base,
			wantLen:     3,
		},
		{
			name: "partial trim keeps entries at and after window start",
			history: []time.Time{
				base.Add(-2 * time.Hour),
				base.Add(-1 * time.Hour),
				base,
				base.Add(1 * time.Hour),
			},
			windowStart: base,
			wantLen:     2,
		},
		{
			name: "entry exactly at window start is kept",
			history: []time.Time{
				base,
			},
			windowStart: base,
			wantLen:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimRestartHistory(tt.history, tt.windowStart)
			if len(result) != tt.wantLen {
				t.Errorf("trimRestartHistory() returned %d entries, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestTrimRestartHistory_DoesNotMutateOriginal(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	original := []time.Time{
		base.Add(-2 * time.Hour),
		base.Add(-1 * time.Hour),
		base.Add(1 * time.Hour),
	}
	originalLen := len(original)

	_ = trimRestartHistory(original, base)

	if len(original) != originalLen {
		t.Errorf("original slice was mutated: got len %d, want %d", len(original), originalLen)
	}
}

func TestFirstFailedCode(t *testing.T) {
	tests := []struct {
		name     string
		result   runtimecheck.PreFlightResult
		wantCode string
	}{
		{
			name: "returns first failed check code",
			result: runtimecheck.PreFlightResult{
				Checks: []runtimecheck.CheckResult{
					{Passed: true, Code: "OK"},
					{Passed: false, Code: "E_MISSING_TOOL"},
					{Passed: false, Code: "E_PORT_CONFLICT"},
				},
			},
			wantCode: "E_MISSING_TOOL",
		},
		{
			name: "fallback to E_PREFLIGHT_FAILED when no explicit code",
			result: runtimecheck.PreFlightResult{
				Checks: []runtimecheck.CheckResult{
					{Passed: false, Code: ""},
				},
			},
			wantCode: "E_PREFLIGHT_FAILED",
		},
		{
			name: "fallback when all checks pass",
			result: runtimecheck.PreFlightResult{
				Checks: []runtimecheck.CheckResult{
					{Passed: true, Code: "OK"},
				},
			},
			wantCode: "E_PREFLIGHT_FAILED",
		},
		{
			name: "fallback on empty checks slice",
			result: runtimecheck.PreFlightResult{
				Checks: []runtimecheck.CheckResult{},
			},
			wantCode: "E_PREFLIGHT_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstFailedCode(tt.result)
			if got != tt.wantCode {
				t.Errorf("firstFailedCode() = %q, want %q", got, tt.wantCode)
			}
		})
	}
}
