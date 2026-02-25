package lifecycle

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"carrier/daemon/internal/commandexec"
	"carrier/daemon/internal/manifest"
	"carrier/daemon/internal/runtimecheck"
	"carrier/shared/redact"
)

func (s *Service) getManifestAndState(agentID string) (manifest.Manifest, AgentState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.manifests[agentID]
	if !ok {
		return manifest.Manifest{}, AgentState{}, ErrAgentNotFound
	}
	state := s.states[agentID]
	return m, state, nil
}

func (s *Service) appendCommandLog(agentID, action, command string, result commandexec.Result, runErr error) {
	line := fmt.Sprintf("[%s] command=%q exit=%d", action, command, result.ExitCode)
	s.appendLog(agentID, line)
	if result.CombinedOutput != "" {
		s.appendLog(agentID, fmt.Sprintf("[%s] output=%s", action, result.CombinedOutput))
	}
	if runErr != nil {
		s.appendLog(agentID, fmt.Sprintf("[%s] error=%v", action, runErr))
	}
}

func (s *Service) appendCommandLogSummary(agentID, action, command string, result commandexec.Result, runErr error) {
	line := fmt.Sprintf("[%s] command=%q exit=%d", action, command, result.ExitCode)
	s.appendLog(agentID, line)
	if runErr != nil {
		s.appendLog(agentID, fmt.Sprintf("[%s] error=%v", action, runErr))
	}
}

func (s *Service) runCommandWithAgentLogs(ctx context.Context, agentID, action, command string) (commandexec.Result, bool, error) {
	streamingRunner, ok := s.runner.(commandexec.StreamingRunner)
	if !ok {
		result, err := s.runner.Run(ctx, command)
		return result, false, err
	}
	result, err := streamingRunner.RunStreaming(ctx, command, func(line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return
		}
		s.appendLog(agentID, fmt.Sprintf("[%s] %s", action, trimmed))
	})
	return result, true, err
}

func (s *Service) appendLog(agentID, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.logs[agentID]
	if !ok {
		return
	}
	entry := fmt.Sprintf("%s %s", s.now().UTC().Format(time.RFC3339), line)
	entries = append(entries, entry)
	if len(entries) > s.logLimit {
		entries = entries[len(entries)-s.logLimit:]
	}
	s.logs[agentID] = entries
}

func (s *Service) updateStateOnInstallError(agentID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[agentID]
	if !ok {
		return
	}
	state.Install = InstallStateBroken
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	state.UpdatedAt = s.now()
	s.states[agentID] = state
}

func (s *Service) updateStateOnStartError(agentID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[agentID]
	if !ok {
		return
	}
	now := s.now()
	restarts := append(s.restarts[agentID], now)
	restarts = trimRestartHistory(restarts, now.Add(-s.crashLoopWindow))
	s.restarts[agentID] = restarts
	state.Runtime = RuntimeStateCrashing
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	if len(restarts) >= s.crashLoopThreshold {
		cooldownUntil := now.Add(s.crashLoopCooldown)
		s.cooldowns[agentID] = cooldownUntil
		state.Runtime = RuntimeStateCrashLoop
		state.LastError = fmt.Sprintf(
			"crash-loop detected: %d restarts within %s; cooldown until %s; last error: %v",
			len(restarts),
			s.crashLoopWindow.String(),
			cooldownUntil.UTC().Format(time.RFC3339),
			err,
		)
	}
	state.UpdatedAt = now
	s.states[agentID] = state
}

func (s *Service) updateStateOnUpgradeError(agentID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[agentID]
	if !ok {
		return
	}
	state.Health = HealthStateUnhealthy
	state.LastError = err.Error()
	state.UpdatedAt = s.now()
	s.states[agentID] = state
}

func (s *Service) checkRuntimePrerequisites(m manifest.Manifest) error {
	if err := s.checker.Check(m); err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimePrerequisites, err)
	}
	return nil
}

func formatPreFlightFailures(result runtimecheck.PreFlightResult) string {
	var parts []string
	for _, c := range result.Checks {
		if !c.Passed {
			msg := c.Message
			if c.Repair != "" {
				msg += " (fix: " + c.Repair + ")"
			}
			parts = append(parts, msg)
		}
	}
	return strings.Join(parts, "; ")
}

func firstFailedCode(result runtimecheck.PreFlightResult) string {
	for _, c := range result.Checks {
		if !c.Passed && c.Code != "" {
			return c.Code
		}
	}
	return "E_PREFLIGHT_FAILED"
}

func (s *Service) validateRequiredEnv(m manifest.Manifest) error {
	missing := make([]string, 0)
	for _, envVar := range m.Env.Required {
		if strings.TrimSpace(os.Getenv(envVar.Name)) == "" {
			missing = append(missing, envVar.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrMissingRequiredEnv, strings.Join(missing, ","))
}

func (s *Service) ensurePortsAvailable(ports []manifest.PortSpec) error {
	for _, port := range ports {
		if port.Port <= 0 {
			continue
		}
		addr := fmt.Sprintf("127.0.0.1:%d", port.Port)
		ln, err := listenTCP("tcp", addr)
		if err != nil {
			return fmt.Errorf("%w: %s (%d) is in use by %s", ErrPortConflict, port.Name, port.Port, portOccupantFor(port.Port))
		}
		_ = ln.Close()
	}
	return nil
}

func (s *Service) blockIfCrashLoopCoolingDown(agentID string, state AgentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cooldownUntil, ok := s.cooldowns[agentID]
	if !ok || cooldownUntil.IsZero() {
		return nil
	}

	now := s.now()
	if !now.Before(cooldownUntil) {
		delete(s.cooldowns, agentID)
		delete(s.restarts, agentID)
		return nil
	}

	restartCount := len(trimRestartHistory(s.restarts[agentID], now.Add(-s.crashLoopWindow)))
	state.Runtime = RuntimeStateCrashLoop
	state.Health = HealthStateUnhealthy
	state.LastError = fmt.Sprintf(
		"crash-loop detected: %d restarts within %s; cooldown until %s",
		restartCount,
		s.crashLoopWindow.String(),
		cooldownUntil.UTC().Format(time.RFC3339),
	)
	state.UpdatedAt = now
	s.states[agentID] = state

	return fmt.Errorf("%w: %s", ErrCrashLoop, state.LastError)
}

func trimRestartHistory(history []time.Time, windowStart time.Time) []time.Time {
	if len(history) == 0 {
		return history
	}
	firstKept := 0
	for firstKept < len(history) && history[firstKept].Before(windowStart) {
		firstKept++
	}
	if firstKept == 0 {
		return history
	}
	if firstKept >= len(history) {
		return nil
	}
	return append([]time.Time(nil), history[firstKept:]...)
}

func describePortOccupant(port int) string {
	if runtime.GOOS != "linux" {
		return "an unknown process"
	}

	inode, err := findListeningSocketInode(port)
	if err != nil || inode == "" {
		return "an unknown process"
	}

	pid, processName, err := findProcessBySocketInode(inode)
	if err != nil || pid <= 0 {
		return fmt.Sprintf("socket inode %s (pid unknown)", inode)
	}
	if processName == "" {
		return fmt.Sprintf("pid %d", pid)
	}
	return fmt.Sprintf("pid %d (%s)", pid, processName)
}

func findListeningSocketInode(port int) (string, error) {
	for _, procFile := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		inode, err := findListeningSocketInodeInFile(procFile, port)
		if err == nil && inode != "" {
			return inode, nil
		}
	}
	return "", errors.New("listening socket not found")
}

func findListeningSocketInodeInFile(path string, port int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "sl") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		localAddress := fields[1]
		state := fields[3]
		if state != "0A" {
			continue
		}
		parts := strings.Split(localAddress, ":")
		if len(parts) != 2 {
			continue
		}
		p, err := strconv.ParseInt(parts[1], 16, 32)
		if err != nil || int(p) != port {
			continue
		}
		return fields[9], nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("socket not found")
}

func findProcessBySocketInode(inode string) (int, string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", err
	}
	target := "socket:[" + inode + "]"
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			linkPath := filepath.Join(fdDir, fd.Name())
			link, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}
			if link != target {
				continue
			}
			nameBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
			if err != nil {
				return pid, "", nil
			}
			return pid, strings.TrimSpace(string(nameBytes)), nil
		}
	}
	return 0, "", errors.New("process not found")
}

func (s *Service) writeDiagnoseZip(path string, m manifest.Manifest, state AgentState, logs []string, createdAt time.Time) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create diagnose zip: %w", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)

	// Build structured diagnose manifest
	diagnoseManifest := buildDiagnoseManifest(state.ID)
	diagnoseManifestJSON, err := json.MarshalIndent(diagnoseManifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal diagnose manifest: %w", err)
	}

	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	agentManifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent manifest: %w", err)
	}

	redactedEnvJSON, err := json.MarshalIndent(redact.RedactEnviron(os.Environ()), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal env: %w", err)
	}

	artifacts := map[string][]byte{
		"manifest.json":       diagnoseManifestJSON,
		"state.json":          []byte(redact.RedactText(string(stateJSON))),
		"agent_manifest.json": []byte(redact.RedactText(string(agentManifestJSON))),
		"logs.txt":            []byte(redact.RedactText(strings.Join(logs, "\n"))),
		"env.json":            redactedEnvJSON,
	}

	metadataJSON, err := redact.MetadataJSON(createdAt, 24*time.Hour, artifacts)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	artifacts["metadata.json"] = metadataJSON

	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := addZipFile(zipWriter, name, artifacts[name]); err != nil {
			return err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}

	return nil
}

func addZipFile(zw *zip.Writer, name string, content []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	if _, err := w.Write(content); err != nil {
		return fmt.Errorf("write zip entry %s: %w", name, err)
	}
	return nil
}

// recordRestart records a process restart for crash-loop detection.
// Must be called with lock held.
func (s *Service) recordRestart(agentID string) {
	now := s.now()
	restarts := append(s.restarts[agentID], now)
	restarts = trimRestartHistory(restarts, now.Add(-s.crashLoopWindow))
	s.restarts[agentID] = restarts

	if len(restarts) >= s.crashLoopThreshold {
		cooldownUntil := now.Add(s.crashLoopCooldown)
		s.cooldowns[agentID] = cooldownUntil
	}
}
