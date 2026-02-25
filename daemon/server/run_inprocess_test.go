package server

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func reserveLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port listen: %v", err)
	}
	defer ln.Close()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected TCPAddr, got %T", ln.Addr())
	}
	return tcp.Port
}

func waitHTTPReady(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) // #nosec G107 -- local test probe only
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}

func TestRun_GracefulShutdownInProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal delivery timing in in-process mode is flaky on windows")
	}

	tmp := t.TempDir()
	port := reserveLoopbackPort(t)
	configPath := filepath.Join(tmp, "config.json")
	config := fmt.Sprintf(`{
  "server": { "host": "127.0.0.1", "port": %d },
  "log": { "level": "info", "format": "text" },
  "lifecycle": { "crash_threshold": 3, "crash_window": "5m", "crash_cooldown": "5m" }
}`, port)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_MEMORY_ROOT", filepath.Join(tmp, "memory"))
	t.Setenv("CARRIER_LIFECYCLE_STATE_FILE", filepath.Join(tmp, "lifecycle-state.json"))

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir to temp config dir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	done := make(chan struct{})
	go func() {
		Run()
		close(done)
	}()

	waitHTTPReady(t, fmt.Sprintf("http://127.0.0.1:%d/healthz", port), 10*time.Second)

	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find self process: %v", err)
	}
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatalf("send interrupt signal: %v", err)
	}

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Run() did not return after interrupt")
	}
}
