package baseagent

import (
	"testing"
	"time"
)

func TestSessionManagerStoresMultiplePendingApprovalsWithExpiry(t *testing.T) {
	sm := NewSessionManager(8)
	now := time.Now().UTC()
	sm.SetPendingApprovals("cli:multi", []*PendingToolApproval{
		{ID: "a1", ToolName: "exec", RequestedAt: now, ExpiresAt: now.Add(5 * time.Minute)},
		{ID: "a2", ToolName: "agent_start", RequestedAt: now, ExpiresAt: now.Add(5 * time.Minute)},
	})

	got := sm.PendingApprovals("cli:multi")
	if len(got) != 2 {
		t.Fatalf("expected 2 pending approvals, got %+v", got)
	}
}

func TestSessionManagerPrunesExpiredPendingApprovalsIntoAudit(t *testing.T) {
	sm := NewSessionManager(8)
	now := time.Now().UTC()
	sm.SetPendingApprovals("cli:expiry", []*PendingToolApproval{
		{ID: "expired", ToolName: "exec", RequestedAt: now.Add(-10 * time.Minute), ExpiresAt: now.Add(-time.Minute), Reason: "Shell execution requires confirmation.", RuleID: structuredPolicyRuleExecConfirmationNeeded},
		{ID: "live", ToolName: "agent_start", RequestedAt: now, ExpiresAt: now.Add(5 * time.Minute)},
	})

	got := sm.PendingApprovals("cli:expiry")
	if len(got) != 1 || got[0].ID != "live" {
		t.Fatalf("expected only live approval to remain pending, got %+v", got)
	}

	audit := sm.ApprovalAudit("cli:expiry")
	if len(audit) != 1 || audit[0].ID != "expired" || audit[0].Decision != approvalDecisionExpired {
		t.Fatalf("expected expired approval to be recorded in audit, got %+v", audit)
	}
}
