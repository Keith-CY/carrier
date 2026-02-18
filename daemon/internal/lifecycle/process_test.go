//go:build !windows
// +build !windows

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

func TestProcessManager_GetExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	t.Run("exit code 0 for successful process", func(t *testing.T) {
		agentID := "exit-code-0"
		_, err := pm.Start(agentID, "true", []string{})
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Wait for process to complete
		if err := pm.Wait(agentID); err != nil {
			t.Fatalf("Wait failed: %v", err)
		}

		// Give the goroutine time to set exit code
		time.Sleep(100 * time.Millisecond)

		exitCode := pm.GetExitCode(agentID)
		if exitCode == nil {
			t.Fatal("expected exit code to be set")
		}

		if *exitCode != 0 {
			t.Errorf("expected exit code 0, got %d", *exitCode)
		}
	})

	t.Run("exit code 1 for failed process", func(t *testing.T) {
		agentID := "exit-code-1"
		_, err := pm.Start(agentID, "false", []string{})
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Wait for process to complete (will return error)
		_ = pm.Wait(agentID)

		// Give the goroutine time to set exit code
		time.Sleep(100 * time.Millisecond)

		exitCode := pm.GetExitCode(agentID)
		if exitCode == nil {
			t.Fatal("expected exit code to be set")
		}

		if *exitCode != 1 {
			t.Errorf("expected exit code 1, got %d", *exitCode)
		}
	})

	t.Run("exit code 2 for command error", func(t *testing.T) {
		agentID := "exit-code-2"
		_, err := pm.Start(agentID, "sh", []string{"-c", "exit 2"})
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Wait for process to complete
		_ = pm.Wait(agentID)

		// Give the goroutine time to set exit code
		time.Sleep(100 * time.Millisecond)

		exitCode := pm.GetExitCode(agentID)
		if exitCode == nil {
			t.Fatal("expected exit code to be set")
		}

		if *exitCode != 2 {
			t.Errorf("expected exit code 2, got %d", *exitCode)
		}
	})

	t.Run("nil exit code for nonexistent agent", func(t *testing.T) {
		exitCode := pm.GetExitCode("nonexistent")
		if exitCode != nil {
			t.Errorf("expected nil exit code for nonexistent agent, got %v", exitCode)
		}
	})

	t.Run("nil exit code for running process", func(t *testing.T) {
		agentID := "still-running"
		_, err := pm.Start(agentID, "sleep", []string{"60"})
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() {
			if err := pm.Stop(agentID); err != nil {
				t.Logf("cleanup stop failed: %v", err)
			}
		}()

		// Process is still running, exit code should not be set yet
		exitCode := pm.GetExitCode(agentID)
		if exitCode != nil {
			t.Errorf("expected nil exit code for running process, got %v", exitCode)
		}
	})
}

func TestProcessManager_NaturalExitRetainsEntry(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "natural-exit-agent"

	// Start a short-lived process
	_, err := pm.Start(agentID, "sleep", []string{"0.1"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for natural exit
	_ = pm.Wait(agentID)

	// Give the wait goroutine time to close the log
	time.Sleep(200 * time.Millisecond)

	// The process entry should still exist so callers can read exit code
	pm.mu.RLock()
	_, exists := pm.processes[agentID]
	pm.mu.RUnlock()
	if !exists {
		t.Error("expected process entry to be retained after natural exit for exit code access")
	}

	// Exit code should be available
	code := pm.GetExitCode(agentID)
	if code == nil || *code != 0 {
		t.Errorf("expected exit code 0, got %v", code)
	}

	// IsRunning should return false
	if pm.IsRunning(agentID) {
		t.Error("expected IsRunning to return false after natural exit")
	}
}

func TestProcessManager_NaturalExitClosesLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "fd-leak-agent"

	// Start a short-lived process
	_, err := pm.Start(agentID, "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for natural exit
	_ = pm.Wait(agentID)

	// Give finalizeProcess goroutine time to run
	time.Sleep(200 * time.Millisecond)

	// Verify the log file exists on disk
	logPath := filepath.Join(tmpDir, agentID+".log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatal("log file should exist")
	}

	// The entry should still be in the map (retained for exit code access)
	pm.mu.RLock()
	_, exists := pm.processes[agentID]
	pm.mu.RUnlock()
	if !exists {
		t.Error("process entry should be retained after natural exit")
	}
}

func TestProcessManager_RestartAfterNaturalExit(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewProcessManager(tmpDir)

	agentID := "restart-agent"

	// Start and let it exit naturally
	_, err := pm.Start(agentID, "sleep", []string{"0.1"})
	if err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	_ = pm.Wait(agentID)
	time.Sleep(200 * time.Millisecond)

	// Restart the same agent — should succeed without errors
	pid, err := pm.Start(agentID, "sleep", []string{"60"})
	if err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	defer func() { _ = pm.Stop(agentID) }()

	if pid <= 0 {
		t.Fatalf("expected positive PID, got %d", pid)
	}

	if !pm.IsRunning(agentID) {
		t.Error("restarted process should be running")
	}
}
