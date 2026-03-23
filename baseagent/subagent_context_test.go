package baseagent

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSubagentManagerBlockingContextRequestsDegradeOnPartialResponse(t *testing.T) {
	manager := NewInMemorySubagentManager(func(ctx context.Context, req SubagentRequest) (string, error) {
		resp, err := RequestDelegationContext(ctx, DelegationContextRequest{
			Kind:             DelegationContextKindExternal,
			Question:         "Need external incident timeline",
			Reason:           "Summarize recent outage history",
			Required:         true,
			RequestedSources: []string{"web", "other-agent"},
		})
		if err != nil {
			return "", err
		}
		return "received: " + resp.Summary, nil
	})

	handle, err := manager.Spawn(context.Background(), SubagentRequest{
		Task: "summarize outage history",
		Contract: &DelegationContract{
			MasterAgentID: "master",
			SubagentID:    "worker-1",
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	var requests []DelegationContextRequest
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.Job(context.Background(), handle.JobID)
		if err != nil {
			t.Fatalf("job: %v", err)
		}
		if job.Status == SubagentJobStatusAwaiting {
			requests, err = manager.ContextRequests(context.Background(), handle.JobID)
			if err != nil {
				t.Fatalf("context requests: %v", err)
			}
			if len(requests) == 1 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one context request, got %+v", requests)
	}

	resolved, err := manager.RespondContextRequest(context.Background(), handle.JobID, requests[0].RequestID, DelegationContextResponse{
		Status:             DelegationContextStatusPartial,
		Summary:            "Only partial incident notes available",
		MissingInformation: []string{"full timeline"},
	})
	if err != nil {
		t.Fatalf("respond context request: %v", err)
	}
	if resolved.Status != DelegationContextStatusPartial {
		t.Fatalf("resolved status=%q want partial", resolved.Status)
	}

	job := waitForSubagentJobState(t, manager, handle.JobID, SubagentJobStatusCompleted)
	if job.Outcome == nil || !job.Outcome.Degraded {
		t.Fatalf("expected degraded outcome, got %+v", job.Outcome)
	}
	if len(job.ContextResponses) != 1 || job.ContextResponses[0].Status != DelegationContextStatusPartial {
		t.Fatalf("unexpected context responses: %+v", job.ContextResponses)
	}
	if len(job.MissingContext) != 1 || !strings.Contains(job.MissingContext[0], "Need external incident timeline") {
		t.Fatalf("unexpected missing context: %+v", job.MissingContext)
	}
	if strings.TrimSpace(job.Result) != "received: Only partial incident notes available" {
		t.Fatalf("unexpected job result: %+v", job)
	}
}
