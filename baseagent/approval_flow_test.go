package baseagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func newApprovalRuntimeForTest(t *testing.T) *Runtime {
	t.Helper()

	provider := &scriptedToolAwareProvider{
		name: "approval-runtime",
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
			{
				Content: "please confirm the pending start",
			},
			{
				Content: "confirmed and started openclaw",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}
	return rt
}

func TestRuntimeRespondPendingApprovalConfirmsSpecificApproval(t *testing.T) {
	rt := newApprovalRuntimeForTest(t)
	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "approval-api",
		Message:  "perform maintenance planning",
	})

	pending := rt.sessions.PendingApprovals("cli:approval-api")
	if len(pending) != 1 {
		t.Fatalf("expected exactly one pending approval, got %+v", pending)
	}
	resp, err := rt.RespondPendingApproval(context.Background(), "cli:approval-api", pending[0].ID, ApprovalDecisionConfirm)

	if err != nil || resp.Action != "approval_confirm" {
		t.Fatalf("expected approval confirm path, got resp=%+v err=%v", resp, err)
	}
}

func TestRuntimeRespondPendingApprovalRejectsSpecificApproval(t *testing.T) {
	rt := newApprovalRuntimeForTest(t)
	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "approval-reject",
		Message:  "perform maintenance planning",
	})

	pending := rt.sessions.PendingApprovals("cli:approval-reject")
	if len(pending) != 1 {
		t.Fatalf("expected exactly one pending approval, got %+v", pending)
	}

	resp, err := rt.RespondPendingApproval(context.Background(), "cli:approval-reject", pending[0].ID, ApprovalDecisionReject)
	if err != nil {
		t.Fatalf("reject pending approval: %v", err)
	}
	if resp.Action != "approval_cancel" {
		t.Fatalf("expected approval_cancel action, got %+v", resp)
	}
	if got := rt.sessions.PendingApproval("cli:approval-reject"); got != nil {
		t.Fatalf("expected pending approval to be cleared, got %+v", got)
	}
}

func TestRuntimeRespondPendingApprovalRejectsInvalidDecision(t *testing.T) {
	rt := newApprovalRuntimeForTest(t)
	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "approval-invalid",
		Message:  "perform maintenance planning",
	})

	pending := rt.sessions.PendingApprovals("cli:approval-invalid")
	if len(pending) != 1 {
		t.Fatalf("expected exactly one pending approval, got %+v", pending)
	}

	_, err := rt.RespondPendingApproval(context.Background(), "cli:approval-invalid", pending[0].ID, ApprovalDecision("later"))
	if !errors.Is(err, ErrInvalidApprovalDecision) {
		t.Fatalf("expected invalid decision error, got %v", err)
	}
}

func TestRuntimeRespondPendingApprovalFailsWhenApprovalMissing(t *testing.T) {
	rt := newApprovalRuntimeForTest(t)
	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "approval-missing",
		Message:  "perform maintenance planning",
	})

	_, err := rt.RespondPendingApproval(context.Background(), "cli:approval-missing", "approval-missing", ApprovalDecisionConfirm)
	if !errors.Is(err, ErrPendingApprovalNotFound) {
		t.Fatalf("expected pending approval not found, got %v", err)
	}
}

func TestSessionManagerConsumePendingApprovalOnlySucceedsOnce(t *testing.T) {
	sm := NewSessionManager(8)
	sm.SetPendingApproval("cli:atomic", &PendingToolApproval{
		ID:       "approval-1",
		ToolName: "agent_start",
	})

	var wg sync.WaitGroup
	results := make(chan *PendingToolApproval, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pending, _ := sm.ConsumePendingApproval("cli:atomic", "approval-1")
			results <- pending
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	for pending := range results {
		if pending != nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful consume, got %d", successes)
	}
	if got := sm.PendingApproval("cli:atomic"); got != nil {
		t.Fatalf("expected pending approval to be cleared after consume, got %+v", got)
	}
}

func TestRuntimeRejectsExpiredApproval(t *testing.T) {
	rt := newApprovalRuntimeForTest(t)
	now := time.Now().UTC()
	rt.sessions.SetPendingApprovals("cli:expired", []*PendingToolApproval{
		{
			ID:          "approval-expired",
			ToolName:    "agent_start",
			RequestedAt: now.Add(-10 * time.Minute),
			ExpiresAt:   now.Add(-time.Minute),
			Reason:      "Agent lifecycle mutation requires confirmation.",
			RuleID:      structuredPolicyRuleAgentConfirmation,
		},
	})

	_, err := rt.RespondPendingApproval(context.Background(), "cli:expired", "approval-expired", ApprovalDecisionConfirm)
	if !errors.Is(err, ErrPendingApprovalNotFound) {
		t.Fatalf("expected missing approval error for expired approval, got %v", err)
	}

	audit := rt.sessions.ApprovalAudit("cli:expired")
	if len(audit) != 1 || audit[0].Decision != approvalDecisionExpired {
		t.Fatalf("expected expired approval audit entry, got %+v", audit)
	}
}
