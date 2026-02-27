package gateway

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type remoteExecResult struct {
	Command    string `json:"command"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exitCode"`
	DurationMs int64  `json:"durationMs"`
}

type remoteStreamChunk struct {
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

var sshExecRunner = defaultSSHExecRunner
var sshExecStreamRunner = defaultSSHExecStreamRunner

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

func defaultSSHExecStreamRunner(ctx context.Context, args []string, onChunk func(remoteStreamChunk)) (remoteExecResult, error) {
	cmd := exec.CommandContext(ctx, "ssh", args...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return remoteExecResult{}, fmt.Errorf("prepare ssh stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return remoteExecResult{}, fmt.Errorf("prepare ssh stderr pipe: %w", err)
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return remoteExecResult{}, fmt.Errorf("start ssh: %w", err)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var cbMu sync.Mutex
	emit := func(chunk remoteStreamChunk) {
		if onChunk == nil {
			return
		}
		cbMu.Lock()
		onChunk(chunk)
		cbMu.Unlock()
	}
	appendLine := func(buf *bytes.Buffer, line string) {
		if buf.Len() > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			appendLine(&stdoutBuf, line)
			emit(remoteStreamChunk{Stream: "stdout", Text: line})
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			appendLine(&stderrBuf, line)
			emit(remoteStreamChunk{Stream: "stderr", Text: line})
		}
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	result := remoteExecResult{
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		DurationMs: time.Since(start).Milliseconds(),
	}
	if waitErr == nil {
		result.ExitCode = 0
		return result, nil
	}
	if ee, ok := waitErr.(*exec.ExitError); ok {
		result.ExitCode = ee.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("execute ssh stream: %w", waitErr)
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

func runRemoteCommandWithRetry(ctx context.Context, host RemoteHost, command string, maxRetries int) (remoteExecResult, error) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	var (
		lastResult remoteExecResult
		lastErr    error
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := runRemoteCommand(ctx, host, command)
		lastResult = result
		lastErr = err
		if err == nil && result.ExitCode == 0 {
			return result, nil
		}
		if attempt < maxRetries && isTransientRemoteFailure(err, result) {
			continue
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
	if lastErr != nil {
		return lastResult, lastErr
	}
	return lastResult, nil
}

func runRemoteCommandStream(ctx context.Context, host RemoteHost, command string, onChunk func(remoteStreamChunk)) (remoteExecResult, error) {
	args, err := buildSSHArgs(host, command)
	if err != nil {
		return remoteExecResult{}, err
	}
	result, err := sshExecStreamRunner(ctx, args, onChunk)
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

func isTransientRemoteFailure(err error, result remoteExecResult) bool {
	text := strings.ToLower(strings.TrimSpace(result.Stderr + " " + result.Stdout))
	if err != nil {
		text += " " + strings.ToLower(strings.TrimSpace(err.Error()))
	}
	if text == "" {
		return false
	}
	patterns := []string{
		"timeout",
		"temporarily unavailable",
		"connection reset",
		"broken pipe",
		"transport closed",
		"network is unreachable",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
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
