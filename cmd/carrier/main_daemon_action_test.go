package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "request timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestDaemonAgentActionInstallRecoversEOFWhenStatusInstalled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/openclaw/install":
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack connection: %v", err)
			}
			_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 25\r\n\r\n{\"status\":\"installed\""))
			_ = conn.Close()
			return
		case "/api/v1/agents/openclaw/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"openclaw","installState":"installed","runtimeState":"stopped"}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	if err := daemonAgentAction("openclaw", "install"); err != nil {
		t.Fatalf("daemonAgentAction install should reconcile EOF using daemon status, got %v", err)
	}
}

func TestDaemonAgentActionInstallKeepsEOFFailureWhenStatusMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/openclaw/install":
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack connection: %v", err)
			}
			_ = conn.Close()
			return
		case "/api/v1/agents/openclaw/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"openclaw","installState":"not_installed","runtimeState":"stopped"}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	err := daemonAgentAction("openclaw", "install")
	if err == nil {
		t.Fatal("daemonAgentAction install should fail when status does not confirm install")
	}
	if !isDaemonEOFError(err) {
		t.Fatalf("daemonAgentAction install error = %v, want EOF-like error", err)
	}
}

func TestDaemonAgentActionInstallReportsDaemonLastErrorOnEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/openclaw/install":
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack connection: %v", err)
			}
			_ = conn.Close()
			return
		case "/api/v1/agents/openclaw/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"openclaw","installState":"broken","runtimeState":"stopped","lastError":"signal: killed"}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	err := daemonAgentAction("openclaw", "install")
	if err == nil {
		t.Fatal("daemonAgentAction install should fail when daemon status is broken")
	}
	if !strings.Contains(err.Error(), "install state=broken: signal: killed") {
		t.Fatalf("daemonAgentAction install error = %q, want daemon lastError details", err.Error())
	}
	if !strings.Contains(err.Error(), "request error:") {
		t.Fatalf("daemonAgentAction install error = %q, want request error context", err.Error())
	}
}

func TestDaemonAgentActionStartReportsDaemonLastErrorOnEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/openclaw/start":
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack connection: %v", err)
			}
			_ = conn.Close()
			return
		case "/api/v1/agents/openclaw/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"statuses":[{"id":"openclaw","installState":"installed","runtimeState":"error","lastError":"failed to bind port"}]}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	err := daemonAgentAction("openclaw", "start")
	if err == nil {
		t.Fatal("daemonAgentAction start should fail when daemon runtime state is error")
	}
	if !strings.Contains(err.Error(), "runtime state=error: failed to bind port") {
		t.Fatalf("daemonAgentAction start error = %q, want daemon lastError details", err.Error())
	}
	if !strings.Contains(err.Error(), "request error:") {
		t.Fatalf("daemonAgentAction start error = %q, want request error context", err.Error())
	}
}

func TestReconcileDaemonActionOnTransportErrorRecoversTimeoutWhenStatusInstalled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/openclaw/status":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"openclaw","installState":"installed","runtimeState":"stopped"}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	reconciled, err := reconcileDaemonActionOnTransportError("openclaw", "install", timeoutNetError{})
	if err != nil {
		t.Fatalf("reconcileDaemonActionOnTransportError returned error: %v", err)
	}
	if !reconciled {
		t.Fatal("reconcileDaemonActionOnTransportError should recover timeout when daemon status is installed")
	}
}

func configureDaemonProbeEnvForTest(t *testing.T, serverURL string) {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	t.Setenv("CARRIER_SERVER_HOST", host)
	t.Setenv("CARRIER_SERVER_PORT", port)
}

func TestDaemonActionTimeoutUsesExtendedInstallWindow(t *testing.T) {
	setCommandTimeout := func(t *testing.T, value string) {
		t.Helper()
		prev, existed := os.LookupEnv("CARRIER_COMMAND_TIMEOUT")
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv("CARRIER_COMMAND_TIMEOUT", prev)
				return
			}
			_ = os.Unsetenv("CARRIER_COMMAND_TIMEOUT")
		})
		t.Setenv("CARRIER_COMMAND_TIMEOUT", value)
	}

	t.Run("default timeout for install", func(t *testing.T) {
		setCommandTimeout(t, "")
		if got := daemonActionTimeout("install"); got != 30*time.Minute {
			t.Fatalf("daemonActionTimeout(install) = %s, want %s", got, 30*time.Minute)
		}
	})

	t.Run("large custom timeout for install", func(t *testing.T) {
		setCommandTimeout(t, "45m")
		if got := daemonActionTimeout("install"); got != 47*time.Minute {
			t.Fatalf("daemonActionTimeout(install, command_timeout=45m) = %s, want %s", got, 47*time.Minute)
		}
	})

	t.Run("non-install action ignores custom timeout", func(t *testing.T) {
		setCommandTimeout(t, "1s")
		if got := daemonActionTimeout("start"); got != 5*time.Minute {
			t.Fatalf("daemonActionTimeout(start, command_timeout=1s) = %s, want %s", got, 5*time.Minute)
		}
	})

	t.Run("small custom timeout for install floor", func(t *testing.T) {
		setCommandTimeout(t, "10m")
		if got := daemonActionTimeout(" INSTALL "); got != 30*time.Minute {
			t.Fatalf("daemonActionTimeout( INSTALL , command_timeout=10m) = %s, want %s", got, 30*time.Minute)
		}
	})
}

func TestDaemonAgentActionWithProgressPrintsNewLogLines(t *testing.T) {
	var installDone atomic.Bool
	var logsCalls atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/openclaw/install":
			time.Sleep(25 * time.Millisecond)
			installDone.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/v1/agents/openclaw/logs":
			logsCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if installDone.Load() {
				_, _ = w.Write([]byte(`{"lines":["old-line","install-progress-line"]}`))
				return
			}
			_, _ = w.Write([]byte(`{"lines":["old-line"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	prevPoll := daemonActionLogPollInterval
	prevHeartbeat := daemonActionHeartbeatInterval
	daemonActionLogPollInterval = 5 * time.Millisecond
	daemonActionHeartbeatInterval = time.Hour
	t.Cleanup(func() {
		daemonActionLogPollInterval = prevPoll
		daemonActionHeartbeatInterval = prevHeartbeat
	})

	var out bytes.Buffer
	if err := daemonAgentActionWithProgress(&out, "openclaw", "install", false); err != nil {
		t.Fatalf("daemonAgentActionWithProgress install error: %v", err)
	}

	output := out.String()
	if strings.Contains(output, "old-line") {
		t.Fatalf("output should not replay baseline log lines, got %q", output)
	}
	if !strings.Contains(output, "install-progress-line") {
		t.Fatalf("output should include new log lines, got %q", output)
	}
	if logsCalls.Load() == 0 {
		t.Fatal("expected logs endpoint to be queried for progress output")
	}
}

func TestDaemonAgentActionWithProgressQuietModeSuppressesLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/openclaw/install":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/v1/agents/openclaw/logs":
			t.Fatalf("logs endpoint should not be called in quiet mode")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureDaemonProbeEnvForTest(t, server.URL)

	var out bytes.Buffer
	if err := daemonAgentActionWithProgress(&out, "openclaw", "install", true); err != nil {
		t.Fatalf("daemonAgentActionWithProgress install error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("quiet mode should not print progress logs, got %q", out.String())
	}
}
