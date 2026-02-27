package codex

import (
	"context"
	"strings"
	"testing"

	"carrier/codeagent/contract"
)

func TestAdapterSupportsCapabilities(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(Options{})
	for _, capability := range []contract.Capability{
		contract.CapabilityReadFile,
		contract.CapabilityWriteFile,
		contract.CapabilityApplyPatch,
		contract.CapabilityRunShell,
		contract.CapabilityRunShellRedirect,
	} {
		if !adapter.Supports(capability) {
			t.Fatalf("expected capability %q to be supported", capability)
		}
	}
}

func TestAdapterResumeFallback(t *testing.T) {
	t.Parallel()

	calls := []string{}
	runner := func(_ context.Context, command string, args []string) (RunResult, error) {
		calls = append(calls, command+" "+strings.Join(args, " "))
		withResume := false
		for idx := range args {
			if args[idx] == "--resume" {
				withResume = true
				break
			}
		}
		if withResume {
			return RunResult{ExitCode: 1, Stderr: "failed to resume session"}, nil
		}
		return RunResult{ExitCode: 0, Stdout: `{"ok":true}`}, nil
	}

	adapter := NewAdapter(Options{Runner: runner})
	out, err := adapter.Run(context.Background(), contract.RunRequest{
		Capability:      contract.CapabilityRunShell,
		Command:         "ls -la",
		ResumeSessionID: "sess-123",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !out.Ok {
		t.Fatalf("expected successful fallback, got %+v", out)
	}
	if len(calls) != 2 {
		t.Fatalf("expected two codex invocations, got %d (%v)", len(calls), calls)
	}
	if !strings.Contains(calls[0], "--resume") {
		t.Fatalf("expected first call to include --resume, calls=%v", calls)
	}
	if strings.Contains(calls[1], "--resume") {
		t.Fatalf("expected second call to omit --resume, calls=%v", calls)
	}
	if out.CostEstimateUSD <= 0 {
		t.Fatalf("expected positive cost estimate, got %+v", out)
	}
}

func TestAdapterRetriesTransientFailures(t *testing.T) {
	t.Parallel()

	callCount := 0
	runner := func(_ context.Context, _ string, _ []string) (RunResult, error) {
		callCount++
		if callCount == 1 {
			return RunResult{ExitCode: 1, Stderr: "transport closed"}, nil
		}
		return RunResult{ExitCode: 0, Stdout: `{"ok":true}`}, nil
	}

	adapter := NewAdapter(Options{Runner: runner, MaxRetries: 1})
	out, err := adapter.Run(context.Background(), contract.RunRequest{
		Capability: contract.CapabilityRunShell,
		Command:    "ls -la",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !out.Ok {
		t.Fatalf("expected retry to recover transient failure, got %+v", out)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls with one retry, got %d", callCount)
	}
}
