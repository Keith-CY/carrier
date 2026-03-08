package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseCarrierCommandRoutesOrchestrate(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "orchestrate", "summarize", "logs"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(orchestrate) error: %v", err)
	}
	if cmd != "orchestrate" {
		t.Fatalf("command = %q, want orchestrate", cmd)
	}
	if len(args) != 2 || args[0] != "summarize" || args[1] != "logs" {
		t.Fatalf("args = %v, want [summarize logs]", args)
	}
}

func TestParseOrchestrateCommandArgs(t *testing.T) {
	opts, err := parseOrchestrateCommandArgs([]string{
		"triage", "incidents",
		"--host-id", "host-a",
		"--host-id", "host-a",
		"--host-id", "host-b",
		"--provider", "openrouter",
		"--max-concurrency", "9",
		"--idempotency-key", "idem-1",
		"--timeout", "45s",
		"--async",
		"--json",
	})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(run) error: %v", err)
	}
	if opts.Action != "run" {
		t.Fatalf("action = %q, want run", opts.Action)
	}
	if opts.Goal != "triage incidents" {
		t.Fatalf("goal = %q, want %q", opts.Goal, "triage incidents")
	}
	if len(opts.HostIDs) != 2 || opts.HostIDs[0] != "host-a" || opts.HostIDs[1] != "host-b" {
		t.Fatalf("host_ids = %v, want [host-a host-b]", opts.HostIDs)
	}
	if opts.Provider != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", opts.Provider)
	}
	if opts.MaxConcurrency != 9 {
		t.Fatalf("max_concurrency = %d, want 9", opts.MaxConcurrency)
	}
	if opts.IdempotencyKey != "idem-1" {
		t.Fatalf("idempotency_key = %q, want idem-1", opts.IdempotencyKey)
	}
	if opts.Timeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", opts.Timeout)
	}
	if !opts.Async || !opts.JSON {
		t.Fatalf("async/json flags should be true, got async=%v json=%v", opts.Async, opts.JSON)
	}

	statusOpts, err := parseOrchestrateCommandArgs([]string{"status", "exec-123", "--json"})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(status) error: %v", err)
	}
	if statusOpts.Action != "status" || statusOpts.ExecutionID != "exec-123" || !statusOpts.JSON {
		t.Fatalf("unexpected status opts: %+v", statusOpts)
	}
}

func TestRunOrchestrateCommandLocalFallbackAndAgentDefault(t *testing.T) {
	var createBody string
	var authorizeBody string

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/base-agent/decompose":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"tasks":[{"id":"t1","input":"collect diagnostics","agentId":""},{"id":"t2","input":"summarize diagnostics","agentId":"picoclaw"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/orchestrator/executions":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			raw, _ := io.ReadAll(r.Body)
			createBody = strings.TrimSpace(string(raw))
			_, _ = w.Write([]byte(`{"result":"ok","execution":{"id":"exec-1","goal":"triage issue","status":"pending_authorization"}}`))
		case "/api/v1/orchestrator/executions/exec-1/authorize":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			raw, _ := io.ReadAll(r.Body)
			authorizeBody = strings.TrimSpace(string(raw))
			_, _ = w.Write([]byte(`{"result":"ok","execution":{"id":"exec-1","goal":"triage issue","status":"provisioning"}}`))
		case "/api/v1/orchestrator/executions/exec-1":
			if r.Method != http.MethodGet {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"result":"ok","execution":{"id":"exec-1","goal":"triage issue","status":"completed","taskUnits":[{"id":"t1","hostId":"local","agentId":"zeroclaw"},{"id":"t2","hostId":"local","agentId":"picoclaw"}],"results":[{"taskId":"t1","status":"completed","hostId":"local","agentId":"zeroclaw","output":"diag ok","latencyMs":12},{"taskId":"t2","status":"completed","hostId":"local","agentId":"picoclaw","output":"summary ok","latencyMs":7}]},"workers":[{"id":"w1","hostId":"local","agentId":"zeroclaw","state":"reclaimed"},{"id":"w2","hostId":"local","agentId":"picoclaw","state":"reclaimed"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_SERVER_HOST", "CARRIER_SERVER_PORT", daemon.URL)
	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	opts := orchestrateCommandOptions{
		Action:  "run",
		Goal:    "triage issue",
		Timeout: 2 * time.Second,
	}
	var out bytes.Buffer
	if err := runOrchestrateCommand(&out, opts); err != nil {
		t.Fatalf("runOrchestrateCommand error: %v", err)
	}

	if strings.TrimSpace(createBody) == "" {
		t.Fatal("expected create request body")
	}
	var createPayload struct {
		RequiredWorkers []struct {
			HostID  string `json:"hostId"`
			AgentID string `json:"agentId"`
		} `json:"requiredWorkers"`
		TaskUnits []struct {
			ID      string `json:"id"`
			HostID  string `json:"hostId"`
			AgentID string `json:"agentId"`
		} `json:"taskUnits"`
	}
	if err := json.Unmarshal([]byte(createBody), &createPayload); err != nil {
		t.Fatalf("decode create body: %v", err)
	}
	if len(createPayload.TaskUnits) != 2 {
		t.Fatalf("task units = %d, want 2", len(createPayload.TaskUnits))
	}
	if createPayload.TaskUnits[0].HostID != "local" || createPayload.TaskUnits[0].AgentID != "zeroclaw" {
		t.Fatalf("task1 target = %+v, want host=local agent=zeroclaw", createPayload.TaskUnits[0])
	}
	if createPayload.TaskUnits[1].HostID != "local" || createPayload.TaskUnits[1].AgentID != "picoclaw" {
		t.Fatalf("task2 target = %+v, want host=local agent=picoclaw", createPayload.TaskUnits[1])
	}
	if len(createPayload.RequiredWorkers) != 2 {
		t.Fatalf("required workers = %d, want 2", len(createPayload.RequiredWorkers))
	}
	if !strings.Contains(authorizeBody, `"approved":true`) {
		t.Fatalf("authorize body = %q, want approved=true", authorizeBody)
	}

	output := out.String()
	if !strings.Contains(output, "status: completed") {
		t.Fatalf("expected completed status in output, got %q", output)
	}
	if !strings.Contains(output, "local/zeroclaw") {
		t.Fatalf("expected local/zeroclaw in output, got %q", output)
	}
	if !strings.Contains(output, "local/picoclaw") {
		t.Fatalf("expected local/picoclaw in output, got %q", output)
	}
}
