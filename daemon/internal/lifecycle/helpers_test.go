package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFindListeningSocketInodeInFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "tcp")
	content := strings.Join([]string{
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
		"   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000   501        0 12345 1 0000000000000000 100 0 0 10 0",
		"",
	}, "\n")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	inode, err := findListeningSocketInodeInFile(tmp, 8080)
	if err != nil {
		t.Fatalf("findListeningSocketInodeInFile returned error: %v", err)
	}
	if inode != "12345" {
		t.Fatalf("inode = %q, want %q", inode, "12345")
	}
}

func TestFindListeningSocketInodeInFile_NotFound(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "tcp")
	content := strings.Join([]string{
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
		"   0: 0100007F:1F90 00000000:0000 01 00000000:00000000 00:00000000 00000000   501        0 99999 1 0000000000000000 100 0 0 10 0",
		"",
	}, "\n")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	if _, err := findListeningSocketInodeInFile(tmp, 8080); err == nil {
		t.Fatal("expected socket not found error")
	}
}

func TestFindListeningSocketAndProcessHelpers(t *testing.T) {
	if _, err := findListeningSocketInode(-1); err == nil {
		t.Fatal("expected not found error for invalid port")
	}

	if _, _, err := findProcessBySocketInode("inode-that-does-not-exist"); err == nil {
		t.Fatal("expected process lookup error for missing inode")
	}
}

func TestDescribePortOccupantReturnsNonEmptyDescription(t *testing.T) {
	got := describePortOccupant(1)
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected non-empty process description")
	}
}
