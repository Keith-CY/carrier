package baseagent

import (
	"context"
	"testing"
)

func TestSubagentManagerSpawn(t *testing.T) {
	manager := NewInMemorySubagentManager(func(_ context.Context, req SubagentRequest) (string, error) {
		return "completed: " + req.Task, nil
	})

	first, err := manager.Spawn(context.Background(), SubagentRequest{Task: "collect dependency graph"})
	if err != nil {
		t.Fatalf("spawn first job: %v", err)
	}
	second, err := manager.Spawn(context.Background(), SubagentRequest{Task: "summarize providers"})
	if err != nil {
		t.Fatalf("spawn second job: %v", err)
	}

	if first.JobID != "subagent-1" || second.JobID != "subagent-2" {
		t.Fatalf("expected stable incrementing job ids, got first=%+v second=%+v", first, second)
	}

	job, err := manager.Job(context.Background(), first.JobID)
	if err != nil {
		t.Fatalf("lookup first job: %v", err)
	}
	if job.Status != SubagentJobStatusCompleted {
		t.Fatalf("expected completed job status, got %+v", job)
	}
	if job.Result != "completed: collect dependency graph" {
		t.Fatalf("unexpected stored job result: %+v", job)
	}
}
