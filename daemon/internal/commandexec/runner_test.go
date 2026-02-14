package commandexec

import (
	"context"
	"testing"
)

func TestShellRunnerSuccess(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}
	result, err := runner.Run(context.Background(), "printf hello")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.CombinedOutput != "hello" {
		t.Fatalf("expected output hello, got %q", result.CombinedOutput)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestShellRunnerFailureExitCode(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}
	result, err := runner.Run(context.Background(), "exit 17")
	if err == nil {
		t.Fatal("expected command failure")
	}
	if result.ExitCode != 17 {
		t.Fatalf("expected exit code 17, got %d", result.ExitCode)
	}
}
