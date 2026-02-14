package commandexec

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type Result struct {
	CombinedOutput string
	ExitCode       int
}

type Runner interface {
	Run(ctx context.Context, command string) (Result, error)
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
	if err := ValidateCommand(command); err != nil {
		return Result{ExitCode: -1}, fmt.Errorf("validate command: %w", err)
	}

	var cmd *exec.Cmd
	if r.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "wsl.exe", "bash", "-lc", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	}

	out, err := cmd.CombinedOutput()
	result := Result{
		CombinedOutput: strings.TrimSpace(string(out)),
		ExitCode:       exitCode(err),
	}
	if err != nil {
		return result, fmt.Errorf("run command %q: %w", command, err)
	}
	return result, nil
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
