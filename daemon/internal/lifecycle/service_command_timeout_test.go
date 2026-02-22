package lifecycle

import (
	"testing"
	"time"
)

func TestLoadCommandTimeoutFromEnv_DefaultsToTwentyMinutes(t *testing.T) {
	t.Parallel()

	cases := []string{"", "not-a-duration", "-1m", "0s"}
	for _, raw := range cases {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if got := loadCommandTimeoutFromEnv(raw); got != 20*time.Minute {
				t.Fatalf("loadCommandTimeoutFromEnv(%q) = %s, want %s", raw, got, 20*time.Minute)
			}
		})
	}
}

func TestLoadCommandTimeoutFromEnv_UsesValidDuration(t *testing.T) {
	t.Parallel()

	if got := loadCommandTimeoutFromEnv("45m"); got != 45*time.Minute {
		t.Fatalf("loadCommandTimeoutFromEnv(%q) = %s, want %s", "45m", got, 45*time.Minute)
	}
}
