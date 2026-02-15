package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestProcessManager_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "test-agent-1"
	pid, err := pm.Start(agentID, "sleep", []string{"60"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("expected positive PID, got %d", pid)
	}

	// Verify process is running
	if !pm.IsRunning(agentID) {
		t.Error("expected process to be running")
	}

	// Stop the process
	if err := pm.Stop(agentID); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify process is no longer running
	if pm.IsRunning(agentID) {
		t.Error("expected process to be stopped")
	}

	// Verify log file was created
	logPath := filepath.Join(tmpDir, agentID+".log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("log file not created at %s", logPath)
	}
}

func TestProcessManager_StartAlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "test-agent-2"
	_, err := pm.Start(agentID, "sleep", []string{"60"})
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer func() {
		if err := pm.Stop(agentID); err != nil {
			t.Logf("cleanup stop failed: %v", err)
		}
	}()

	// Try to start again
	_, err = pm.Start(agentID, "sleep", []string{"60"})
	if err == nil {
		t.Error("expected error when starting already-running agent")
	}
}

func TestProcessManager_SIGTERMHandling(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	// Create a shell script that handles SIGTERM gracefully
	scriptPath := filepath.Join(tmpDir, "graceful.sh")
	script := `#!/bin/bash
trap 'exit 0' TERM
while true; do sleep 0.1; done
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("create script: %v", err)
	}

	agentID := "test-agent-3"
	_, err := pm.Start(agentID, "bash", []string{scriptPath})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give the process time to set up signal handler
	time.Sleep(200 * time.Millisecond)

	// Stop should complete within the 10-second grace period
	start := time.Now()
	if err := pm.Stop(agentID); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Stop took too long (expected <2s, got %v) - process may not have handled SIGTERM", elapsed)
	}

	if pm.IsRunning(agentID) {
		t.Error("process should be stopped")
	}
}

func TestProcessManager_IsRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "test-agent-4"

	// Before start
	if pm.IsRunning(agentID) {
		t.Error("should not be running before start")
	}

	// Start process
	_, err := pm.Start(agentID, "sleep", []string{"60"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		if err := pm.Stop(agentID); err != nil {
			t.Logf("cleanup stop failed: %v", err)
		}
	}()

	// After start
	if !pm.IsRunning(agentID) {
		t.Error("should be running after start")
	}

	// After manual kill
	pm.mu.RLock()
	info := pm.processes[agentID]
	pm.mu.RUnlock()
	if err := info.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("failed to send SIGKILL: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if pm.IsRunning(agentID) {
		t.Error("should not be running after kill")
	}
}

func TestProcessManager_Wait(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "test-agent-5"
	_, err := pm.Start(agentID, "sleep", []string{"0.5"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for process to exit
	start := time.Now()
	err = pm.Wait(agentID)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Wait returned error: %v", err)
	}

	if elapsed < 400*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("Wait returned too quickly or slowly (expected ~500ms, got %v)", elapsed)
	}

	if pm.IsRunning(agentID) {
		t.Error("process should have exited")
	}
}

func TestProcessManager_StopNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	err := pm.Stop("nonexistent-agent")
	if err == nil {
		t.Error("expected error when stopping nonexistent agent")
	}
}

func TestProcessManager_StopKillsChildProcessesViaProcessGroup(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	pidFile := filepath.Join(tmpDir, "child.pid")
	scriptPath := filepath.Join(tmpDir, "spawn-child.sh")
	script := "#!/usr/bin/env bash\nset -euo pipefail\nsleep 60 &\necho $! > \"" + pidFile + "\"\nwait\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("create script: %v", err)
	}

	agentID := "test-agent-child-group"
	if _, err := pm.Start(agentID, "bash", []string{scriptPath}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	var childPID int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			if _, scanErr := fmt.Sscanf(string(data), "%d", &childPID); scanErr == nil && childPID > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID <= 0 {
		t.Fatalf("failed to capture child PID from %s", pidFile)
	}

	if err := pm.Stop(agentID); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(childPID, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected child PID %d to be terminated", childPID)
}

func TestProcessManager_ForcedKillAfterTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	// Create a script that ignores SIGTERM
	scriptPath := filepath.Join(tmpDir, "stubborn.sh")
	script := `#!/bin/bash
trap '' TERM  # Ignore SIGTERM
while true; do sleep 0.1; done
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("create script: %v", err)
	}

	agentID := "test-agent-6"
	_, err := pm.Start(agentID, "bash", []string{scriptPath})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give script time to set up signal trap
	time.Sleep(200 * time.Millisecond)

	// Stop should force-kill after 10 seconds
	start := time.Now()
	if err := pm.Stop(agentID); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	elapsed := time.Since(start)

	// Should take approximately 10 seconds (grace period)
	if elapsed < 9*time.Second || elapsed > 12*time.Second {
		t.Errorf("Stop took unexpected time (expected ~10s, got %v)", elapsed)
	}

	if pm.IsRunning(agentID) {
		t.Error("process should be stopped after force kill")
	}
}

func TestProcessManager_LogFileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "test-agent-7"
	_, err := pm.Start(agentID, "echo", []string{"test log output"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for process to complete
	if err := pm.Wait(agentID); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}

	// Check log file exists and contains output
	logPath := filepath.Join(tmpDir, agentID+".log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("log file is empty")
	}
}

func TestProcessManager_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	// Start multiple agents
	agents := []string{"cleanup-1", "cleanup-2", "cleanup-3"}
	for _, id := range agents {
		if _, err := pm.Start(id, "sleep", []string{"60"}); err != nil {
			t.Fatalf("Start %s failed: %v", id, err)
		}
	}

	// Verify all are running
	for _, id := range agents {
		if !pm.IsRunning(id) {
			t.Errorf("agent %s should be running", id)
		}
	}

	// Cleanup all
	pm.Cleanup()

	// Verify all are stopped
	for _, id := range agents {
		if pm.IsRunning(id) {
			t.Errorf("agent %s should be stopped after cleanup", id)
		}
	}
}
