package commandexec

import (
	"context"
	"strings"
	"testing"
)

func TestValidateCommand_Empty(t *testing.T) {
	for _, cmd := range []string{"", "   ", "\t\n"} {
		if err := ValidateCommand(cmd); err == nil {
			t.Errorf("expected error for empty command %q", cmd)
		}
	}
}

func TestValidateCommand_NullByte(t *testing.T) {
	if err := ValidateCommand("echo\x00injected"); err == nil {
		t.Error("expected error for command with null byte")
	}
}

func TestValidateCommand_Valid(t *testing.T) {
	if err := ValidateCommand("echo hello"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShellRunner_RejectsEmptyCommand(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}
	_, err := runner.Run(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty command error, got: %v", err)
	}
}

func TestShellRunner_RejectsNullByte(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}
	_, err := runner.Run(context.Background(), "echo\x00world")
	if err == nil {
		t.Fatal("expected error for null byte command")
	}
}

// TestShellRunner_InjectionPatterns verifies that shell metacharacters in
// manifest commands don't cause unexpected behavior — they are passed to sh
// as-is, which is expected since manifests are trusted.
func TestShellRunner_InjectionPatterns(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}

	// Semicolon injection: both commands run, but that's fine since the
	// entire string comes from a trusted manifest.
	result, err := runner.Run(context.Background(), "printf safe; printf also_safe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.CombinedOutput, "safe") {
		t.Fatalf("unexpected output: %s", result.CombinedOutput)
	}
}
