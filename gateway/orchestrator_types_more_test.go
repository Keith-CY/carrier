package gateway

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeOrchestratorRequiredWorkerDefaultsAndValidation(t *testing.T) {
	out, err := normalizeOrchestratorRequiredWorker(OrchestratorRequiredWorker{
		HostID:     " host-1 ",
		HostLabels: []string{" Prod ", "gpu", "prod"},
		AgentID:    "",
		Count:      0,
	})
	if err != nil {
		t.Fatalf("normalizeOrchestratorRequiredWorker defaulting failed: %v", err)
	}
	if out.HostID != "host-1" {
		t.Fatalf("expected trimmed host id, got %q", out.HostID)
	}
	if strings.Join(out.HostLabels, ",") != "gpu,prod" {
		t.Fatalf("expected normalized host labels gpu,prod got %v", out.HostLabels)
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

	selectorOnly, err := normalizeOrchestratorRequiredWorker(OrchestratorRequiredWorker{
		HostLabels: []string{" staging "},
		AgentID:    "picoclaw",
	})
	if err != nil {
		t.Fatalf("normalizeOrchestratorRequiredWorker selector-only failed: %v", err)
	}
	if selectorOnly.HostID != "" || strings.Join(selectorOnly.HostLabels, ",") != "staging" {
		t.Fatalf("unexpected selector-only worker: %+v", selectorOnly)
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
		HostLabels:  []string{" gpu ", "prod"},
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
	if strings.Join(out.HostLabels, ",") != "gpu,prod" {
		t.Fatalf("expected normalized host labels gpu,prod got %v", out.HostLabels)
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
	}); err == nil || !strings.Contains(err.Error(), "requiredWorkers.hostId or requiredWorkers.hostLabels is required") {
		t.Fatalf("expected worker hostId validation error, got %v", err)
	}

	selectorExec, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal: "g",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostLabels: []string{"prod"}, AgentID: "picoclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{{ID: "t1", Input: "hello", HostLabels: []string{"prod"}, AgentID: "picoclaw"}},
	})
	if err != nil {
		t.Fatalf("expected selector-based execution validation success, got %v", err)
	}
	if selectorExec.RequiredWorkers[0].HostID != "" || strings.Join(selectorExec.RequiredWorkers[0].HostLabels, ",") != "prod" {
		t.Fatalf("unexpected normalized selector worker: %+v", selectorExec.RequiredWorkers[0])
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
		RequiredMemory: []string{" shared:incident ", "private:checkout", "shared:incident"},
		DistillOutputs: []string{"shared:distill", " shared:distill "},
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
	if strings.Join(out.RequiredMemory, ",") != "private:checkout,shared:incident" {
		t.Fatalf("expected normalized requiredMemory, got %v", out.RequiredMemory)
	}
	if strings.Join(out.DistillOutputs, ",") != "shared:distill" {
		t.Fatalf("expected normalized distillOutputs, got %v", out.DistillOutputs)
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

func TestNormalizeOrchestratorExecutionStoresDelegatedMemoryFields(t *testing.T) {
	out, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal:                  " delegate incident ",
		AgentLifecycleMode:    " Delegated ",
		MemoryBindingMode:     " snapshot ",
		SourceScopes:          []string{" shared:team ", "public", "shared:team"},
		SnapshotID:            " snapshot-1 ",
		ChildAgentID:          " child-agent ",
		ChildPerAgentMemoryID: " child-memory ",
		DistillRunID:          " distill-1 ",
		CleanupStatus:         " Cleanup Pending ",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "host-1", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{
			{Input: "collect incident context"},
		},
	})
	if err != nil {
		t.Fatalf("normalizeOrchestratorExecution delegated memory fields failed: %v", err)
	}
	if out.AgentLifecycleMode != "delegated" {
		t.Fatalf("agentLifecycleMode = %q, want delegated", out.AgentLifecycleMode)
	}
	if out.MemoryBindingMode != "snapshot" {
		t.Fatalf("memoryBindingMode = %q, want snapshot", out.MemoryBindingMode)
	}
	if strings.Join(out.SourceScopes, ",") != "public,shared:team" {
		t.Fatalf("sourceScopes = %v, want [public shared:team]", out.SourceScopes)
	}
	if out.SnapshotDigest != buildMemoryContractDigest([]string{"public", "shared:team"}) {
		t.Fatalf("snapshotDigest = %q, want derived source scope digest", out.SnapshotDigest)
	}
	if out.SnapshotID != "snapshot-1" {
		t.Fatalf("snapshotId = %q, want snapshot-1", out.SnapshotID)
	}
	if out.ChildAgentID != "child-agent" {
		t.Fatalf("childAgentId = %q, want child-agent", out.ChildAgentID)
	}
	if out.ChildPerAgentMemoryID != "child-memory" {
		t.Fatalf("childPerAgentMemoryId = %q, want child-memory", out.ChildPerAgentMemoryID)
	}
	if out.DistillRunID != "distill-1" {
		t.Fatalf("distillRunId = %q, want distill-1", out.DistillRunID)
	}
	if out.CleanupStatus != "cleanup_pending" {
		t.Fatalf("cleanupStatus = %q, want cleanup_pending", out.CleanupStatus)
	}
}

func TestNormalizeOrchestratorExecutionForStorePreservesLineageAndOutcome(t *testing.T) {
	out := normalizeOrchestratorExecutionForStore(OrchestratorExecution{
		ID:                " exec-1 ",
		Goal:              "  investigate failure  ",
		RequiredMemory:    []string{"shared:incident", "private:checkout"},
		DistillOutputs:    []string{"shared:distill"},
		ParentExecutionID: " parent-1 ",
		SourceExecutionID: " source-1 ",
		LaunchReason:      " retry_failed_tasks ",
		Status:            OrchestratorExecutionStatusRetryableFailed,
		Outcome: OrchestratorExecutionOutcome{
			Summary:         " retryable failure ",
			FailureReason:   " provider timeout ",
			FailureCategory: " provider_failed ",
		},
		Results: []OrchestratorTaskResult{
			{
				TaskID:          "task-1",
				Status:          OrchestratorTaskStatusFailed,
				Summary:         " collect failed ",
				FailureReason:   " timeout ",
				FailureCategory: " provider_failed ",
			},
		},
	})

	if out.ID != "exec-1" || out.Goal != "investigate failure" {
		t.Fatalf("unexpected normalized id/goal: %+v", out)
	}
	if out.ParentExecutionID != "parent-1" {
		t.Fatalf("parentExecutionId = %q, want parent-1", out.ParentExecutionID)
	}
	if out.SourceExecutionID != "source-1" {
		t.Fatalf("sourceExecutionId = %q, want source-1", out.SourceExecutionID)
	}
	if out.LaunchReason != "retry_failed_tasks" {
		t.Fatalf("launchReason = %q, want retry_failed_tasks", out.LaunchReason)
	}
	if out.MemoryContractDigest == "" {
		t.Fatalf("expected memoryContractDigest to be hydrated, got %+v", out)
	}
	if strings.Join(out.MemoryProvenance, ",") != "private:checkout,shared:incident" {
		t.Fatalf("memoryProvenance = %v, want [private:checkout shared:incident]", out.MemoryProvenance)
	}
	if strings.Join(out.DistillOutputs, ",") != "shared:distill" {
		t.Fatalf("distillOutputs = %v, want [shared:distill]", out.DistillOutputs)
	}
	if out.Status != OrchestratorExecutionStatusRetryableFailed {
		t.Fatalf("status = %q, want retryable_failed", out.Status)
	}
	if out.Outcome.Summary != "retryable failure" || out.Outcome.FailureReason != "provider timeout" || out.Outcome.FailureCategory != "provider_failed" {
		t.Fatalf("unexpected normalized outcome: %+v", out.Outcome)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(out.Results))
	}
	if out.Results[0].Summary != "collect failed" || out.Results[0].FailureReason != "timeout" || out.Results[0].FailureCategory != "provider_failed" {
		t.Fatalf("unexpected normalized result: %+v", out.Results[0])
	}
}

func TestNormalizeOrchestratorExecutionForStoreHydratesProviderUsage(t *testing.T) {
	out := normalizeOrchestratorExecutionForStore(OrchestratorExecution{
		ID:                "exec-provider-usage",
		Goal:              "collect provider usage",
		RequestedProvider: "openrouter",
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: "host-1", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{
			{ID: "task-1", Input: "collect deployment diagnostics for checkout", HostID: "host-1", AgentID: "zeroclaw"},
		},
		Results: []OrchestratorTaskResult{
			{
				TaskID:    "task-1",
				Status:    OrchestratorTaskStatusCompleted,
				HostID:    "host-1",
				AgentID:   "zeroclaw",
				Summary:   "diagnostics summarized for checkout rollback",
				LatencyMs: 1800,
			},
		},
		Governance: OrchestratorExecutionGovernance{
			ProviderResolutions: []ProviderGovernanceResolution{{
				HostID:   "host-1",
				AgentID:  "zeroclaw",
				Source:   "instance",
				Provider: "openrouter",
				Model:    "anthropic/claude-3.7-sonnet",
			}},
		},
	})

	if len(out.Governance.ProviderResolutions) != 1 {
		t.Fatalf("expected one provider resolution, got %+v", out.Governance.ProviderResolutions)
	}
	resolution := out.Governance.ProviderResolutions[0]
	if resolution.EstimatedInputTokens <= 0 {
		t.Fatalf("expected estimated input tokens, got %+v", resolution)
	}
	if resolution.EstimatedOutputTokens <= 0 {
		t.Fatalf("expected estimated output tokens, got %+v", resolution)
	}
	if resolution.EstimatedTotalTokens != resolution.EstimatedInputTokens+resolution.EstimatedOutputTokens {
		t.Fatalf("expected total tokens to match input+output, got %+v", resolution)
	}
	if resolution.EstimatedCostUSD <= 0 {
		t.Fatalf("expected estimated cost usd, got %+v", resolution)
	}
	if resolution.SuccessfulTasks != 1 || resolution.FailedTasks != 0 {
		t.Fatalf("expected successful task accounting, got %+v", resolution)
	}
	if resolution.AvgLatencyMs != 1800 {
		t.Fatalf("expected avg latency 1800, got %+v", resolution)
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
		ToolPolicy: OrchestratorToolPolicy{
			Mode:         " restricted ",
			AllowedTools: []string{" shell ", "grep", "shell"},
		},
		RequiredWorkers: []OrchestratorRequiredWorker{
			{HostID: " local ", AgentID: "zeroclaw", Count: 1},
		},
		TaskUnits: []OrchestratorTaskUnit{
			{ID: "t1", Input: "hello", TimeoutMs: 45000, RetryBudget: 1},
			{ID: "t2", Input: "world", TimeoutMs: 120000, RetryBudget: 3},
		},
		Error: "  boom  ",
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
	if exec.Policy.Decision != "allow" {
		t.Fatalf("expected default policy decision allow, got %+v", exec.Policy)
	}
	if exec.Policy.EffectiveMaxConcurrency != 2 {
		t.Fatalf("expected effective maxConcurrency 2, got %+v", exec.Policy)
	}
	if exec.Policy.MaxTaskTimeoutMs != 120000 {
		t.Fatalf("expected max task timeout 120000, got %+v", exec.Policy)
	}
	if exec.Policy.MaxRetryBudget != 3 {
		t.Fatalf("expected max retry budget 3, got %+v", exec.Policy)
	}
	if len(exec.Policy.ToolPolicy.AllowedTools) != 2 {
		t.Fatalf("expected normalized allowed tools, got %+v", exec.Policy.ToolPolicy.AllowedTools)
	}

	defaultExec := normalizeOrchestratorExecutionForStore(OrchestratorExecution{
		ID: "exec-2",
	})
	if defaultExec.MaxConcurrency != defaultOrchestratorMaxConcurrency {
		t.Fatalf("expected default maxConcurrency %d, got %d", defaultOrchestratorMaxConcurrency, defaultExec.MaxConcurrency)
	}
	if defaultExec.Policy.Decision != "allow" {
		t.Fatalf("expected default policy decision allow, got %+v", defaultExec.Policy)
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
	if lease.LeaseState != string(OrchestratorWorkerStateProvisioning) {
		t.Fatalf("expected leaseState to mirror state, got %q", lease.LeaseState)
	}
	if lease.QueuePosition != 0 {
		t.Fatalf("expected default queuePosition=0, got %d", lease.QueuePosition)
	}
}

func TestIsWorkerLeaseStale(t *testing.T) {
	now := time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	cfg := &GatewayConfig{
		WorkerLeaseStaleAfter:     5 * time.Minute,
		WorkerHeartbeatTimeout:    2 * time.Minute,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		MaxCommandBodyBytes:       64 * 1024,
	}

	tests := []struct {
		name       string
		lease      OrchestratorWorkerLease
		executions map[string]OrchestratorExecution
		wantStale  bool
		wantReason string
	}{
		{
			name: "lease expired",
			lease: OrchestratorWorkerLease{
				ID:            "lease-1",
				ExecutionID:   "exec-1",
				HostID:        "host-1",
				AgentID:       "zeroclaw",
				State:         OrchestratorWorkerStateReady,
				LeaseExpireAt: now.Add(-time.Minute).Format(time.RFC3339),
				HeartbeatAt:   now.Add(-30 * time.Second).Format(time.RFC3339),
			},
			wantStale:  true,
			wantReason: "lease_expired",
		},
		{
			name: "heartbeat timeout",
			lease: OrchestratorWorkerLease{
				ID:          "lease-2",
				ExecutionID: "exec-2",
				HostID:      "host-1",
				AgentID:     "zeroclaw",
				State:       OrchestratorWorkerStateBusy,
				HeartbeatAt: now.Add(-3 * time.Minute).Format(time.RFC3339),
			},
			wantStale:  true,
			wantReason: "heartbeat_timeout",
		},
		{
			name: "terminal execution left busy",
			lease: OrchestratorWorkerLease{
				ID:          "lease-3",
				ExecutionID: "exec-3",
				HostID:      "host-1",
				AgentID:     "zeroclaw",
				State:       OrchestratorWorkerStateBusy,
				HeartbeatAt: now.Add(-30 * time.Second).Format(time.RFC3339),
			},
			executions: map[string]OrchestratorExecution{
				"exec-3": {
					ID:     "exec-3",
					Status: OrchestratorExecutionStatusCompleted,
				},
			},
			wantStale:  true,
			wantReason: "execution_terminal",
		},
		{
			name: "healthy busy lease",
			lease: OrchestratorWorkerLease{
				ID:            "lease-4",
				ExecutionID:   "exec-4",
				HostID:        "host-1",
				AgentID:       "zeroclaw",
				State:         OrchestratorWorkerStateBusy,
				HeartbeatAt:   now.Add(-30 * time.Second).Format(time.RFC3339),
				LeaseExpireAt: now.Add(4 * time.Minute).Format(time.RFC3339),
			},
			executions: map[string]OrchestratorExecution{
				"exec-4": {
					ID:     "exec-4",
					Status: OrchestratorExecutionStatusRunning,
				},
			},
			wantStale: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStale, gotReason := isWorkerLeaseStale(tc.lease, tc.executions, now, cfg)
			if gotStale != tc.wantStale || gotReason != tc.wantReason {
				t.Fatalf("isWorkerLeaseStale(%s) = stale=%v reason=%q, want stale=%v reason=%q", tc.name, gotStale, gotReason, tc.wantStale, tc.wantReason)
			}
		})
	}
}

func TestMarkStaleWorkerLeases(t *testing.T) {
	now := time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	cfg := &GatewayConfig{
		WorkerLeaseStaleAfter:     5 * time.Minute,
		WorkerHeartbeatTimeout:    2 * time.Minute,
		RemoteControlPlaneEnabled: true,
	}
	leases := []OrchestratorWorkerLease{
		{
			ID:            "lease-expired",
			ExecutionID:   "exec-1",
			HostID:        "host-1",
			AgentID:       "zeroclaw",
			State:         OrchestratorWorkerStateReady,
			LeaseExpireAt: now.Add(-time.Minute).Format(time.RFC3339),
			HeartbeatAt:   now.Add(-time.Minute).Format(time.RFC3339),
		},
		{
			ID:          "lease-healthy",
			ExecutionID: "exec-2",
			HostID:      "host-1",
			AgentID:     "picoclaw",
			State:       OrchestratorWorkerStateBusy,
			HeartbeatAt: now.Add(-time.Minute).Format(time.RFC3339),
		},
	}

	marked := markStaleWorkerLeases(leases, map[string]OrchestratorExecution{
		"exec-2": {ID: "exec-2", Status: OrchestratorExecutionStatusRunning},
	}, now, cfg)
	if !marked[0].Stale || marked[0].StaleReason != "lease_expired" {
		t.Fatalf("expected first lease stale lease_expired, got %+v", marked[0])
	}
	if marked[0].LeaseState != string(OrchestratorWorkerStateReady) {
		t.Fatalf("expected first leaseState ready, got %+v", marked[0])
	}
	if marked[0].LastHeartbeatAt == "" {
		t.Fatalf("expected first lastHeartbeatAt populated, got %+v", marked[0])
	}
	if marked[1].Stale || marked[1].StaleReason != "" {
		t.Fatalf("expected second lease healthy, got %+v", marked[1])
	}
}
