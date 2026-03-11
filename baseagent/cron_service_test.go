package baseagent

import (
	"context"
	"testing"
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
