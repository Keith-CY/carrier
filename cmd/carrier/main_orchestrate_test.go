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

func TestParseCarrierCommandRoutesExecutions(t *testing.T) {
	cmd, args, err := parseCarrierCommand([]string{"carrier", "executions", "show", "exec-123"})
	if err != nil {
		t.Fatalf("parseCarrierCommand(executions) error: %v", err)
	}
	if cmd != "executions" {
		t.Fatalf("command = %q, want executions", cmd)
	}
	if len(args) != 2 || args[0] != "show" || args[1] != "exec-123" {
		t.Fatalf("args = %v, want [show exec-123]", args)
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

	planOpts, err := parseOrchestrateCommandArgs([]string{"triage", "incidents", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(plan) error: %v", err)
	}
	if planOpts.Action != "plan" || planOpts.Goal != "triage incidents" || !planOpts.JSON {
		t.Fatalf("unexpected plan opts: %+v", planOpts)
	}

	statusOpts, err := parseOrchestrateCommandArgs([]string{"status", "exec-123", "--json"})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(status) error: %v", err)
	}
	if statusOpts.Action != "status" || statusOpts.ExecutionID != "exec-123" || !statusOpts.JSON {
		t.Fatalf("unexpected status opts: %+v", statusOpts)
	}

	cancelOpts, err := parseOrchestrateCommandArgs([]string{"cancel", "exec-456", "--json"})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(cancel) error: %v", err)
	}
	if cancelOpts.Action != "cancel" || cancelOpts.ExecutionID != "exec-456" || !cancelOpts.JSON {
		t.Fatalf("unexpected cancel opts: %+v", cancelOpts)
	}
}

func TestParseExecutionsCommandArgs(t *testing.T) {
	listOpts, err := parseExecutionsCommandArgs([]string{"list", "--limit", "5", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(list) error: %v", err)
	}
	if listOpts.Action != "list" || listOpts.Limit != 5 || !listOpts.JSON {
		t.Fatalf("unexpected list opts: %+v", listOpts)
	}

	showOpts, err := parseExecutionsCommandArgs([]string{"show", "exec-42"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(show) error: %v", err)
	}
	if showOpts.Action != "status" || showOpts.ExecutionID != "exec-42" {
		t.Fatalf("unexpected show opts: %+v", showOpts)
	}

	implicitShowOpts, err := parseExecutionsCommandArgs([]string{"exec-99", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(implicit show) error: %v", err)
	}
	if implicitShowOpts.Action != "status" || implicitShowOpts.ExecutionID != "exec-99" || !implicitShowOpts.JSON {
		t.Fatalf("unexpected implicit show opts: %+v", implicitShowOpts)
	}

	cancelOpts, err := parseExecutionsCommandArgs([]string{"cancel", "exec-55", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(cancel) error: %v", err)
	}
	if cancelOpts.Action != "cancel" || cancelOpts.ExecutionID != "exec-55" || !cancelOpts.JSON {
		t.Fatalf("unexpected cancel opts: %+v", cancelOpts)
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
		Action:   "run",
		Goal:     "triage issue",
		Provider: "openrouter",
		Timeout:  2 * time.Second,
	}
	var out bytes.Buffer
	if err := runOrchestrateCommand(&out, opts); err != nil {
		t.Fatalf("runOrchestrateCommand error: %v", err)
	}

	if strings.TrimSpace(createBody) == "" {
		t.Fatal("expected create request body")
	}
	var createPayload struct {
		RequestedProvider string `json:"requestedProvider"`
		RequiredWorkers   []struct {
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
	if createPayload.RequestedProvider != "openrouter" {
		t.Fatalf("requestedProvider = %q, want openrouter", createPayload.RequestedProvider)
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

func TestRunOrchestrateCommandDryRunPlan(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/base-agent/decompose":
			_, _ = w.Write([]byte(`{"tasks":[{"id":"t1","input":"collect diagnostics","agentId":"zeroclaw"},{"id":"t2","input":"summarize diagnostics","agentId":"picoclaw"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer daemon.Close()

	setProbeEnvFromURL(t, "CARRIER_SERVER_HOST", "CARRIER_SERVER_PORT", daemon.URL)

	opts := orchestrateCommandOptions{
		Action:  "plan",
		Goal:    "triage issue",
		Timeout: 2 * time.Second,
	}
	var out bytes.Buffer
	if err := runOrchestrateCommand(&out, opts); err != nil {
		t.Fatalf("runOrchestrateCommand(plan) error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "orchestration plan") {
		t.Fatalf("expected plan header in output, got %q", output)
	}
	if !strings.Contains(output, "local/zeroclaw") {
		t.Fatalf("expected local/zeroclaw in plan output, got %q", output)
	}
	if !strings.Contains(output, "local/picoclaw") {
		t.Fatalf("expected local/picoclaw in plan output, got %q", output)
	}
}

func TestRunOrchestrateListCommand(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/orchestrator/executions":
			_, _ = w.Write([]byte(`{"result":"ok","executions":[{"id":"exec-old","goal":"older goal","status":"completed","updatedAt":"2026-03-08T04:00:00Z","taskUnits":[{"id":"t1"}],"results":[{"taskId":"t1","status":"completed"}]},{"id":"exec-new","goal":"newer goal","status":"running","updatedAt":"2026-03-08T05:00:00Z","taskUnits":[{"id":"t1"},{"id":"t2"}],"results":[{"taskId":"t1","status":"completed"}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	opts := orchestrateCommandOptions{
		Action: "list",
		Limit:  1,
	}
	var out bytes.Buffer
	if err := runOrchestrateCommand(&out, opts); err != nil {
		t.Fatalf("runOrchestrateCommand(list) error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "exec-new") {
		t.Fatalf("expected newest execution in output, got %q", output)
	}
	if strings.Contains(output, "exec-old") {
		t.Fatalf("did not expect trimmed older execution in output, got %q", output)
	}
}

func TestRunOrchestrateCancelCommand(t *testing.T) {
	var cancelBody string

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/orchestrator/executions/exec-cancel/cancel":
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			raw, _ := io.ReadAll(r.Body)
			cancelBody = strings.TrimSpace(string(raw))
			_, _ = w.Write([]byte(`{"result":"ok","execution":{"id":"exec-cancel","goal":"triage issue","status":"cancelled","error":"execution cancelled by carrier-cli","taskUnits":[{"id":"t1","hostId":"local","agentId":"zeroclaw"}],"results":[{"taskId":"t1","status":"failed","hostId":"local","agentId":"zeroclaw","error":"cancelled","latencyMs":5}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	opts := orchestrateCommandOptions{
		Action:      "cancel",
		ExecutionID: "exec-cancel",
	}
	var out bytes.Buffer
	if err := runOrchestrateCommand(&out, opts); err != nil {
		t.Fatalf("runOrchestrateCommand(cancel) error: %v", err)
	}

	if !strings.Contains(cancelBody, `"actor":"carrier-cli"`) {
		t.Fatalf("cancel body = %q, want actor=carrier-cli", cancelBody)
	}
	output := out.String()
	if !strings.Contains(output, "status: cancelled") {
		t.Fatalf("expected cancelled status in output, got %q", output)
	}
	if !strings.Contains(output, "execution cancelled by carrier-cli") {
		t.Fatalf("expected cancel reason in output, got %q", output)
	}
}
