package gateway

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteControlStorePathDefaultAndOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "remote-control.json")
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", custom)

	path, err := remoteControlStorePath()
	if err != nil {
		t.Fatalf("remoteControlStorePath custom failed: %v", err)
	}
	if path != custom {
		t.Fatalf("expected custom path %q, got %q", custom, path)
	}

	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", "")
	path, err = remoteControlStorePath()
	if err != nil {
		t.Fatalf("remoteControlStorePath default failed: %v", err)
	}
	if !strings.Contains(path, ".carrier/remote-control.json") {
		t.Fatalf("expected default remote-control path, got %q", path)
	}
}

func TestOrchestratorExecutionStoreLifecycle(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	executions, err := listOrchestratorExecutions()
	if err != nil {
		t.Fatalf("listOrchestratorExecutions empty failed: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("expected empty executions, got %d", len(executions))
	}

	if _, err := upsertOrchestratorExecution(OrchestratorExecution{}); err == nil {
		t.Fatal("expected upsertOrchestratorExecution to fail on empty id")
	}

	created, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:                " exec-1 ",
		Goal:              "  plan and run ",
		IdempotencyKey:    " idem-1 ",
		ParentExecutionID: " parent-0 ",
		SourceExecutionID: " source-0 ",
		LaunchReason:      " rerun_execution ",
		Status:            OrchestratorExecutionStatusPartialCompleted,
		Outcome: OrchestratorExecutionOutcome{
			Summary:         "partial completion",
			FailureReason:   "one task failed",
			FailureCategory: "worker_failed",
		},
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution create failed: %v", err)
	}
	if created.ID != "exec-1" || created.Goal != "plan and run" || created.IdempotencyKey != "idem-1" {
		t.Fatalf("unexpected normalized execution: %+v", created)
	}
	if created.ParentExecutionID != "parent-0" || created.SourceExecutionID != "source-0" || created.LaunchReason != "rerun_execution" {
		t.Fatalf("unexpected execution lineage: %+v", created)
	}
	if created.Status != OrchestratorExecutionStatusPartialCompleted {
		t.Fatalf("expected partial_completed status, got %q", created.Status)
	}
	if created.Outcome.Summary != "partial completion" || created.Outcome.FailureCategory != "worker_failed" {
		t.Fatalf("unexpected execution outcome: %+v", created.Outcome)
	}
	if created.ApprovalScope != "infrastructure_only" {
		t.Fatalf("expected default approvalScope, got %q", created.ApprovalScope)
	}
	if created.CreatedAt == "" || created.UpdatedAt == "" {
		t.Fatalf("expected timestamps set, got %+v", created)
	}
	if created.Results == nil || len(created.Results) != 0 {
		t.Fatalf("expected empty result slice default, got %+v", created.Results)
	}

	got, found, err := getOrchestratorExecution("EXEC-1")
	if err != nil || !found {
		t.Fatalf("getOrchestratorExecution failed found=%v err=%v got=%+v", found, err, got)
	}
	if got.ID != created.ID {
		t.Fatalf("expected fetched id %q, got %q", created.ID, got.ID)
	}

	if _, found, err := findOrchestratorExecutionByIdempotencyKey(" "); err != nil || found {
		t.Fatalf("empty idempotency lookup should miss found=%v err=%v", found, err)
	}
	byKey, found, err := findOrchestratorExecutionByIdempotencyKey("IDEM-1")
	if err != nil || !found {
		t.Fatalf("findOrchestratorExecutionByIdempotencyKey failed found=%v err=%v", found, err)
	}
	if byKey.ID != created.ID {
		t.Fatalf("expected idempotency hit %q, got %q", created.ID, byKey.ID)
	}

	createdAt := created.CreatedAt
	updated, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:                "exec-1",
		Goal:              "updated goal",
		IdempotencyKey:    "idem-1",
		ParentExecutionID: "parent-0",
		SourceExecutionID: "source-0",
		LaunchReason:      "retry_failed_tasks",
		Status:            OrchestratorExecutionStatusRetryableFailed,
		Outcome: OrchestratorExecutionOutcome{
			Summary:         "retryable failure",
			FailureReason:   "provider timeout",
			FailureCategory: "provider_failed",
		},
		CreatedAt: "should-be-overwritten",
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution update failed: %v", err)
	}
	if updated.CreatedAt != createdAt {
		t.Fatalf("expected createdAt to be preserved, got %q want %q", updated.CreatedAt, createdAt)
	}
	if updated.Goal != "updated goal" || updated.Status != OrchestratorExecutionStatusRetryableFailed {
		t.Fatalf("unexpected updated execution: %+v", updated)
	}
	if updated.LaunchReason != "retry_failed_tasks" || updated.Outcome.FailureCategory != "provider_failed" {
		t.Fatalf("unexpected updated execution lineage/outcome: %+v", updated)
	}

	executions, err = listOrchestratorExecutions()
	if err != nil {
		t.Fatalf("listOrchestratorExecutions after update failed: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected one execution, got %d", len(executions))
	}
	executions[0].Goal = "mutated-locally"
	again, err := listOrchestratorExecutions()
	if err != nil {
		t.Fatalf("listOrchestratorExecutions second read failed: %v", err)
	}
	if again[0].Goal == "mutated-locally" {
		t.Fatalf("expected listOrchestratorExecutions to return copied slice")
	}
}

func TestOrchestratorWorkerLeaseStoreLifecycle(t *testing.T) {
	t.Setenv("CARRIER_REMOTE_CONTROL_STORE", filepath.Join(t.TempDir(), "remote-control.json"))

	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		t.Fatalf("listOrchestratorWorkerLeases empty failed: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected empty worker leases, got %d", len(leases))
	}

	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{}); err == nil {
		t.Fatal("expected upsertOrchestratorWorkerLease to fail on missing id")
	}
	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{ID: "lease-1"}); err == nil {
		t.Fatal("expected upsertOrchestratorWorkerLease to fail on missing executionId")
	}
	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{ID: "lease-1", ExecutionID: "exec-1"}); err == nil {
		t.Fatal("expected upsertOrchestratorWorkerLease to fail on missing hostId")
	}
	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{ID: "lease-1", ExecutionID: "exec-1", HostID: "host-1"}); err == nil {
		t.Fatal("expected upsertOrchestratorWorkerLease to fail on missing agentId")
	}

	created, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
		ID:          " lease-1 ",
		ExecutionID: " exec-1 ",
		HostID:      " host-1 ",
		AgentID:     " zeroclaw ",
		LastError:   "  boom  ",
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorWorkerLease create failed: %v", err)
	}
	if created.ID != "lease-1" || created.ExecutionID != "exec-1" || created.HostID != "host-1" || created.AgentID != "zeroclaw" {
		t.Fatalf("unexpected normalized lease: %+v", created)
	}
	if created.State != OrchestratorWorkerStateProvisioning {
		t.Fatalf("expected default lease state provisioning, got %q", created.State)
	}
	if created.CreatedAt == "" || created.UpdatedAt == "" || created.HeartbeatAt == "" || created.LeaseExpireAt == "" {
		t.Fatalf("expected lease timestamps to be set, got %+v", created)
	}
	if created.LastError != "boom" {
		t.Fatalf("expected trimmed lastError, got %q", created.LastError)
	}

	leases, err = listOrchestratorWorkerLeasesByExecution("EXEC-1")
	if err != nil {
		t.Fatalf("listOrchestratorWorkerLeasesByExecution failed: %v", err)
	}
	if len(leases) != 1 {
		t.Fatalf("expected one lease by execution, got %d", len(leases))
	}

	all, err := listOrchestratorWorkerLeases()
	if err != nil || len(all) != 1 {
		t.Fatalf("listOrchestratorWorkerLeases failed len=%d err=%v", len(all), err)
	}

	createdAt := created.CreatedAt
	updated, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
		ID:          "lease-1",
		ExecutionID: "exec-1",
		HostID:      "host-1",
		AgentID:     "zeroclaw",
		State:       OrchestratorWorkerStateReady,
		TaskCount:   3,
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorWorkerLease update failed: %v", err)
	}
	if updated.CreatedAt != createdAt {
		t.Fatalf("expected lease createdAt preserved, got %q want %q", updated.CreatedAt, createdAt)
	}
	if updated.State != OrchestratorWorkerStateReady || updated.TaskCount != 3 {
		t.Fatalf("unexpected updated lease: %+v", updated)
	}

	if err := deleteOrchestratorWorkerLease("LEASE-1"); err != nil {
		t.Fatalf("deleteOrchestratorWorkerLease failed: %v", err)
	}
	if err := deleteOrchestratorWorkerLease("missing-lease"); err != nil {
		t.Fatalf("deleteOrchestratorWorkerLease missing should be idempotent: %v", err)
	}
	all, err = listOrchestratorWorkerLeases()
	if err != nil {
		t.Fatalf("listOrchestratorWorkerLeases after delete failed: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected no leases after delete, got %d", len(all))
	}
}
