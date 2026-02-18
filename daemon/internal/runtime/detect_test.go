package runtime

import (
	"os/exec"
	"testing"
)

func TestDetectBun_ReturnsErrorWhenMissing(t *testing.T) {
	// Save and clear PATH to simulate bun not being installed.
	t.Setenv("PATH", "/nonexistent")

	_, _, err := DetectBun()
	if err == nil {
		t.Fatal("expected error when bun is not in PATH")
	}
}

func TestDetectBun_FindsBunIfPresent(t *testing.T) {
	// Skip if bun is not installed in the test environment.
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not installed, skipping")
	}

	path, version, err := DetectBun()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	if version == "" {
		t.Error("expected non-empty version")
	}
}

func TestEnsureBun_ReturnsErrorWhenMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	_, err := EnsureBun()
	if err == nil {
		t.Fatal("expected error when bun is not in PATH")
	}
}
