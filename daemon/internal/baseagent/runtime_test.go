package baseagent

import (
	"context"
	"testing"
)

type stubAgentService struct{}

func (s *stubAgentService) ListAgents() []AgentState { return nil }

func (s *stubAgentService) Install(_ context.Context, _ string) error { return nil }

func (s *stubAgentService) Uninstall(_ context.Context, _ string) error { return nil }

func (s *stubAgentService) Start(_ context.Context, _ string) error { return nil }

func (s *stubAgentService) Stop(_ context.Context, _ string) error { return nil }

func (s *stubAgentService) Status(agentID string) (AgentState, error) {
	return AgentState{ID: agentID}, nil
}

func (s *stubAgentService) Logs(_ string, _ int) ([]string, error) { return nil, nil }

func (s *stubAgentService) Upgrade(_ context.Context, agentID string) (UpgradeResult, error) {
	return UpgradeResult{AgentID: agentID}, nil
}

func (s *stubAgentService) Diagnose(agentID string) (string, error) { return agentID + "-diag", nil }

func TestExecuteAgentAction_InstallUnsupported(t *testing.T) {
	svc := &stubAgentService{}
	rt := NewRuntime(svc, nil)

	_, err := rt.executeAgentAction(context.Background(), "install", "openclaw")
	if err == nil {
		t.Fatal("expected unsupported action error for install")
	}
	if err.Error() != "unsupported action: install" {
		t.Fatalf("unexpected error: %v", err)
	}
}
