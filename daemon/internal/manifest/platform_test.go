package manifest

import (
	"testing"
)

func TestPlatformCommandSpec_ResolveDefault(t *testing.T) {
	spec := PlatformCommandSpec{
		Command: "default-cmd",
	}
	if got := spec.Resolve("linux"); got != "default-cmd" {
		t.Errorf("expected default-cmd, got %q", got)
	}
}

func TestPlatformCommandSpec_ResolvePlatformOverride(t *testing.T) {
	spec := PlatformCommandSpec{
		Command: "default-cmd",
		Platforms: map[string]string{
			"linux":   "linux-cmd",
			"darwin":  "darwin-cmd",
			"windows": "windows-cmd",
		},
	}

	tests := []struct {
		goos     string
		expected string
	}{
		{"linux", "linux-cmd"},
		{"darwin", "darwin-cmd"},
		{"windows", "windows-cmd"},
		{"freebsd", "default-cmd"}, // unknown platform falls back
	}

	for _, tc := range tests {
		if got := spec.Resolve(tc.goos); got != tc.expected {
			t.Errorf("Resolve(%q) = %q, want %q", tc.goos, got, tc.expected)
		}
	}
}

func TestPlatformCommandSpec_NilPlatforms(t *testing.T) {
	spec := PlatformCommandSpec{
		Command:   "fallback",
		Platforms: nil,
	}
	if got := spec.Resolve("linux"); got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}
}

func TestPlatformCommandSpec_EmptyPlatforms(t *testing.T) {
	spec := PlatformCommandSpec{
		Command:   "fallback",
		Platforms: map[string]string{},
	}
	if got := spec.Resolve("linux"); got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}
}

func TestPlatformCommandSpec_ResolveForCurrentPlatform(t *testing.T) {
	spec := PlatformCommandSpec{
		Command: "default-cmd",
	}
	// Should not panic and return the default
	got := spec.ResolveForCurrentPlatform()
	if got != "default-cmd" {
		t.Errorf("expected default-cmd, got %q", got)
	}
}
