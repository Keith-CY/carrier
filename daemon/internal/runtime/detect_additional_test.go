package runtime

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestDetectBun_VersionFormat(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not installed")
	}

	_, version, err := DetectBun()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Version should not be empty and should not contain newlines
	if version == "" {
		t.Error("version should not be empty")
	}
	for _, c := range version {
		if c == '\n' || c == '\r' {
			t.Error("version should not contain newlines")
		}
	}
}

func TestEnsureBun_FindsBunIfPresent(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not installed")
	}

	path, err := EnsureBun()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestInstallBun_FailsWithInvalidPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	// Set PATH to empty so bash/curl can't be found
	t.Setenv("PATH", "/nonexistent")

	err := InstallBun()
	if err == nil {
		t.Fatal("expected error when bash/curl not available")
	}
}

func TestDetectBun_WithInvalidPath(t *testing.T) {
	t.Setenv("PATH", "/definitely/not/a/real/path")

	path, version, err := DetectBun()
	if err == nil {
		t.Fatal("expected error with invalid PATH")
	}
	if path != "" {
		t.Errorf("expected empty path, got %q", path)
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestDetectBun_WithEmptyPath(t *testing.T) {
	t.Setenv("PATH", "")

	_, _, err := DetectBun()
	if err == nil {
		t.Fatal("expected error with empty PATH")
	}
}

func TestPromptAndInstallBun_ReturnsBunIfPresent(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not installed")
	}

	// If bun is present, PromptAndInstallBun should return immediately
	path, err := PromptAndInstallBun()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}
