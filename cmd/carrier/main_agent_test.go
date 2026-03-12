package main

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestParseCarrierCommandRoutesAgent(t *testing.T) {
    cmd, args, err := parseCarrierCommand([]string{"carrier", "agent", "launcher", "picoclaw"})
    if err != nil {
        t.Fatalf("parseCarrierCommand(agent) error: %v", err)
    }
    if cmd != "agent" {
        t.Fatalf("command=%q want agent", cmd)
    }
    if len(args) != 2 || args[0] != "launcher" || args[1] != "picoclaw" {
        t.Fatalf("args=%v want [launcher picoclaw]", args)
    }
}

func TestParseAgentCommandArgs(t *testing.T) {
    runOpts, err := parseAgentCommandArgs([]string{"run", "picoclaw", "-m", "hello", "--provider", "openrouter", "--session-id", "sess-1", "--json"})
    if err != nil {
        t.Fatalf("parseAgentCommandArgs(run) error: %v", err)
    }
    if runOpts.Action != "run" || runOpts.AgentID != "picoclaw" || runOpts.Message != "hello" || runOpts.Provider != "openrouter" || runOpts.SessionID != "sess-1" || !runOpts.JSON {
        t.Fatalf("unexpected run opts: %+v", runOpts)
    }

    shellOpts, err := parseAgentCommandArgs([]string{"shell", "picoclaw", "--provider", "openrouter", "--session-id", "sess-2"})
    if err != nil {
        t.Fatalf("parseAgentCommandArgs(shell) error: %v", err)
    }
    if shellOpts.Action != "shell" || shellOpts.AgentID != "picoclaw" || shellOpts.Provider != "openrouter" || shellOpts.SessionID != "sess-2" {
        t.Fatalf("unexpected shell opts: %+v", shellOpts)
    }

    launcherOpts, err := parseAgentCommandArgs([]string{"launcher", "picoclaw", "--json"})
    if err != nil {
        t.Fatalf("parseAgentCommandArgs(launcher) error: %v", err)
    }
    if launcherOpts.Action != "launcher" || launcherOpts.AgentID != "picoclaw" || !launcherOpts.JSON {
        t.Fatalf("unexpected launcher opts: %+v", launcherOpts)
    }

    heartbeatOpts, err := parseAgentCommandArgs([]string{"heartbeat", "picoclaw", "--json"})
    if err != nil {
        t.Fatalf("parseAgentCommandArgs(heartbeat) error: %v", err)
    }
    if heartbeatOpts.Action != "heartbeat" || heartbeatOpts.AgentID != "picoclaw" || !heartbeatOpts.JSON {
        t.Fatalf("unexpected heartbeat opts: %+v", heartbeatOpts)
    }
}

func TestRunAgentCommand(t *testing.T) {
    gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/healthz":
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`{"status":"ok"}`))
        case "/api/v1/agents/picoclaw/chat":
            if r.Method != http.MethodPost {
                http.NotFound(w, r)
                return
            }
            var body map[string]any
            _ = json.NewDecoder(r.Body).Decode(&body)
            if body["message"] != "hello launcher" {
                t.Fatalf("message=%v want hello launcher", body["message"])
            }
            _, _ = w.Write([]byte(`{"agentId":"picoclaw","sessionId":"sess-run","message":"pong"}`))
        case "/api/v1/agents/picoclaw/launcher":
            if r.Method != http.MethodGet {
                http.NotFound(w, r)
                return
            }
            _, _ = w.Write([]byte(`{"result":"ok","agentId":"picoclaw","status":{"runtimeState":"running","installState":"installed","health":"healthy"},"heartbeat":{"state":"fresh","ageSeconds":4,"lastActivityAt":"2026-03-12T00:00:00Z"},"memory":{"contractId":"memory-alpha","contractDigest":"digest-1","syncState":"synced"},"providerReadiness":{"provider":"openrouter","ready":true,"authMode":"api_key"},"session":{"instanceId":"picoclaw-main","runtimeMode":"managed_gateway","updatedAt":"2026-03-12T00:00:00Z"}}`))
        default:
            http.NotFound(w, r)
        }
    }))
    defer gateway.Close()

    setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

    var out bytes.Buffer
    if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "run", AgentID: "picoclaw", Message: "hello launcher"}); err != nil {
        t.Fatalf("runAgentCommand(run) error: %v", err)
    }
    if !strings.Contains(out.String(), "pong") || !strings.Contains(out.String(), "sess-run") {
        t.Fatalf("run output=%s", out.String())
    }

    out.Reset()
    if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "launcher", AgentID: "picoclaw"}); err != nil {
        t.Fatalf("runAgentCommand(launcher) error: %v", err)
    }
    if !strings.Contains(out.String(), "fresh") || !strings.Contains(out.String(), "memory-alpha") || !strings.Contains(out.String(), "openrouter") {
        t.Fatalf("launcher output=%s", out.String())
    }

    out.Reset()
    if err := runAgentCommand(strings.NewReader(""), &out, agentCommandOptions{Action: "heartbeat", AgentID: "picoclaw"}); err != nil {
        t.Fatalf("runAgentCommand(heartbeat) error: %v", err)
    }
    if !strings.Contains(out.String(), "fresh") || !strings.Contains(out.String(), "4s") {
        t.Fatalf("heartbeat output=%s", out.String())
    }
}

func TestRunAgentShellCommand(t *testing.T) {
    gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/healthz":
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte(`{"status":"ok"}`))
        case "/api/v1/agents/picoclaw/chat":
            _, _ = io.WriteString(w, `{"agentId":"picoclaw","sessionId":"sess-shell","message":"ack"}`)
        default:
            http.NotFound(w, r)
        }
    }))
    defer gateway.Close()

    setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

    var out bytes.Buffer
    in := strings.NewReader("hello\n/exit\n")
    if err := runAgentCommand(in, &out, agentCommandOptions{Action: "shell", AgentID: "picoclaw"}); err != nil {
        t.Fatalf("runAgentCommand(shell) error: %v", err)
    }
    if !strings.Contains(out.String(), "Interactive shell") || !strings.Contains(out.String(), "ack") {
        t.Fatalf("shell output=%s", out.String())
    }
}
