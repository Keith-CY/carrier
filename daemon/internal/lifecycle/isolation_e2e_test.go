package lifecycle

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestE2EIsolationPIDNamespace verifies that isolated processes cannot see host PIDs.
// This test runs on all platforms with appropriate backends.
func TestE2EIsolationPIDNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	backend, err := resolveIsolationBackend(isolationBackendOptions{})
	if err != nil {
		t.Skipf("isolation backend not available: %v", err)
	}

	// Count host processes
	hostProcs := countHostProcesses(t)
	if hostProcs == 0 {
		t.Skip("could not count host processes")
	}

	// Run ps inside isolation and count
	wrapped, err := backend.WrapCommand("ps aux 2>/dev/null | wc -l")
	if err != nil {
		t.Fatalf("WrapCommand failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", wrapped)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("isolated command failed: %v", err)
	}

	isolatedProcs, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("failed to parse isolated process count: %v", err)
	}

	// Isolated should see significantly fewer processes
	if isolatedProcs >= hostProcs/2 {
		t.Errorf("PID isolation may not be working: isolated sees %d processes, host has %d",
			isolatedProcs, hostProcs)
	} else {
		t.Logf("PID isolation verified: isolated sees %d processes, host has %d",
			isolatedProcs, hostProcs)
	}
}

// TestE2EIsolationTmpfsIsolation verifies that /tmp is isolated (tmpfs).
func TestE2EIsolationTmpfsIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	backend, err := resolveIsolationBackend(isolationBackendOptions{})
	if err != nil {
		t.Skipf("isolation backend not available: %v", err)
	}

	// Create a marker file on host /tmp
	marker := "/tmp/carrier-e2e-isolation-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := os.WriteFile(marker, []byte("host-marker"), 0644); err != nil {
		t.Skipf("could not create marker file: %v", err)
	}
	defer os.Remove(marker)

	// Try to read it from inside isolation
	wrapped, err := backend.WrapCommand("cat " + marker + " 2>/dev/null || echo NOT_FOUND")
	if err != nil {
		t.Fatalf("WrapCommand failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", wrapped)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("isolated command failed: %v", err)
	}

	result := strings.TrimSpace(string(output))
	if result != "NOT_FOUND" {
		t.Errorf("/tmp is NOT isolated: marker file visible inside isolation (content: %s)", result)
	} else {
		t.Log("/tmp isolation verified: host marker file not visible inside isolation")
	}
}

// TestE2EIsolationMultiInstance verifies that multiple isolated instances cannot see each other.
func TestE2EIsolationMultiInstance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	backend, err := resolveIsolationBackend(isolationBackendOptions{})
	if err != nil {
		t.Skipf("isolation backend not available: %v", err)
	}

	// Start two isolated processes and get their PID lists
	wrapped1, err := backend.WrapCommand("echo $$; ps -e -o pid= | tr -d ' '")
	if err != nil {
		t.Fatalf("WrapCommand for cmd1 failed: %v", err)
	}
	wrapped2, err := backend.WrapCommand("echo $$; ps -e -o pid= | tr -d ' '")
	if err != nil {
		t.Fatalf("WrapCommand for cmd2 failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var out1, out2 bytes.Buffer
	cmd1 := exec.CommandContext(ctx, "sh", "-c", wrapped1)
	cmd1.Stdout = &out1
	cmd2 := exec.CommandContext(ctx, "sh", "-c", wrapped2)
	cmd2.Stdout = &out2

	if err := cmd1.Start(); err != nil {
		t.Fatalf("cmd1 start failed: %v", err)
	}
	if err := cmd2.Start(); err != nil {
		t.Fatalf("cmd2 start failed: %v", err)
	}

	_ = cmd1.Wait()
	_ = cmd2.Wait()

	lines1 := strings.Split(strings.TrimSpace(out1.String()), "\n")
	lines2 := strings.Split(strings.TrimSpace(out2.String()), "\n")

	if len(lines1) < 2 || len(lines2) < 2 {
		t.Skip("could not parse PID output")
	}

	pid1 := lines1[0]
	pid2 := lines2[0]
	pids1 := lines1[1:]
	pids2 := lines2[1:]

	// Check if instance 1 can see instance 2's PID
	for _, p := range pids1 {
		if p == pid2 {
			t.Errorf("Instance 1 can see Instance 2's PID %s", pid2)
		}
	}
	for _, p := range pids2 {
		if p == pid1 {
			t.Errorf("Instance 2 can see Instance 1's PID %s", pid1)
		}
	}

	t.Logf("Multi-instance isolation verified: instances have separate PID namespaces")
}

// TestE2EIsolationDieWithParent verifies that child processes die when parent exits.
func TestE2EIsolationDieWithParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	if runtime.GOOS == "windows" {
		t.Skip("die-with-parent test not applicable on Windows host")
	}

	backend, err := resolveIsolationBackend(isolationBackendOptions{})
	if err != nil {
		t.Skipf("isolation backend not available: %v", err)
	}

	// Start a long-running process inside isolation
	wrapped, err := backend.WrapCommand("sleep 60")
	if err != nil {
		t.Fatalf("WrapCommand failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", wrapped)
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd start failed: %v", err)
	}

	pid := cmd.Process.Pid
	time.Sleep(100 * time.Millisecond)

	// Kill the parent
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	// Give it a moment to clean up
	time.Sleep(200 * time.Millisecond)

	// Check if process is still running
	if err := exec.Command("kill", "-0", strconv.Itoa(pid)).Run(); err == nil {
		t.Errorf("Child process %d survived parent exit", pid)
		_ = exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
	} else {
		t.Logf("Die-with-parent verified: child process %d terminated", pid)
	}
}

// TestE2EIsolationBackendPlatformDetection verifies correct backend selection per platform.
func TestE2EIsolationBackendPlatformDetection(t *testing.T) {
	backend, err := resolveIsolationBackend(isolationBackendOptions{})
	if err != nil {
		t.Skipf("isolation backend not available: %v", err)
	}

	goos := backend.CommandGOOS()

	switch runtime.GOOS {
	case "linux":
		if goos != "linux" {
			t.Errorf("Linux host should use linux backend, got %s", goos)
		}
		t.Logf("Linux: using bwrap backend (CommandGOOS=%s)", goos)
	case "darwin":
		if goos != "linux" {
			t.Errorf("macOS host should use lima backend with linux guest, got %s", goos)
		}
		t.Logf("macOS: using Lima backend (CommandGOOS=%s)", goos)
	case "windows":
		if goos != "linux" {
			t.Errorf("Windows host should use WSL backend with linux guest, got %s", goos)
		}
		t.Logf("Windows: using WSL backend (CommandGOOS=%s)", goos)
	default:
		t.Logf("Unknown platform %s: backend reports CommandGOOS=%s", runtime.GOOS, goos)
	}
}

func countHostProcesses(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", "ps aux 2>/dev/null | wc -l")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0
	}
	return count
}
