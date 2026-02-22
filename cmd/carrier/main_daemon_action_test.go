package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

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
	if got := daemonActionTimeout("install"); got != 20*time.Minute {
		t.Fatalf("daemonActionTimeout(install) = %s, want %s", got, 20*time.Minute)
	}
	if got := daemonActionTimeout("start"); got != 5*time.Minute {
		t.Fatalf("daemonActionTimeout(start) = %s, want %s", got, 5*time.Minute)
	}
	if got := daemonActionTimeout(" INSTALL "); got != 20*time.Minute {
		t.Fatalf("daemonActionTimeout( INSTALL ) = %s, want %s", got, 20*time.Minute)
	}
}
