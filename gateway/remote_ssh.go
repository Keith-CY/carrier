package gateway

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type remoteExecResult struct {
	Command    string `json:"command"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exitCode"`
	DurationMs int64  `json:"durationMs"`
}

var sshExecRunner = defaultSSHExecRunner

func defaultSSHExecRunner(ctx context.Context, args []string) (remoteExecResult, error) {
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)
	result := remoteExecResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: duration.Milliseconds(),
	}
	if runErr == nil {
		result.ExitCode = 0
		return result, nil
	}
	if ee, ok := runErr.(*exec.ExitError); ok {
		result.ExitCode = ee.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("execute ssh: %w", runErr)
}

func runRemoteCommand(ctx context.Context, host RemoteHost, command string) (remoteExecResult, error) {
	args, err := buildSSHArgs(host, command)
	if err != nil {
		return remoteExecResult{}, err
	}
	result, err := sshExecRunner(ctx, args)
	result.Command = command
	return result, err
}

func buildSSHArgs(host RemoteHost, remoteCommand string) ([]string, error) {
	h := normalizeRemoteHost(host)
	if err := validateRemoteHost(h); err != nil {
		return nil, err
	}
	command := strings.TrimSpace(remoteCommand)
	if command == "" {
		return nil, fmt.Errorf("remote command is empty")
	}

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
	}
	if h.Port > 0 {
		args = append(args, "-p", strconv.Itoa(h.Port))
	}
	if h.AuthMode == RemoteAuthModePrivateKey {
		args = append(args,
			"-i", filepath.Clean(h.KeyPath),
			"-o", "IdentitiesOnly=yes",
		)
	}

	targetHost := strings.TrimSpace(h.Host)
	if h.AuthMode == RemoteAuthModeSSHConfig {
		targetHost = strings.TrimSpace(h.SSHConfigHost)
		if targetHost == "" {
			targetHost = strings.TrimSpace(h.Host)
		}
	}
	if targetHost == "" {
		return nil, fmt.Errorf("target host is empty")
	}
	target := targetHost
	if strings.TrimSpace(h.User) != "" {
		target = strings.TrimSpace(h.User) + "@" + targetHost
	}

	args = append(args, target, command)
	return args, nil
}

func shellSingleQuote(input string) string {
	if input == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(input, "'", "'\\''") + "'"
}

func remoteCommandError(result remoteExecResult, action string) error {
	stderr := strings.TrimSpace(result.Stderr)
	stdout := strings.TrimSpace(result.Stdout)
	if stderr == "" {
		stderr = stdout
	}
	if stderr == "" {
		stderr = "remote command failed"
	}
	return fmt.Errorf("%s failed (exit %d): %s", action, result.ExitCode, RedactErrorMessage(stderr))
}
