package codex

import (
	"context"
	"errors"
	"strings"
	"testing"

	"carrier/codeagent/contract"
)

func TestAdapterDefaultsAndNoopMethods(t *testing.T) {
	t.Parallel()

	a := NewAdapter(Options{Binary: " ", MaxRetries: -3})
	if a.binary != "codex" {
		t.Fatalf("expected default binary codex, got %q", a.binary)
	}
	if a.runner == nil {
		t.Fatalf("expected default runner")
	}
	if a.maxRetries != 0 {
		t.Fatalf("expected maxRetries to clamp to 0, got %d", a.maxRetries)
	}

	if err := a.Install(context.Background(), contract.Target{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if err := a.Configure(context.Background(), contract.Target{}, contract.Profile{}); err != nil {
		t.Fatalf("Configure returned error: %v", err)
	}
}

func TestAdapterHealthAndVersionBranches(t *testing.T) {
	t.Parallel()

	t.Run("health error", func(t *testing.T) {
		a := NewAdapter(Options{Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{}, errors.New("boom")
		}})
		if err := a.Health(context.Background()); err == nil {
			t.Fatal("expected health error")
		}
	})

	t.Run("version from stdout", func(t *testing.T) {
		a := NewAdapter(Options{Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{Stdout: " codex v1.2.3 "}, nil
		}})
		version, err := a.Version(context.Background())
		if err != nil {
			t.Fatalf("Version returned error: %v", err)
		}
		if version != "codex v1.2.3" {
			t.Fatalf("unexpected version %q", version)
		}
	})

	t.Run("version from stderr", func(t *testing.T) {
		a := NewAdapter(Options{Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{Stderr: " codex v1.2.4 "}, nil
		}})
		version, err := a.Version(context.Background())
		if err != nil {
			t.Fatalf("Version returned error: %v", err)
		}
		if version != "codex v1.2.4" {
			t.Fatalf("unexpected version %q", version)
		}
	})

	t.Run("version empty output", func(t *testing.T) {
		a := NewAdapter(Options{Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{}, nil
		}})
		if _, err := a.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "empty codex version output") {
			t.Fatalf("expected empty-output error, got %v", err)
		}
	})

	t.Run("version runner error", func(t *testing.T) {
		a := NewAdapter(Options{Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{}, errors.New("version failed")
		}})
		if _, err := a.Version(context.Background()); err == nil || !strings.Contains(err.Error(), "version failed") {
			t.Fatalf("expected runner error, got %v", err)
		}
	})
}

func TestAdapterPromptAndArgsBranches(t *testing.T) {
	t.Parallel()

	a := NewAdapter(Options{Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
		return RunResult{ExitCode: 0}, nil
	}})

	cases := []contract.RunRequest{
		{Capability: contract.CapabilityReadFile, Path: " /tmp/a "},
		{Capability: contract.CapabilityWriteFile, Path: "/tmp/b", Content: "x", WriteMode: contract.WriteModeAppend},
		{Capability: contract.CapabilityWriteFile, Path: "/tmp/c", Content: "x"},
		{Capability: contract.CapabilityApplyPatch, Content: "@@"},
		{Capability: contract.CapabilityRunShell, Command: " echo hi "},
		{Capability: contract.CapabilityRunShellRedirect, Command: "ls", StdoutPath: "o", StderrPath: "e", AppendOutput: true},
		{Capability: contract.Capability("unknown")},
	}
	for _, req := range cases {
		prompt := a.requestPrompt(req)
		if strings.TrimSpace(prompt) == "" {
			t.Fatalf("requestPrompt returned empty prompt for %+v", req)
		}
	}

	argsWithResume := a.buildExecArgs(contract.RunRequest{Capability: contract.CapabilityRunShell, Command: "ls", ResumeSessionID: "sess-1"}, true)
	if !containsArg(argsWithResume, "--resume") {
		t.Fatalf("expected --resume in args: %v", argsWithResume)
	}
	argsWithoutResume := a.buildExecArgs(contract.RunRequest{Capability: contract.CapabilityRunShell, Command: "ls", ResumeSessionID: "sess-1"}, false)
	if containsArg(argsWithoutResume, "--resume") {
		t.Fatalf("did not expect --resume in args: %v", argsWithoutResume)
	}
}

func TestAdapterRunWithRetryErrorBranches(t *testing.T) {
	t.Parallel()

	calls := 0
	a := NewAdapter(Options{
		MaxRetries: 1,
		Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			calls++
			if calls == 1 {
				return RunResult{}, errors.New("timeout")
			}
			return RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	})
	res, err := a.runWithRetry(context.Background(), []string{"exec"})
	if err != nil || res.ExitCode != 0 || calls != 2 {
		t.Fatalf("unexpected retry success result=%+v err=%v calls=%d", res, err, calls)
	}

	aNoRetry := NewAdapter(Options{
		MaxRetries: 1,
		Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{}, errors.New("permission denied")
		},
	})
	if _, err := aNoRetry.runWithRetry(context.Background(), []string{"exec"}); err == nil {
		t.Fatal("expected non-transient error without retry success")
	}
}

func TestDefaultRunnerBranches(t *testing.T) {
	t.Parallel()

	okRes, okErr := defaultRunner(context.Background(), "go", []string{"version"})
	if okErr != nil || okRes.ExitCode != 0 {
		t.Fatalf("expected success branch, result=%+v err=%v", okRes, okErr)
	}

	exitRes, exitErr := defaultRunner(context.Background(), "go", []string{"tool", "definitely-missing-subcommand"})
	if exitErr != nil {
		t.Fatalf("expected exit-error branch to return nil error, got %v", exitErr)
	}
	if exitRes.ExitCode == 0 {
		t.Fatalf("expected non-zero exit code for failing command: %+v", exitRes)
	}

	_, err := defaultRunner(context.Background(), "definitely-missing-command-carrier", nil)
	if err == nil || !strings.Contains(err.Error(), "run codex command") {
		t.Fatalf("expected wrapped runner error, got %v", err)
	}
}

func TestAdapterAdditionalBranches(t *testing.T) {
	t.Parallel()

	a := NewAdapter(Options{
		Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{ExitCode: 1, Stderr: "generic failure"}, nil
		},
	})
	out, err := a.Run(context.Background(), contract.RunRequest{
		Capability:      contract.CapabilityRunShell,
		Command:         "ls -la",
		ResumeSessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if out.Ok {
		t.Fatalf("expected non-fallback failed run result, got %+v", out)
	}

	aErr := NewAdapter(Options{
		Runner: func(_ context.Context, _ string, _ []string) (RunResult, error) {
			return RunResult{}, errors.New("hard fail")
		},
	})
	if _, err := aErr.Run(context.Background(), contract.RunRequest{
		Capability: contract.CapabilityRunShell,
		Command:    "ls -la",
	}); err == nil {
		t.Fatal("expected run error path")
	}

	if a.Supports(contract.Capability("custom")) {
		t.Fatalf("expected unknown capability to be unsupported")
	}
	if estimateCostUSD(contract.RunRequest{}, RunResult{}) != 0 {
		t.Fatalf("expected zero cost estimate for empty request/result")
	}
	if isTransientExecutionError("") {
		t.Fatalf("expected empty error message not transient")
	}
	if isTransientExecutionError("permission denied") {
		t.Fatalf("expected non-transient message")
	}
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}
