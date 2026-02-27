package profilesync

import "testing"

func TestNormalizeSyncMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  SyncMode
	}{
		{name: "empty defaults to always_push", input: "", want: SyncModeAlwaysPush},
		{name: "manual lowercased", input: " MANUAL ", want: SyncModeManual},
		{name: "pull_validate_push keeps value", input: "pull_validate_push", want: SyncModePullValidatePush},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeSyncMode(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeSyncMode(%q)=%q want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateSyncMode(t *testing.T) {
	t.Parallel()

	validModes := []SyncMode{SyncModeAlwaysPush, SyncModePullValidatePush, SyncModeManual}
	for _, mode := range validModes {
		if err := ValidateSyncMode(mode); err != nil {
			t.Fatalf("expected mode %q to be valid: %v", mode, err)
		}
	}
	if err := ValidateSyncMode(SyncMode("invalid")); err == nil {
		t.Fatalf("expected invalid mode to fail validation")
	}
}
