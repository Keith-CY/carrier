package commandexec

import (
	"context"
	"strings"
	"testing"
	"time"
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

func TestShellRunnerTimeout(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Command that sleeps longer than the timeout
	result, err := runner.Run(ctx, "sleep 5")
	if err == nil {
		t.Fatal("expected timeout error")
	}

	// Exit code should be -1 for timeout/signal termination
	if result.ExitCode != -1 {
		t.Fatalf("expected exit code -1 for timeout, got %d", result.ExitCode)
	}
}

func TestShellRunnerNonZeroExitWithStderr(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}

	// Command that writes to stderr and exits with non-zero code
	result, err := runner.Run(context.Background(), "echo 'error message' >&2; exit 42")
	if err == nil {
		t.Fatal("expected command failure")
	}

	if result.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", result.ExitCode)
	}

	if !strings.Contains(result.CombinedOutput, "error message") {
		t.Fatalf("expected stderr output to be captured, got %q", result.CombinedOutput)
	}
}

func TestShellRunnerCapturesBothStdoutAndStderr(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}

	// Command that writes to both stdout and stderr
	result, err := runner.Run(context.Background(), "echo 'stdout'; echo 'stderr' >&2")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.CombinedOutput, "stdout") {
		t.Fatalf("expected stdout in output, got %q", result.CombinedOutput)
	}

	if !strings.Contains(result.CombinedOutput, "stderr") {
		t.Fatalf("expected stderr in output, got %q", result.CombinedOutput)
	}
}

func TestShellRunnerContextCancellation(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result, err := runner.Run(ctx, "sleep 10")
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	// Exit code should be -1 for cancellation
	if result.ExitCode != -1 {
		t.Fatalf("expected exit code -1 for cancellation, got %d", result.ExitCode)
	}
}

func TestShellRunnerExitCodeZeroOnSuccess(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}

	// Explicit exit 0 should work
	result, err := runner.Run(context.Background(), "exit 0")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestShellRunnerRunStreamingEmitsOutputLines(t *testing.T) {
	runner := ShellRunner{GOOS: "linux"}

	var lines []string
	result, err := runner.RunStreaming(context.Background(), "echo stdout-line; echo stderr-line >&2", func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if len(lines) == 0 {
		t.Fatal("expected streaming callback to receive at least one line")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "stdout-line") {
		t.Fatalf("expected stdout line in streamed output, got %v", lines)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "stderr-line") {
		t.Fatalf("expected stderr line in streamed output, got %v", lines)
	}
}
