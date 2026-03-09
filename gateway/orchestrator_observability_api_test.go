package gateway

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOrchestratorMetricsSummary(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		WorkerLeaseStaleAfter:     5 * time.Minute,
		WorkerHeartbeatTimeout:    2 * time.Minute,
	}, nil)
	hostID := createRemoteHostForTests(t, mux)

	if _, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:                "exec-complete",
		Goal:              "prepare summary",
		RequestedProvider: "openrouter",
		Status:            OrchestratorExecutionStatusCompleted,
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  hostID,
			AgentID: "zeroclaw",
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:      "task-1",
			Input:   "prepare release summary for checkout deployment",
			HostID:  hostID,
			AgentID: "zeroclaw",
		}},
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision: "allow",
		},
		Governance: OrchestratorExecutionGovernance{
			ProviderResolutions: []ProviderGovernanceResolution{{
				HostID:     hostID,
				AgentID:    "zeroclaw",
				Provider:   "openrouter",
				Model:      "anthropic/claude-3.7-sonnet",
				DriftState: "in_sync",
			}},
		},
		Results: []OrchestratorTaskResult{{
			TaskID:    "task-1",
			Status:    OrchestratorTaskStatusCompleted,
			HostID:    hostID,
			AgentID:   "zeroclaw",
			Summary:   "release summary ready for checkout deployment",
			Attempts:  2,
			LatencyMs: 700,
		}},
		StartedAt:   "2026-03-09T11:00:00Z",
		CompletedAt: "2026-03-09T11:02:00Z",
		CreatedAt:   nowTimestamp(),
		UpdatedAt:   nowTimestamp(),
	}); err != nil {
		t.Fatalf("upsert exec-complete failed: %v", err)
	}

	if _, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:                "exec-failed-provider",
		Goal:              "diagnose provider issue",
		RequestedProvider: "anthropic",
		Status:            OrchestratorExecutionStatusFailed,
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  hostID,
			AgentID: "picoclaw",
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:      "task-1",
			Input:   "diagnose provider timeout for checkout incident",
			HostID:  hostID,
			AgentID: "picoclaw",
		}},
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision: "ask",
		},
		Governance: OrchestratorExecutionGovernance{
			ProviderResolutions: []ProviderGovernanceResolution{{
				HostID:     hostID,
				AgentID:    "picoclaw",
				Provider:   "anthropic",
				Model:      "claude-3-7-sonnet",
				DriftState: "override",
			}},
		},
		Outcome: OrchestratorExecutionOutcome{
			FailureCategory: "provider_failed",
		},
		Results: []OrchestratorTaskResult{{
			TaskID:          "task-1",
			Status:          OrchestratorTaskStatusFailed,
			HostID:          hostID,
			AgentID:         "picoclaw",
			Attempts:        3,
			FailureCategory: "provider_failed",
			FailureReason:   "provider timeout",
			LatencyMs:       900,
		}},
		StartedAt:   "2026-03-09T12:00:00Z",
		CompletedAt: "2026-03-09T12:01:00Z",
		CreatedAt:   nowTimestamp(),
		UpdatedAt:   nowTimestamp(),
	}); err != nil {
		t.Fatalf("upsert exec-failed-provider failed: %v", err)
	}

	if _, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:                "exec-cancelled",
		Goal:              "cancel task",
		RequestedProvider: "openrouter",
		Status:            OrchestratorExecutionStatusCancelled,
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision: "deny",
		},
		CreatedAt: nowTimestamp(),
		UpdatedAt: nowTimestamp(),
	}); err != nil {
		t.Fatalf("upsert exec-cancelled failed: %v", err)
	}

	for _, lease := range []OrchestratorWorkerLease{
		{
			ID:              "lease-busy-stale",
			ExecutionID:     "exec-cancelled",
			HostID:          hostID,
			AgentID:         "zeroclaw",
			State:           OrchestratorWorkerStateBusy,
			LeaseState:      "busy",
			HeartbeatAt:     time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
			LastHeartbeatAt: time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
			CreatedAt:       time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
			UpdatedAt:       time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
		},
		{
			ID:              "lease-ready",
			ExecutionID:     "exec-complete",
			HostID:          hostID,
			AgentID:         "picoclaw",
			State:           OrchestratorWorkerStateReady,
			LeaseState:      "ready",
			HeartbeatAt:     time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
			LastHeartbeatAt: time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
			CreatedAt:       time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
			UpdatedAt:       time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		},
		{
			ID:              "lease-error",
			ExecutionID:     "exec-failed-provider",
			HostID:          hostID,
			AgentID:         "picoclaw",
			State:           OrchestratorWorkerStateError,
			LeaseState:      "error",
			LastError:       "provider timeout",
			HeartbeatAt:     time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
			LastHeartbeatAt: time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
			CreatedAt:       time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
			UpdatedAt:       time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		},
	} {
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			t.Fatalf("upsert lease %s failed: %v", lease.ID, err)
		}
	}

	rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/metrics", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", rec.Code, rec.Body.String())
	}

	payload := decodeJSONMap(t, rec)
	metrics, _ := payload["metrics"].(map[string]interface{})
	executions, _ := metrics["executions"].(map[string]interface{})
	if got := int(anyToFloat(executions["total"])); got != 3 {
		t.Fatalf("executions.total=%d want 3 metrics=%+v", got, metrics)
	}
	if got := int(anyToFloat(executions["completed"])); got != 1 {
		t.Fatalf("executions.completed=%d want 1", got)
	}
	if got := int(anyToFloat(executions["failed"])); got != 1 {
		t.Fatalf("executions.failed=%d want 1", got)
	}
	if got := int(anyToFloat(executions["cancelled"])); got != 1 {
		t.Fatalf("executions.cancelled=%d want 1", got)
	}
	if got := int(anyToFloat(executions["retryCount"])); got != 3 {
		t.Fatalf("executions.retryCount=%d want 3", got)
	}
	if got := int(anyToFloat(executions["avgLatencyMs"])); got != 90000 {
		t.Fatalf("executions.avgLatencyMs=%d want 90000", got)
	}

	workers, _ := metrics["workers"].(map[string]interface{})
	if got := int(anyToFloat(workers["total"])); got != 3 {
		t.Fatalf("workers.total=%d want 3", got)
	}
	if got := int(anyToFloat(workers["busy"])); got != 1 {
		t.Fatalf("workers.busy=%d want 1", got)
	}
	if got := int(anyToFloat(workers["ready"])); got != 1 {
		t.Fatalf("workers.ready=%d want 1", got)
	}
	if got := int(anyToFloat(workers["error"])); got != 1 {
		t.Fatalf("workers.error=%d want 1", got)
	}
	if got := int(anyToFloat(workers["stale"])); got != 1 {
		t.Fatalf("workers.stale=%d want 1", got)
	}

	providers, _ := metrics["providers"].(map[string]interface{})
	requested, _ := providers["requestedFailures"].(map[string]interface{})
	if got := int(anyToFloat(requested["anthropic"])); got != 1 {
		t.Fatalf("providers.requestedFailures[anthropic]=%d want 1 providers=%+v", got, providers)
	}
	resolved, _ := providers["resolvedFailures"].(map[string]interface{})
	if got := int(anyToFloat(resolved["anthropic"])); got != 1 {
		t.Fatalf("providers.resolvedFailures[anthropic]=%d want 1 providers=%+v", got, providers)
	}
	driftStates, _ := providers["driftStates"].(map[string]interface{})
	if got := int(anyToFloat(driftStates["override"])); got != 1 {
		t.Fatalf("providers.driftStates[override]=%d want 1 providers=%+v", got, providers)
	}
	if got := int(anyToFloat(driftStates["in_sync"])); got != 1 {
		t.Fatalf("providers.driftStates[in_sync]=%d want 1 providers=%+v", got, providers)
	}
	if got := anyToFloat(providers["totalEstimatedCostUsd"]); got <= 0 {
		t.Fatalf("providers.totalEstimatedCostUsd=%f want > 0 providers=%+v", got, providers)
	}
	aggregates, _ := providers["aggregates"].([]interface{})
	if len(aggregates) < 2 {
		t.Fatalf("providers.aggregates len=%d want >=2 providers=%+v", len(aggregates), providers)
	}
	var anthropicAggregate map[string]interface{}
	for _, item := range aggregates {
		current, _ := item.(map[string]interface{})
		if strings.TrimSpace(anyToString(current["provider"])) == "anthropic" {
			anthropicAggregate = current
			break
		}
	}
	if anthropicAggregate == nil {
		t.Fatalf("expected anthropic aggregate providers=%+v", providers)
	}
	if got := int(anyToFloat(anthropicAggregate["failures"])); got != 1 {
		t.Fatalf("anthropic failures=%d want 1 aggregate=%+v", got, anthropicAggregate)
	}
	if got := anyToFloat(anthropicAggregate["estimatedCostUsd"]); got <= 0 {
		t.Fatalf("anthropic estimatedCostUsd=%f want > 0 aggregate=%+v", got, anthropicAggregate)
	}
	models, _ := providers["models"].([]interface{})
	if len(models) < 2 {
		t.Fatalf("providers.models len=%d want >=2 providers=%+v", len(models), providers)
	}
	var anthropicModel map[string]interface{}
	for _, item := range models {
		current, _ := item.(map[string]interface{})
		if strings.TrimSpace(anyToString(current["provider"])) == "anthropic" {
			anthropicModel = current
			break
		}
	}
	if anthropicModel == nil {
		t.Fatalf("expected anthropic model aggregate providers=%+v", providers)
	}
	if got := int(anyToFloat(anthropicModel["failures"])); got != 1 {
		t.Fatalf("anthropic model failures=%d want 1 aggregate=%+v", got, anthropicModel)
	}

	policies, _ := metrics["policies"].(map[string]interface{})
	if got := int(anyToFloat(policies["deny"])); got != 1 {
		t.Fatalf("policies.deny=%d want 1", got)
	}
	if got := int(anyToFloat(policies["ask"])); got != 1 {
		t.Fatalf("policies.ask=%d want 1", got)
	}
}
