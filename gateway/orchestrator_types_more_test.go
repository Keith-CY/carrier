package gateway

import (
	"strings"
	"testing"
)

func TestNormalizeOrchestratorRequiredWorkerDefaultsAndValidation(t *testing.T) {
	out, err := normalizeOrchestratorRequiredWorker(OrchestratorRequiredWorker{
		HostID:  " host-1 ",
		AgentID: "",
		Count:   0,
	})
	if err != nil {
		t.Fatalf("normalizeOrchestratorRequiredWorker defaulting failed: %v", err)
	}
	if out.HostID != "host-1" {
		t.Fatalf("expected trimmed host id, got %q", out.HostID)
	}
	if out.AgentID != "zeroclaw" {
		t.Fatalf("expected default agent zeroclaw, got %q", out.AgentID)
	}
	if out.Count != 1 {
		t.Fatalf("expected default count=1, got %d", out.Count)
	}

	clamped, err := normalizeOrchestratorRequiredWorker(OrchestratorRequiredWorker{
		HostID:  "h",
		AgentID: "PiCoClAw",
		Count:   999,
	})
	if err != nil {
		t.Fatalf("normalizeOrchestratorRequiredWorker clamp failed: %v", err)
	}
	if clamped.AgentID != "picoclaw" {
		t.Fatalf("expected normalized agent id, got %q", clamped.AgentID)
	}
	if clamped.Count != 64 {
		t.Fatalf("expected count clamp=64, got %d", clamped.Count)
	}

	if _, err := normalizeOrchestratorRequiredWorker(OrchestratorRequiredWorker{
		HostID:  "h",
		AgentID: "bad id",
		Count:   1,
	}); err == nil {
		t.Fatal("expected invalid agent id error")
	}
}

func TestNormalizeOrchestratorTaskValidationAndDefaults(t *testing.T) {
	if _, err := normalizeOrchestratorTask(OrchestratorTaskUnit{
		ID:    "task-1",
		Input: "   ",
	}, 2); err == nil || !strings.Contains(err.Error(), "item 2: task input is required") {
		t.Fatalf("expected indexed task input validation error, got %v", err)
	}

	if _, err := normalizeOrchestratorTask(OrchestratorTaskUnit{
		ID:      "task-1",
		Input:   "ok",
		AgentID: "bad id",
	}, 1); err == nil || !strings.Contains(err.Error(), "item 1:") {
		t.Fatalf("expected indexed invalid agent id error, got %v", err)
	}

	out, err := normalizeOrchestratorTask(OrchestratorTaskUnit{
		Input:       "  do-work  ",
		TimeoutMs:   -1,
		RetryBudget: -5,
		ToolPolicy:  " restricted ",
		SessionID:   " sess-1 ",
	}, 0)
	if err != nil {
		t.Fatalf("normalizeOrchestratorTask defaults failed: %v", err)
	}
	if out.ID != "task-1" {
		t.Fatalf("expected generated id task-1, got %q", out.ID)
	}
	if out.TimeoutMs != 60000 {
		t.Fatalf("expected default timeout 60000, got %d", out.TimeoutMs)
	}
	if out.RetryBudget != 0 {
		t.Fatalf("expected retryBudget clamp to 0, got %d", out.RetryBudget)
	}
	if out.ToolPolicy != "restricted" {
		t.Fatalf("expected trimmed toolPolicy, got %q", out.ToolPolicy)
	}
	if out.SessionID != "sess-1" {
		t.Fatalf("expected trimmed session id, got %q", out.SessionID)
	}

	high, err := normalizeOrchestratorTask(OrchestratorTaskUnit{
		ID:          "task-x",
		Input:       "x",
		TimeoutMs:   999999,
		RetryBudget: 99,
	}, 3)
	if err != nil {
		t.Fatalf("normalizeOrchestratorTask clamp-high failed: %v", err)
	}
	if high.TimeoutMs != 300000 {
		t.Fatalf("expected timeout clamp 300000, got %d", high.TimeoutMs)
	}
	if high.RetryBudget != 5 {
		t.Fatalf("expected retryBudget clamp 5, got %d", high.RetryBudget)
	}
}

func TestNormalizeOrchestratorExecutionValidationAndDefaults(t *testing.T) {
	if _, err := normalizeOrchestratorExecution(OrchestratorExecution{}); err == nil || !strings.Contains(err.Error(), "goal is required") {
		t.Fatalf("expected goal validation error, got %v", err)
	}

	if _, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal:          "g",
		ApprovalScope: "full",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "h1", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
	}); err == nil || !strings.Contains(err.Error(), "approvalScope must be infrastructure_only") {
		t.Fatalf("expected approvalScope validation error, got %v", err)
	}

	if _, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal:      "g",
		TaskUnits: []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
	}); err == nil || !strings.Contains(err.Error(), "requiredWorkers is required") {
		t.Fatalf("expected requiredWorkers validation error, got %v", err)
	}

	if _, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal: "g",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
	}); err == nil || !strings.Contains(err.Error(), "requiredWorkers.hostId is required") {
		t.Fatalf("expected worker hostId validation error, got %v", err)
	}

	if _, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal: "g",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "h1", AgentID: "bad id", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{{ID: "t1", Input: "hello"}},
	}); err == nil || !strings.Contains(err.Error(), "invalid requiredWorkers entry") {
		t.Fatalf("expected worker agent validation error, got %v", err)
	}

	if _, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal: "g",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "h1", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: nil,
	}); err == nil || !strings.Contains(err.Error(), "taskUnits is required") {
		t.Fatalf("expected taskUnits validation error, got %v", err)
	}

	if _, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal: "g",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "h1", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{{ID: "t1", Input: ""}},
	}); err == nil || !strings.Contains(err.Error(), "task input is required") {
		t.Fatalf("expected task validation error, got %v", err)
	}

	out, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal:           "  orchestrate ",
		MaxConcurrency: 999,
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: " host-1 ", AgentID: "", Count: 0},
		},
		TaskUnits: []OrchestratorTaskUnit{
			{Input: "hello"},
		},
		ToolPolicy: OrchestratorToolPolicy{
			Mode: " ",
		},
	})
	if err != nil {
		t.Fatalf("normalizeOrchestratorExecution defaults failed: %v", err)
	}
	if out.Goal != "orchestrate" {
		t.Fatalf("expected trimmed goal, got %q", out.Goal)
	}
	if out.ApprovalScope != "infrastructure_only" {
		t.Fatalf("expected default approval scope, got %q", out.ApprovalScope)
	}
	if out.MaxConcurrency != 64 {
		t.Fatalf("expected maxConcurrency clamp 64, got %d", out.MaxConcurrency)
	}
	if out.RequiredWorkers[0].HostID != "host-1" || out.RequiredWorkers[0].AgentID != "zeroclaw" || out.RequiredWorkers[0].Count != 1 {
		t.Fatalf("unexpected normalized worker: %+v", out.RequiredWorkers[0])
	}
	if out.TaskUnits[0].ID != "task-1" || out.TaskUnits[0].TimeoutMs != 60000 {
		t.Fatalf("unexpected normalized task: %+v", out.TaskUnits[0])
	}
	if out.ToolPolicy.Mode != "restricted" {
		t.Fatalf("expected default tool policy mode restricted, got %q", out.ToolPolicy.Mode)
	}

	defaultConcurrency, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal: "g",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "h1", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{
			{Input: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("normalizeOrchestratorExecution default concurrency failed: %v", err)
	}
	if defaultConcurrency.MaxConcurrency != 8 {
		t.Fatalf("expected default maxConcurrency 8, got %d", defaultConcurrency.MaxConcurrency)
	}
}

func TestErrOrchestratorValidationFormatting(t *testing.T) {
	if got := errOrchestratorValidation("  bad input  ", 3).Error(); got != "item 3: bad input" {
		t.Fatalf("unexpected indexed error format: %q", got)
	}
	if got := errOrchestratorValidation("  bad input  ", -1).Error(); got != "bad input" {
		t.Fatalf("unexpected non-indexed error format: %q", got)
	}
}

func TestNormalizeOrchestratorStoreHelpers(t *testing.T) {
	exec := normalizeOrchestratorExecutionForStore(OrchestratorExecution{
		ID:             " exec-1 ",
		Goal:           "  g  ",
		IdempotencyKey: " idem ",
		ApprovalScope:  " ",
		MaxConcurrency: 999,
		Error:          "  boom  ",
	})
	if exec.ID != "exec-1" || exec.Goal != "g" || exec.IdempotencyKey != "idem" {
		t.Fatalf("unexpected trimmed execution fields: %+v", exec)
	}
	if exec.ApprovalScope != "infrastructure_only" {
		t.Fatalf("expected default approval scope, got %q", exec.ApprovalScope)
	}
	if exec.MaxConcurrency != 64 {
		t.Fatalf("expected maxConcurrency clamp to 64, got %d", exec.MaxConcurrency)
	}
	if exec.Results == nil {
		t.Fatalf("expected results to be initialized")
	}
	if exec.Error != "boom" {
		t.Fatalf("expected trimmed error, got %q", exec.Error)
	}

	defaultExec := normalizeOrchestratorExecutionForStore(OrchestratorExecution{
		ID: "exec-2",
	})
	if defaultExec.MaxConcurrency != defaultOrchestratorMaxConcurrency {
		t.Fatalf("expected default maxConcurrency %d, got %d", defaultOrchestratorMaxConcurrency, defaultExec.MaxConcurrency)
	}

	lease := normalizeOrchestratorWorkerLeaseForStore(OrchestratorWorkerLease{
		ID:          " lease-1 ",
		ExecutionID: " exec-1 ",
		HostID:      " host-1 ",
		AgentID:     " agent-1 ",
		LastError:   "  err  ",
	})
	if lease.ID != "lease-1" || lease.ExecutionID != "exec-1" || lease.HostID != "host-1" || lease.AgentID != "agent-1" {
		t.Fatalf("unexpected trimmed lease fields: %+v", lease)
	}
	if lease.LastError != "err" {
		t.Fatalf("expected trimmed lease error, got %q", lease.LastError)
	}
	if lease.State != OrchestratorWorkerStateProvisioning {
		t.Fatalf("expected default lease state provisioning, got %q", lease.State)
	}
}
