package gateway

import (
	"strings"
	"testing"
)

func TestRedactErrorMessage(t *testing.T) {
	tests := []struct {
		input   string
		notWant string
		wantSub string
	}{
		{
			input:   "failed: MY_API_KEY=abc123",
			notWant: "abc123",
			wantSub: "***REDACTED***",
		},
		{
			input:   "error: TOKEN=supersecret",
			notWant: "supersecret",
			wantSub: "***REDACTED***",
		},
		{
			input:   "connect to postgres://user:pass@localhost",
			notWant: "pass",
			wantSub: "***REDACTED***",
		},
		{
			input:   "just a normal error message",
			wantSub: "just a normal error message",
		},
	}
	for _, tc := range tests {
		got := RedactErrorMessage(tc.input)
		if tc.notWant != "" && strings.Contains(got, tc.notWant) {
			t.Errorf("RedactErrorMessage(%q) = %q, should not contain %q", tc.input, got, tc.notWant)
		}
		if tc.wantSub != "" && !strings.Contains(got, tc.wantSub) {
			t.Errorf("RedactErrorMessage(%q) = %q, should contain %q", tc.input, got, tc.wantSub)
		}
	}
}
