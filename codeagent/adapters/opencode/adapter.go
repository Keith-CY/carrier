package opencode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"carrier/codeagent/contract"
)

type Runner func(ctx context.Context, command string, args []string) (RunResult, error)

type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Options struct {
	Binary     string
	Runner     Runner
	MaxRetries int
}

type Adapter struct {
	binary     string
	runner     Runner
	maxRetries int
}

func NewAdapter(opts Options) *Adapter {
	binary := strings.TrimSpace(opts.Binary)
	if binary == "" {
		binary = "opencode"
	}
	runner := opts.Runner
	if runner == nil {
		runner = defaultRunner
	}
	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &Adapter{
		binary:     binary,
		runner:     runner,
		maxRetries: maxRetries,
	}
}

func (a *Adapter) Install(context.Context, contract.Target) error {
	return nil
}

func (a *Adapter) Configure(context.Context, contract.Target, contract.Profile) error {
	return nil
}

func (a *Adapter) Run(ctx context.Context, request contract.RunRequest) (contract.ResultEnvelope, error) {
	started := time.Now()
	args := a.buildExecArgs(request, true)
	runResult, err := a.runWithRetry(ctx, args)
	if err != nil {
		return contract.ResultEnvelope{}, err
	}

	shouldFallback := strings.TrimSpace(request.ResumeSessionID) != "" &&
		runResult.ExitCode != 0 &&
		strings.Contains(strings.ToLower(runResult.Stderr), "resume")
	if shouldFallback {
		args = a.buildExecArgs(request, false)
		runResult, err = a.runWithRetry(ctx, args)
		if err != nil {
			return contract.ResultEnvelope{}, err
		}
	}

	cost := estimateCostUSD(request, runResult)
	return contract.ResultEnvelope{
		Ok:              runResult.ExitCode == 0,
		ExitCode:        runResult.ExitCode,
		Stdout:          strings.TrimSpace(runResult.Stdout),
		Stderr:          strings.TrimSpace(runResult.Stderr),
		DurationMS:      time.Since(started).Milliseconds(),
		CostEstimateUSD: cost,
		PolicyDecision:  contract.PolicyDecisionAllow,
	}, nil
}

func (a *Adapter) Health(ctx context.Context) error {
	_, err := a.runner(ctx, a.binary, []string{"--version"})
	return err
}

func (a *Adapter) Version(ctx context.Context) (string, error) {
	out, err := a.runner(ctx, a.binary, []string{"--version"})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(out.Stdout)
	if text == "" {
		text = strings.TrimSpace(out.Stderr)
	}
	if text == "" {
		return "", errors.New("empty opencode version output")
	}
	return text, nil
}

func (a *Adapter) Supports(capability contract.Capability) bool {
	switch capability {
	case contract.CapabilityReadFile,
		contract.CapabilityWriteFile,
		contract.CapabilityApplyPatch,
		contract.CapabilityRunShell,
		contract.CapabilityRunShellRedirect:
		return true
	default:
		return false
	}
}

func (a *Adapter) buildExecArgs(request contract.RunRequest, includeResume bool) []string {
	args := []string{"exec", a.requestPrompt(request), "--json"}
	if includeResume && strings.TrimSpace(request.ResumeSessionID) != "" {
		args = append(args, "--resume", strings.TrimSpace(request.ResumeSessionID))
	}
	return args
}

func (a *Adapter) requestPrompt(request contract.RunRequest) string {
	switch request.Capability {
	case contract.CapabilityReadFile:
		return fmt.Sprintf("Read file: %s", strings.TrimSpace(request.Path))
	case contract.CapabilityWriteFile:
		mode := request.WriteMode
		if mode == "" {
			mode = contract.WriteModeOverwrite
		}
		return fmt.Sprintf("Write file (%s): %s\n%s", mode, strings.TrimSpace(request.Path), request.Content)
	case contract.CapabilityApplyPatch:
		return "Apply patch:\n" + request.Content
	case contract.CapabilityRunShell:
		return "Run shell command:\n" + strings.TrimSpace(request.Command)
	case contract.CapabilityRunShellRedirect:
		return fmt.Sprintf(
			"Run shell command with redirect:\ncommand=%s\nstdout=%s\nstderr=%s\nappend=%t",
			strings.TrimSpace(request.Command),
			strings.TrimSpace(request.StdoutPath),
			strings.TrimSpace(request.StderrPath),
			request.AppendOutput,
		)
	default:
		return "Run request"
	}
}

func (a *Adapter) runWithRetry(ctx context.Context, args []string) (RunResult, error) {
	var (
		lastResult RunResult
		lastErr    error
	)
	for attempt := 0; attempt <= a.maxRetries; attempt++ {
		runResult, err := a.runner(ctx, a.binary, args)
		if err != nil {
			lastErr = err
			if attempt < a.maxRetries && isTransientExecutionError(err.Error()) {
				continue
			}
			return runResult, err
		}
		lastResult = runResult
		if runResult.ExitCode == 0 {
			return runResult, nil
		}
		if attempt < a.maxRetries && isTransientExecutionError(runResult.Stderr+" "+runResult.Stdout) {
			continue
		}
		return runResult, nil
	}
	return lastResult, lastErr
}

func isTransientExecutionError(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}
	patterns := []string{
		"timeout",
		"temporarily unavailable",
		"connection reset",
		"broken pipe",
		"transport closed",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func estimateCostUSD(request contract.RunRequest, result RunResult) float64 {
	charCount := len(request.Command) + len(request.Content) + len(request.Path) + len(result.Stdout) + len(result.Stderr)
	if charCount <= 0 {
		return 0
	}
	tokens := float64(charCount) / 4.0
	return tokens * 0.00001
}

func defaultRunner(ctx context.Context, command string, args []string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	out := RunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if runErr == nil {
		out.ExitCode = 0
		return out, nil
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		out.ExitCode = exitErr.ExitCode()
		return out, nil
	}
	return out, fmt.Errorf("run opencode command: %w", runErr)
}
