package baseagent

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSubagentManagerSpawn(t *testing.T) {
	release := make(chan struct{})
	manager := NewInMemorySubagentManager(func(_ context.Context, req SubagentRequest) (string, error) {
		if req.Task == "collect dependency graph" {
			<-release
		}
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
	if first.Status != string(SubagentJobStatusQueued) || second.Status != string(SubagentJobStatusQueued) {
		t.Fatalf("expected queued handles, got first=%+v second=%+v", first, second)
	}

	job, err := manager.Job(context.Background(), first.JobID)
	if err != nil {
		t.Fatalf("lookup first job: %v", err)
	}
	if job.Status == SubagentJobStatusCompleted {
		t.Fatalf("expected async job before release, got %+v", job)
	}

	close(release)
	job = waitForSubagentJobState(t, manager, first.JobID, SubagentJobStatusCompleted)
	if job.Result != "completed: collect dependency graph" {
		t.Fatalf("unexpected stored job result: %+v", job)
	}

	secondJob := waitForSubagentJobState(t, manager, second.JobID, SubagentJobStatusCompleted)
	if secondJob.Status != SubagentJobStatusCompleted {
		t.Fatalf("expected completed second job status, got %+v", secondJob)
	}
	if secondJob.Result != "completed: summarize providers" {
		t.Fatalf("unexpected second stored job result: %+v", secondJob)
	}
}

func TestSubagentManagerCancelLifecycle(t *testing.T) {
	manager := NewInMemorySubagentManager(func(ctx context.Context, _ SubagentRequest) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	handle, err := manager.Spawn(context.Background(), SubagentRequest{Task: "watch logs"})
	if err != nil {
		t.Fatalf("spawn job: %v", err)
	}

	if err := manager.Cancel(context.Background(), handle.JobID); err != nil {
		t.Fatalf("cancel job: %v", err)
	}

	job := waitForSubagentJobState(t, manager, handle.JobID, SubagentJobStatusCancelled)
	if job.Error == "" {
		t.Fatalf("expected cancelled job error message, got %+v", job)
	}

	polled, err := manager.Job(context.Background(), handle.JobID)
	if err != nil {
		t.Fatalf("lookup cancelled job: %v", err)
	}
	if polled.Status != SubagentJobStatusCancelled {
		t.Fatalf("expected stable cancelled job status, got %+v", polled)
	}
}

func TestSubagentManagerRecentJobsRetainsBoundedTerminalHistory(t *testing.T) {
	releases := map[string]chan struct{}{
		"first":  make(chan struct{}),
		"second": make(chan struct{}),
		"third":  make(chan struct{}),
	}
	manager := NewInMemorySubagentManager(func(_ context.Context, req SubagentRequest) (string, error) {
		if release, ok := releases[req.Task]; ok {
			<-release
		}
		return "completed: " + req.Task, nil
	})
	manager.maxHistory = 2

	first, err := manager.Spawn(context.Background(), SubagentRequest{Task: "first"})
	if err != nil {
		t.Fatalf("spawn first job: %v", err)
	}
	close(releases["first"])
	waitForSubagentJobState(t, manager, first.JobID, SubagentJobStatusCompleted)

	second, err := manager.Spawn(context.Background(), SubagentRequest{Task: "second"})
	if err != nil {
		t.Fatalf("spawn second job: %v", err)
	}
	close(releases["second"])
	waitForSubagentJobState(t, manager, second.JobID, SubagentJobStatusCompleted)

	third, err := manager.Spawn(context.Background(), SubagentRequest{Task: "third"})
	if err != nil {
		t.Fatalf("spawn third job: %v", err)
	}
	close(releases["third"])
	waitForSubagentJobState(t, manager, third.JobID, SubagentJobStatusCompleted)

	jobs := manager.RecentJobs(context.Background(), 10)
	if len(jobs) != 2 {
		t.Fatalf("expected bounded recent jobs, got %+v", jobs)
	}
	if jobs[0].JobID != third.JobID || jobs[1].JobID != second.JobID {
		t.Fatalf("expected newest-first recent jobs, got %+v", jobs)
	}

	if _, err := manager.Job(context.Background(), first.JobID); err == nil {
		t.Fatalf("expected oldest terminal job to be pruned from lookup")
	}
}

func TestSubagentManagerPersistsJobsAcrossRestart(t *testing.T) {
	storagePath := filepath.Join(t.TempDir(), "subagents.json")
	manager := NewInMemorySubagentManagerWithStorage(func(_ context.Context, req SubagentRequest) (string, error) {
		return "completed: " + req.Task, nil
	}, storagePath)

	handle, err := manager.Spawn(context.Background(), SubagentRequest{Task: "collect diagnostics"})
	if err != nil {
		t.Fatalf("spawn job: %v", err)
	}
	job := waitForSubagentJobState(t, manager, handle.JobID, SubagentJobStatusCompleted)
	if job.Result == "" {
		t.Fatalf("expected completed result before reload, got %+v", job)
	}

	reloaded := NewInMemorySubagentManagerWithStorage(func(_ context.Context, req SubagentRequest) (string, error) {
		return "completed: " + req.Task, nil
	}, storagePath)
	persisted, err := reloaded.Job(context.Background(), handle.JobID)
	if err != nil {
		t.Fatalf("lookup persisted job: %v", err)
	}
	if persisted.Status != SubagentJobStatusCompleted || persisted.Result != "completed: collect diagnostics" {
		t.Fatalf("unexpected persisted job: %+v", persisted)
	}
	jobs := reloaded.RecentJobs(context.Background(), 5)
	if len(jobs) != 1 || jobs[0].JobID != handle.JobID {
		t.Fatalf("unexpected persisted recent jobs: %+v", jobs)
	}
}

func waitForSubagentJobState(t *testing.T, manager *InMemorySubagentManager, jobID string, want SubagentJobStatus) SubagentJob {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.Job(context.Background(), jobID)
		if err == nil && job.Status == want {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	job, err := manager.Job(context.Background(), jobID)
	if err != nil {
		t.Fatalf("lookup job %s: %v", jobID, err)
	}
	if job.Status != want {
		t.Fatalf("expected completed job status, got %+v", job)
	}
	return job
}
