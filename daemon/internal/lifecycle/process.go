package lifecycle

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// defaultGracePeriod is the default time to wait after SIGTERM before sending SIGKILL.
const defaultGracePeriod = 10 * time.Second

// ProcessManager tracks and manages running agent processes.
type ProcessManager struct {
	mu        sync.RWMutex
	processes map[string]*processInfo
	logDir    string
}

type processInfo struct {
	cmd      *exec.Cmd
	pid      int
	agentID  string
	logFile  *os.File
	done     chan struct{}
	waitErr  error
	exitCode *int
	stopping bool
}

// NewProcessManager creates a new process manager.
// logDir specifies where to write per-agent log files.
func NewProcessManager(logDir string) *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*processInfo),
		logDir:    logDir,
	}
}

// Start spawns a new process for the given agent and returns its PID.
// stdout and stderr are captured to {logDir}/{agentID}.log
func (pm *ProcessManager) Start(agentID string, command string, args []string) (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already running
	if info, exists := pm.processes[agentID]; exists {
		if pm.isProcessAlive(info) {
			return 0, fmt.Errorf("agent %s already running with PID %d", agentID, info.pid)
		}
		// Clean up stale entry
		delete(pm.processes, agentID)
	}

	// Ensure log directory exists
	if err := os.MkdirAll(pm.logDir, 0o755); err != nil {
		return 0, fmt.Errorf("create log dir: %w", err)
	}

	// Open log file (rotate first if oversized)
	logPath := filepath.Join(pm.logDir, fmt.Sprintf("%s.log", agentID))
	if err := rotateLogFile(logPath, maxLogSize); err != nil {
		return 0, fmt.Errorf("rotate log file: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}

	// Create command
	cmd := exec.Command(command, args...)
	cmd.Stdout = io.MultiWriter(logFile, os.Stdout)
	cmd.Stderr = io.MultiWriter(logFile, os.Stderr)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group for clean signal handling
	}

	// Start process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("start process: %w", err)
	}

	// Track process
	info := &processInfo{
		cmd:     cmd,
		pid:     cmd.Process.Pid,
		agentID: agentID,
		logFile: logFile,
		done:    make(chan struct{}),
	}

	// Monitor process in background
	go func() {
		info.waitErr = cmd.Wait()
		// Extract exit code if available
		if exitErr, ok := info.waitErr.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			info.exitCode = &code
		} else if info.waitErr == nil {
			// Process exited successfully (exit code 0)
			code := 0
			info.exitCode = &code
		}
		close(info.done)
	}()

	pm.processes[agentID] = info
	return info.pid, nil
}

// Stop sends SIGTERM to the agent's process, waits up to 10 seconds,
// then sends SIGKILL if still running.
func (pm *ProcessManager) Stop(agentID string) error {
	return pm.StopWithContext(context.Background(), agentID)
}

// StopWithContext sends SIGTERM to the agent's process. It waits until the
// context expires (or defaultGracePeriod if the context has no deadline)
// before escalating to SIGKILL.
func (pm *ProcessManager) StopWithContext(ctx context.Context, agentID string) error {
	pm.mu.Lock()
	info, exists := pm.processes[agentID]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("agent %s is not running", agentID)
	}
	if info.stopping {
		pm.mu.Unlock()
		return fmt.Errorf("agent %s is already stopping", agentID)
	}
	info.stopping = true
	pm.mu.Unlock()

	// Send SIGTERM to the full process group so child processes are terminated too.
	var stopErr error
	if err := syscall.Kill(-info.pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		stopErr = fmt.Errorf("send SIGTERM to process group: %w", err)
	} else {
		// Wait up to 10 seconds for graceful shutdown
		timeout := time.After(10 * time.Second)
		select {
		case <-info.done:
			// Process exited gracefully
		case <-timeout:
			// Force kill the full process group.
			if err := syscall.Kill(-info.pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
				stopErr = fmt.Errorf("send SIGKILL to process group: %w", err)
			} else {
				<-info.done // Wait for process to actually exit
			}
		}
	}

	// Cleanup
	info.logFile.Close()

	pm.mu.Lock()
	delete(pm.processes, agentID)
	pm.mu.Unlock()

	return stopErr
}

// IsRunning checks if the agent's process is currently running.
func (pm *ProcessManager) IsRunning(agentID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	info, exists := pm.processes[agentID]
	if !exists {
		return false
	}

	return pm.isProcessAlive(info)
}

// Wait blocks until the agent's process exits and returns the exit error.
func (pm *ProcessManager) Wait(agentID string) error {
	pm.mu.RLock()
	info, exists := pm.processes[agentID]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s is not running", agentID)
	}

	<-info.done
	return info.waitErr
}

// isProcessAlive checks if the tracked process is still running.
// Must be called with lock held.
// Uses the stored *os.Process handle to avoid PID reuse issues.
func (pm *ProcessManager) isProcessAlive(info *processInfo) bool {
	// First check if the process has already exited (non-blocking)
	select {
	case <-info.done:
		return false
	default:
	}

	// Verify using signal 0 on the original Process handle (not a PID lookup)
	// This guards against PID reuse because we're checking the specific
	// Process object we created, not looking up a potentially recycled PID.
	if info.cmd == nil || info.cmd.Process == nil {
		return false
	}

	err := info.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// GetExitCode returns the exit code of a terminated process, or nil if not available.
func (pm *ProcessManager) GetExitCode(agentID string) *int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	info, exists := pm.processes[agentID]
	if !exists {
		return nil
	}

	// Only read exitCode after the monitoring goroutine has finished writing it.
	select {
	case <-info.done:
		return info.exitCode
	default:
		return nil
	}
}

// maxLogSize is the size threshold (10 MB) at which a log file is rotated
// before a new process start.
const maxLogSize int64 = 10 * 1024 * 1024

// rotateLogFile checks if the log file at path exceeds maxBytes and, if so,
// renames it to path + ".1" (overwriting any previous backup) so a fresh
// file can be created by the caller.
func rotateLogFile(path string, maxBytes int64) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to rotate
		}
		return err
	}
	if fi.Size() < maxBytes {
		return nil
	}
	return os.Rename(path, path+".1")
}

// Cleanup stops all running processes (for graceful shutdown).
func (pm *ProcessManager) Cleanup() {
	pm.mu.Lock()
	agentIDs := make([]string, 0, len(pm.processes))
	for id := range pm.processes {
		agentIDs = append(agentIDs, id)
	}
	pm.mu.Unlock()

	for _, id := range agentIDs {
		if err := pm.Stop(id); err != nil {
			// Best-effort cleanup during shutdown; continue stopping other processes.
			continue
		}
	}
}
