package commandexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

type Result struct {
	CombinedOutput string
	ExitCode       int
}

type Runner interface {
	Run(ctx context.Context, command string) (Result, error)
}

type StreamingRunner interface {
	RunStreaming(ctx context.Context, command string, onLine func(string)) (Result, error)
}

// ValidateCommand checks that a command string is safe to execute.
// It rejects empty commands and commands containing null bytes.
func ValidateCommand(command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("command must not be empty")
	}
	for i := 0; i < len(command); i++ {
		if command[i] == 0 {
			return errors.New("command must not contain null bytes")
		}
	}
	return nil
}

type ShellRunner struct {
	GOOS string
}

func NewShellRunner() ShellRunner {
	return ShellRunner{GOOS: runtime.GOOS}
}

func (r ShellRunner) Run(ctx context.Context, command string) (Result, error) {
	return r.run(ctx, command, nil)
}

func (r ShellRunner) RunStreaming(ctx context.Context, command string, onLine func(string)) (Result, error) {
	return r.run(ctx, command, onLine)
}

func (r ShellRunner) run(ctx context.Context, command string, onLine func(string)) (Result, error) {
	if err := ValidateCommand(command); err != nil {
		return Result{ExitCode: -1}, fmt.Errorf("validate command: %w", err)
	}

	var cmd *exec.Cmd
	if r.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "wsl.exe", "bash", "-lc", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	}

	stream := &lineCaptureWriter{onLine: onLine}
	cmd.Stdout = stream
	cmd.Stderr = stream
	err := cmd.Run()
	stream.Flush()
	result := Result{
		CombinedOutput: stream.CombinedOutput(),
		ExitCode:       exitCode(err),
	}
	if err != nil {
		return result, fmt.Errorf("run command %q: %w", command, err)
	}
	return result, nil
}

type lineCaptureWriter struct {
	mu      sync.Mutex
	output  bytes.Buffer
	pending string
	onLine  func(string)
}

func (w *lineCaptureWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	_, _ = w.output.Write(p)
	chunk := w.pending + string(p)
	start := 0
	for start < len(chunk) {
		idx := strings.IndexByte(chunk[start:], '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSuffix(chunk[start:start+idx], "\r")
		if w.onLine != nil {
			w.onLine(line)
		}
		start += idx + 1
	}
	w.pending = chunk[start:]
	return len(p), nil
}

func (w *lineCaptureWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending == "" {
		return
	}
	line := strings.TrimSuffix(w.pending, "\r")
	if w.onLine != nil {
		w.onLine(line)
	}
	w.pending = ""
}

func (w *lineCaptureWriter) CombinedOutput() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(w.output.String())
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
