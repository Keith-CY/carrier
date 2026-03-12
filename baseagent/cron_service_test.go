package baseagent

import (
	"context"
	"testing"
	"time"
)

func TestCronJobReentersStructuredLoopWithSessionContext(t *testing.T) {
	provider := &scriptedToolAwareProvider{
		name: "cron-runtime",
		replies: []StructuredToolReply{
			{Content: "cron run complete"},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	job, err := rt.ScheduleJob(context.Background(), CronJob{
		SessionKey: "cli:cron-check",
		Prompt:     "perform maintenance planning",
	})
	if err != nil {
		t.Fatalf("schedule job: %v", err)
	}
	if job.ID == "" {
		t.Fatalf("expected scheduled cron job id, got %+v", job)
	}

	history := rt.sessions.History("cli:cron-check")
	if len(history) == 0 {
		t.Fatalf("expected cron job to reenter session history, got %+v", history)
	}
}

func TestCronJobRespectsPendingApprovalPolicy(t *testing.T) {
	provider := &scriptedToolAwareProvider{
		name: "cron-pending",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "agent_start",
						Arguments: map[string]any{
							"agent_id": "openclaw",
						},
					},
				},
			},
			{Content: "cron requires confirmation"},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	if _, err := rt.ScheduleJob(context.Background(), CronJob{
		SessionKey: "cli:cron-pending",
		Prompt:     "perform maintenance planning",
	}); err != nil {
		t.Fatalf("schedule job: %v", err)
	}

	pending := rt.sessions.PendingApprovals("cli:cron-pending")
	if len(pending) != 1 || pending[0].ToolName != "agent_start" {
		t.Fatalf("expected cron job to preserve pending approval policy, got %+v", pending)
	}
}

func TestCronServiceListIncludesLastRunState(t *testing.T) {
	var executed int
	svc := NewCronService(func(_ context.Context, job CronJob) error {
		executed++
		return nil
	})

	job, err := svc.Schedule(context.Background(), CronJob{
		ID:         "cron-list-1",
		SessionKey: "agent:picoclaw",
		Prompt:     "summarize heartbeat",
	})
	if err != nil {
		t.Fatalf("schedule job: %v", err)
	}

	jobs := svc.List("agent:picoclaw")
	if len(jobs) != 1 {
		t.Fatalf("jobs=%d want 1", len(jobs))
	}
	if jobs[0].ID != "cron-list-1" || jobs[0].LastRunAt == nil || jobs[0].LastResult != "succeeded" {
		t.Fatalf("unexpected listed job: %+v", jobs[0])
	}
	if jobs[0].NextRunAt.IsZero() {
		t.Fatalf("expected next run at on listed job: %+v", jobs[0])
	}
	if executed != 1 {
		t.Fatalf("executed=%d want 1", executed)
	}
	if job.ID != "cron-list-1" {
		t.Fatalf("scheduled job id=%q want cron-list-1", job.ID)
	}
}

func TestCronServiceCancelMarksJobCancelled(t *testing.T) {
	svc := NewCronService(nil)
	scheduledAt := time.Now().UTC().Add(5 * time.Minute)
	if _, err := svc.Schedule(context.Background(), CronJob{
		ID:         "cron-cancel-1",
		SessionKey: "agent:picoclaw",
		Prompt:     "ping",
		NextRunAt:  scheduledAt,
	}); err != nil {
		t.Fatalf("schedule job: %v", err)
	}

	job, err := svc.Cancel("cron-cancel-1")
	if err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	if job.CancelledAt == nil || job.LastResult != "cancelled" {
		t.Fatalf("unexpected cancelled job: %+v", job)
	}

	jobs := svc.List("agent:picoclaw")
	if len(jobs) != 1 || jobs[0].CancelledAt == nil || jobs[0].LastResult != "cancelled" {
		t.Fatalf("unexpected listed cancelled job: %+v", jobs)
	}
}
