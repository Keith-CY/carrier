package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strconv"
	"strings"
	"testing"
)

func TestParseCarrierCommandRoutesRemote(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "remote", "sync", "host-1", "main"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(remote) error: %v", err)
	}
	if cmd != "remote" {
		t.Fatalf("command = %q, want remote", cmd)
	}
	if len(args) != 3 || args[0] != "sync" || args[1] != "host-1" || args[2] != "main" {
		t.Fatalf("args = %v, want [sync host-1 main]", args)
	}
}

func TestParseRemoteCommandArgsDefaultsAndValidation(t *testing.T) {
	opts, err := parseRemoteCommandArgs([]string{"sync", "host-1", "main", "--mode", "pull_validate_push"})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(sync) error: %v", err)
	}
	if opts.Action != "sync" {
		t.Fatalf("action = %q, want sync", opts.Action)
	}
	if opts.Mode != "pull_validate_push" {
		t.Fatalf("mode = %q, want pull_validate_push", opts.Mode)
	}

	runOpts, err := parseRemoteCommandArgs([]string{"codeagent-run", "host-1", "main", "--command", "echo hello"})
	if err != nil {
		t.Fatalf("parseRemoteCommandArgs(codeagent-run) error: %v", err)
	}
	if runOpts.Capability != "run_shell" {
		t.Fatalf("capability = %q, want run_shell", runOpts.Capability)
	}

	if _, err := parseRemoteCommandArgs([]string{"codeagent-run", "host-1", "main", "--backend", "bad-backend", "--command", "echo hello"}); err == nil {
		t.Fatal("expected backend validation error")
	}
}

func TestRunRemoteInstanceCommandSync(t *testing.T) {
	var seenPath string
	var seenMode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/remote/hosts/host-1/instances/main/sync":
			seenPath = r.URL.Path
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			seenMode = strings.TrimSpace(anyToStringForTest(body["mode"]))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok","sync":{"driftState":"in_sync"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureGatewayProbeEnvForTest(t, server.URL)

	var out bytes.Buffer
	err := runRemoteInstanceCommand(&out, remoteCommandOptions{
		Action:  "sync",
		HostID:  "host-1",
		AgentID: "main",
		Mode:    "pull_validate_push",
	})
	if err != nil {
		t.Fatalf("runRemoteInstanceCommand(sync) error: %v", err)
	}
	if seenPath != "/api/v1/remote/hosts/host-1/instances/main/sync" {
		t.Fatalf("seenPath = %q", seenPath)
	}
	if seenMode != "pull_validate_push" {
		t.Fatalf("seenMode = %q, want pull_validate_push", seenMode)
	}
	if !strings.Contains(out.String(), `"driftState": "in_sync"`) {
		t.Fatalf("output missing sync payload: %s", out.String())
	}
}

func TestRunRemoteInstanceCommandCodeAgentRunIncludesGatewayAuth(t *testing.T) {
	var seenAuth string
	var seenPath string
	var seenCapability string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/remote/hosts/host-1/instances/main/codeagent/run":
			seenAuth = r.Header.Get("Authorization")
			seenPath = r.URL.Path
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			seenCapability = strings.TrimSpace(anyToStringForTest(body["capability"]))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"ok","run":{"backend":"codex","result":{"ok":true,"exit_code":0}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	configureGatewayProbeEnvForTest(t, server.URL)
	t.Setenv("CARRIER_GATEWAY_API_TOKEN", "gw-test-token")

	var out bytes.Buffer
	err := runRemoteInstanceCommand(&out, remoteCommandOptions{
		Action:        "codeagent-run",
		HostID:        "host-1",
		AgentID:       "main",
		Backend:       "codex",
		WorkspaceRoot: "/workspace",
		Capability:    "run_shell",
		Command:       "echo ok",
	})
	if err != nil {
		t.Fatalf("runRemoteInstanceCommand(codeagent-run) error: %v", err)
	}
	if seenPath != "/api/v1/remote/hosts/host-1/instances/main/codeagent/run" {
		t.Fatalf("seenPath = %q", seenPath)
	}
	if seenAuth != "Bearer gw-test-token" {
		t.Fatalf("seenAuth = %q, want Bearer gw-test-token", seenAuth)
	}
	if seenCapability != "run_shell" {
		t.Fatalf("seenCapability = %q, want run_shell", seenCapability)
	}
	if !strings.Contains(out.String(), `"ok": true`) {
		t.Fatalf("output missing run result: %s", out.String())
	}
}

func configureGatewayProbeEnvForTest(t *testing.T, serverURL string) {
	t.Helper()
	parsed, err := neturl.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	t.Setenv("CARRIER_GATEWAY_HOST", host)
	t.Setenv("CARRIER_GATEWAY_PORT", port)
}

func anyToStringForTest(v interface{}) string {
	switch typed := v.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
