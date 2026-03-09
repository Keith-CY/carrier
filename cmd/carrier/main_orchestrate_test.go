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
		"--host-label", "gpu",
		"--host-label", "prod",
		"--host-label", "gpu",
		"--provider", "openrouter",
		"--max-concurrency", "9",
		"--idempotency-key", "idem-1",
		"--policy-approve",
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
	if len(opts.HostLabels) != 2 || opts.HostLabels[0] != "gpu" || opts.HostLabels[1] != "prod" {
		t.Fatalf("host_labels = %v, want [gpu prod]", opts.HostLabels)
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
	if !opts.PolicyApprove {
		t.Fatalf("policyApprove should be true")
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

	authorizeOpts, err := parseOrchestrateCommandArgs([]string{"authorize", "exec-789", "--policy-approve", "--json"})
	if err != nil {
		t.Fatalf("parseOrchestrateCommandArgs(authorize) error: %v", err)
	}
	if authorizeOpts.Action != "authorize" || authorizeOpts.ExecutionID != "exec-789" || !authorizeOpts.PolicyApprove || !authorizeOpts.JSON {
		t.Fatalf("unexpected authorize opts: %+v", authorizeOpts)
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

	retryOpts, err := parseExecutionsCommandArgs([]string{"retry", "exec-77", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(retry) error: %v", err)
	}
	if retryOpts.Action != "retry" || retryOpts.ExecutionID != "exec-77" || !retryOpts.JSON {
		t.Fatalf("unexpected retry opts: %+v", retryOpts)
	}

	rerunOpts, err := parseExecutionsCommandArgs([]string{"rerun", "exec-88", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(rerun) error: %v", err)
	}
	if rerunOpts.Action != "rerun" || rerunOpts.ExecutionID != "exec-88" || !rerunOpts.JSON {
		t.Fatalf("unexpected rerun opts: %+v", rerunOpts)
	}

	cloneOpts, err := parseExecutionsCommandArgs([]string{"clone", "exec-99", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(clone) error: %v", err)
	}
	if cloneOpts.Action != "clone" || cloneOpts.ExecutionID != "exec-99" || !cloneOpts.JSON {
		t.Fatalf("unexpected clone opts: %+v", cloneOpts)
	}

	artifactsOpts, err := parseExecutionsCommandArgs([]string{"artifacts", "exec-100", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(artifacts) error: %v", err)
	}
	if artifactsOpts.Action != "artifacts" || artifactsOpts.ExecutionID != "exec-100" || !artifactsOpts.JSON {
		t.Fatalf("unexpected artifacts opts: %+v", artifactsOpts)
	}

	authorizeOpts, err := parseExecutionsCommandArgs([]string{"authorize", "exec-66", "--policy-approve", "--json"})
	if err != nil {
		t.Fatalf("parseExecutionsCommandArgs(authorize) error: %v", err)
	}
	if authorizeOpts.Action != "authorize" || authorizeOpts.ExecutionID != "exec-66" || !authorizeOpts.PolicyApprove || !authorizeOpts.JSON {
		t.Fatalf("unexpected authorize opts: %+v", authorizeOpts)
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

func TestRunOrchestrateCommandDryRunPlanWithHostLabels(t *testing.T) {
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
		Action:     "plan",
		Goal:       "triage issue",
		HostLabels: []string{"gpu", "prod"},
		Timeout:    2 * time.Second,
	}
	var out bytes.Buffer
	if err := runOrchestrateCommand(&out, opts); err != nil {
		t.Fatalf("runOrchestrateCommand(plan with host labels) error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "labels[gpu,prod]/zeroclaw") {
		t.Fatalf("expected labels[gpu,prod]/zeroclaw in plan output, got %q", output)
	}
	if !strings.Contains(output, "labels[gpu,prod]/picoclaw") {
		t.Fatalf("expected labels[gpu,prod]/picoclaw in plan output, got %q", output)
	}
}

func TestRenderOrchestrateExecutionIncludesPolicySummary(t *testing.T) {
	raw := []byte(`{
		"result":"ok",
		"execution":{
			"id":"exec-policy-1",
			"goal":"triage issue",
			"status":"running",
			"policy":{
				"decision":"ask",
				"reason":"prod picoclaw runs require operator approval",
				"summary":"infrastructure approval required; tool mode restricted; effective concurrency 2",
				"matchedRuleName":"review picoclaw production runs",
				"configuredMaxConcurrency":9,
				"effectiveMaxConcurrency":2,
				"maxTaskTimeoutMs":120000,
				"maxRetryBudget":3,
				"approvedBy":"operator-ui",
				"approvedAt":"2026-03-09T12:00:00Z",
				"toolPolicy":{"mode":"restricted","allowedTools":["grep","shell"]},
				"targets":[
					{"hostId":"local","agentId":"zeroclaw","count":1},
					{"hostId":"host-b","agentId":"picoclaw","count":2}
				]
			},
			"taskUnits":[
				{"id":"task-1","input":"collect traces","hostId":"local","agentId":"zeroclaw"},
				{"id":"task-2","input":"summarize traces","hostId":"host-b","agentId":"picoclaw"}
			],
			"results":[]
		}
	}`)
	resp, err := decodeOrchestrateExecutionResponse(raw)
	if err != nil {
		t.Fatalf("decodeOrchestrateExecutionResponse error: %v", err)
	}

	out := renderOrchestrateExecution(resp)
	if !strings.Contains(out, "policy: ask") {
		t.Fatalf("expected policy decision in output, got %q", out)
	}
	if !strings.Contains(out, "policy rule: review picoclaw production runs") {
		t.Fatalf("expected policy rule in output, got %q", out)
	}
	if !strings.Contains(out, "policy approved by: operator-ui") {
		t.Fatalf("expected policy approval actor in output, got %q", out)
	}
	if !strings.Contains(out, "tool mode=restricted") {
		t.Fatalf("expected tool mode in output, got %q", out)
	}
	if !strings.Contains(out, "effective concurrency=2") {
		t.Fatalf("expected effective concurrency in output, got %q", out)
	}
	if !strings.Contains(out, "max timeout=120000ms") {
		t.Fatalf("expected max timeout in output, got %q", out)
	}
	if !strings.Contains(out, "host-b/picoclaw x2") {
		t.Fatalf("expected policy target in output, got %q", out)
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

func TestRunOrchestrateDerivedExecutionCommands(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/orchestrator/executions/exec-source/retry":
			_, _ = w.Write([]byte(`{"result":"ok","execution":{"id":"exec-retry","goal":"retry source","status":"pending_authorization","parentExecutionId":"exec-source","sourceExecutionId":"exec-source","launchReason":"retry_failed_tasks"}}`))
		case "/api/v1/orchestrator/executions/exec-source/rerun":
			_, _ = w.Write([]byte(`{"result":"ok","execution":{"id":"exec-rerun","goal":"rerun source","status":"pending_authorization","parentExecutionId":"exec-source","sourceExecutionId":"exec-source","launchReason":"rerun_execution"}}`))
		case "/api/v1/orchestrator/executions/exec-source/clone":
			_, _ = w.Write([]byte(`{"result":"ok","execution":{"id":"exec-clone","goal":"clone source","status":"pending_authorization","parentExecutionId":"exec-source","sourceExecutionId":"exec-source","launchReason":"clone_execution"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	cases := []struct {
		action string
		wantID string
	}{
		{action: "retry", wantID: "exec-retry"},
		{action: "rerun", wantID: "exec-rerun"},
		{action: "clone", wantID: "exec-clone"},
	}

	for _, tc := range cases {
		var out bytes.Buffer
		err := runOrchestrateCommand(&out, orchestrateCommandOptions{
			Action:      tc.action,
			ExecutionID: "exec-source",
			JSON:        true,
		})
		if err != nil {
			t.Fatalf("runOrchestrateCommand(%s) error: %v", tc.action, err)
		}
		if !strings.Contains(out.String(), tc.wantID) {
			t.Fatalf("runOrchestrateCommand(%s) output=%s want id=%s", tc.action, out.String(), tc.wantID)
		}
	}
}

func TestRenderOrchestrateExecutionIncludesLineageAndArtifacts(t *testing.T) {
	raw := []byte(`{
		"result":"ok",
		"execution":{
			"id":"exec-derived-1",
			"goal":"retry release notes generation",
			"triggerSource":"github",
			"triggerId":"trigger-gh-1",
			"triggerEvent":"issue_comment",
			"initiator":"github:alice",
			"parentExecutionId":"exec-source-1",
			"sourceExecutionId":"exec-source-1",
			"launchReason":"retry_failed_tasks",
			"status":"retryable_failed",
			"outcome":{
				"summary":"one task failed and can be retried",
				"failureReason":"provider timeout",
				"failureCategory":"provider_failed",
				"artifacts":[
					{"id":"artifact-1","taskId":"task-2","name":"release-notes.txt","kind":"text","contentType":"text/plain","sizeBytes":128}
				]
			},
			"taskUnits":[
				{"id":"task-2","input":"draft release notes","hostId":"local","agentId":"zeroclaw"}
			],
			"results":[
				{"taskId":"task-2","status":"failed","summary":"provider timeout","failureReason":"timeout","failureCategory":"provider_failed","latencyMs":18}
			]
		}
	}`)
	resp, err := decodeOrchestrateExecutionResponse(raw)
	if err != nil {
		t.Fatalf("decodeOrchestrateExecutionResponse error: %v", err)
	}

	out := renderOrchestrateExecution(resp)
	if !strings.Contains(out, "lineage: parent=exec-source-1 source=exec-source-1 launch=retry_failed_tasks") {
		t.Fatalf("expected lineage in output, got %q", out)
	}
	if !strings.Contains(out, "trigger: source=github id=trigger-gh-1 event=issue_comment initiator=github:alice") {
		t.Fatalf("expected trigger metadata in output, got %q", out)
	}
	if !strings.Contains(out, "outcome: one task failed and can be retried") {
		t.Fatalf("expected outcome summary in output, got %q", out)
	}
	if !strings.Contains(out, "failure: provider_failed (provider timeout)") {
		t.Fatalf("expected failure summary in output, got %q", out)
	}
	if !strings.Contains(out, "artifacts:") || !strings.Contains(out, "release-notes.txt") {
		t.Fatalf("expected artifacts in output, got %q", out)
	}
}

func TestRunExecutionArtifactsCommand(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/orchestrator/executions/exec-artifacts/artifacts":
			_, _ = w.Write([]byte(`{"result":"ok","artifacts":[{"id":"artifact-1","taskId":"task-2","name":"release-notes.txt","kind":"text","contentType":"text/plain","sizeBytes":128},{"id":"artifact-2","taskId":"task-3","name":"summary.json","kind":"json","contentType":"application/json","sizeBytes":64}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	setProbeEnvFromURL(t, "CARRIER_GATEWAY_HOST", "CARRIER_GATEWAY_PORT", gateway.URL)

	var out bytes.Buffer
	err := runOrchestrateCommand(&out, orchestrateCommandOptions{
		Action:      "artifacts",
		ExecutionID: "exec-artifacts",
	})
	if err != nil {
		t.Fatalf("runOrchestrateCommand(artifacts) error: %v", err)
	}
	if !strings.Contains(out.String(), "artifact-1") || !strings.Contains(out.String(), "release-notes.txt") || !strings.Contains(out.String(), "summary.json") {
		t.Fatalf("unexpected artifacts output: %q", out.String())
	}
}
