package gateway

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
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
var knownHostsRepairer = repairKnownHostEntries
var knownHostEntryRemover = defaultKnownHostEntryRemover

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
	args, err := buildSSHArgs(host, wrapRemoteCommandForSSH(command))
	if err != nil {
		return remoteExecResult{}, err
	}
	result, err := sshExecRunner(ctx, args)
	result.Command = command
	if shouldRepairKnownHostMismatch(result, err) {
		if repairErr := knownHostsRepairer(host); repairErr == nil {
			result, err = sshExecRunner(ctx, args)
			result.Command = command
		}
	}
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
	args, err := buildSSHArgs(host, wrapRemoteCommandForSSH(command))
	if err != nil {
		return remoteExecResult{}, err
	}
	result, err := sshExecStreamRunner(ctx, args, onChunk)
	result.Command = command
	if shouldRepairKnownHostMismatch(result, err) {
		if repairErr := knownHostsRepairer(host); repairErr == nil {
			result, err = sshExecStreamRunner(ctx, args, onChunk)
			result.Command = command
		}
	}
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
		keyPath := strings.TrimSpace(h.KeyPath)
		if keyPath == "" && strings.TrimSpace(h.KeyRef) != "" {
			resolvedPath, err := resolveRemoteKeyPath(h.KeyRef)
			if err != nil {
				return nil, err
			}
			keyPath = resolvedPath
		}
		args = append(args,
			"-i", filepath.Clean(keyPath),
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

// shellSingleQuote is the canonical shell-safe quoting function for remote command arguments.
// It wraps input in single quotes and escapes embedded single quotes using the '\” idiom.
// Used in security-sensitive paths (e.g. wrapRemoteCommandForSSH) to prevent shell injection.
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

// wrapRemoteCommandForSSH wraps a command for execution via SSH using bash -lc.
// Note: bash -lc loads the remote user's login profile (.bash_profile / .bashrc),
// which ensures consistent PATH resolution but means any side effects in those
// files will execute on every remote command invocation.
func wrapRemoteCommandForSSH(command string) string {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return command
	}
	script := "export LC_ALL=C LANG=C; export PATH=\"$HOME/.npm-global/bin:$HOME/.local/bin:$PATH\"; " + cmd
	return "bash -lc " + shellSingleQuote(script)
}

func shouldRepairKnownHostMismatch(result remoteExecResult, err error) bool {
	if err != nil {
		return false
	}
	if result.ExitCode == 0 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(result.Stderr + "\n" + result.Stdout))
	if text == "" {
		return false
	}
	if strings.Contains(text, "remote host identification has changed") {
		return true
	}
	if strings.Contains(text, "host key verification failed") && strings.Contains(text, "offending") {
		return true
	}
	return false
}

func repairKnownHostEntries(host RemoteHost) error {
	h := normalizeRemoteHost(host)
	targetHost := strings.TrimSpace(h.Host)
	if h.AuthMode == RemoteAuthModeSSHConfig {
		targetHost = strings.TrimSpace(h.SSHConfigHost)
		if targetHost == "" {
			targetHost = strings.TrimSpace(h.Host)
		}
	}
	if targetHost == "" {
		return fmt.Errorf("target host is empty")
	}

	port := h.Port
	if port == 0 {
		port = 22
	}

	entries := []string{}
	if port == 22 {
		entries = append(entries, targetHost)
	} else {
		entries = append(entries, fmt.Sprintf("[%s]:%d", targetHost, port), targetHost)
	}

	repaired := false
	var lastErr error
	seen := map[string]struct{}{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		removed, err := knownHostEntryRemover(entry)
		if err == nil {
			if removed {
				repaired = true
			}
			continue
		}
		log.Printf("[gateway] repairKnownHostEntries: skipped entry %q removal error: %v", entry, err)
		lastErr = err
	}
	if repaired || lastErr == nil {
		return nil
	}
	return lastErr
}

func defaultKnownHostEntryRemover(entry string) (bool, error) {
	cmd := exec.Command("ssh-keygen", "-R", entry)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	msg := strings.ToLower(strings.TrimSpace(string(output)))
	if strings.Contains(msg, "not found in") {
		return false, nil
	}
	return false, fmt.Errorf("ssh-keygen -R %s failed: %w", entry, err)
}
