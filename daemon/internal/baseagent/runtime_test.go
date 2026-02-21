package baseagent

import (
	"context"
	"strings"
	"testing"
)

type stubAgentService struct {
	installCalls []string
}

func (s *stubAgentService) ListAgents() []AgentState { return nil }

func (s *stubAgentService) Install(_ context.Context, agentID string) error {
	s.installCalls = append(s.installCalls, agentID)
	return nil
}

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

func TestExecuteAgentAction_InstallPicoclawRequiresGUI(t *testing.T) {
	svc := &stubAgentService{}
	rt := NewRuntime(svc, nil)

	resp, err := rt.executeAgentAction(context.Background(), "install", "picoclaw")
	if err != nil {
		t.Fatalf("executeAgentAction returned error: %v", err)
	}
	if !strings.Contains(resp.Message, "Carrier GUI") {
		t.Fatalf("expected GUI guidance, got: %q", resp.Message)
	}
	if len(svc.installCalls) != 0 {
		t.Fatalf("expected no install calls, got %v", svc.installCalls)
	}
}

func TestExecuteAgentAction_InstallOtherAgentRequiresGUI(t *testing.T) {
	svc := &stubAgentService{}
	rt := NewRuntime(svc, nil)

	resp, err := rt.executeAgentAction(context.Background(), "install", "openclaw")
	if err != nil {
		t.Fatalf("executeAgentAction returned error: %v", err)
	}
	if !strings.Contains(resp.Message, "disabled in chat") || !strings.Contains(resp.Message, "Carrier GUI") {
		t.Fatalf("expected GUI-only install guidance, got: %q", resp.Message)
	}
	if len(svc.installCalls) != 0 {
		t.Fatalf("expected no install call for openclaw, got %v", svc.installCalls)
	}
}
