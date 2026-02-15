package lifecycle

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

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
	stopOnce sync.Once
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
		if pm.isProcessAlive(info.pid) {
			return 0, fmt.Errorf("agent %s already running with PID %d", agentID, info.pid)
		}
		// Clean up stale entry
		delete(pm.processes, agentID)
	}

	// Ensure log directory exists
	if err := os.MkdirAll(pm.logDir, 0o755); err != nil {
		return 0, fmt.Errorf("create log dir: %w", err)
	}

	// Open log file
	logPath := filepath.Join(pm.logDir, fmt.Sprintf("%s.log", agentID))
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
		close(info.done)
	}()

	pm.processes[agentID] = info
	return info.pid, nil
}

// Stop sends SIGTERM to the agent's process, waits up to 10 seconds,
// then sends SIGKILL if still running.
func (pm *ProcessManager) Stop(agentID string) error {
	pm.mu.Lock()
	info, exists := pm.processes[agentID]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("agent %s is not running", agentID)
	}
	pm.mu.Unlock()

	var stopErr error
	info.stopOnce.Do(func() {
		// Send SIGTERM to the full process group so child processes are terminated too.
		if err := syscall.Kill(-info.pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			stopErr = fmt.Errorf("send SIGTERM to process group: %w", err)
			return
		}

		// Wait up to 10 seconds for graceful shutdown
		timeout := time.After(10 * time.Second)
		select {
		case <-info.done:
			// Process exited gracefully
		case <-timeout:
			// Force kill the full process group.
			if err := syscall.Kill(-info.pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
				stopErr = fmt.Errorf("send SIGKILL to process group: %w", err)
				return
			}
			<-info.done // Wait for process to actually exit
		}

		// Cleanup
		info.logFile.Close()
	})

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

	return pm.isProcessAlive(info.pid)
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

// isProcessAlive checks if a process with the given PID exists.
// Must be called with lock held.
func (pm *ProcessManager) isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
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
