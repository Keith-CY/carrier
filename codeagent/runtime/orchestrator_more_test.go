package runtime

import (
	"context"
	"testing"

	"carrier/codeagent/contract"
)

type emptyPolicyAdapter struct{}

func (a *emptyPolicyAdapter) Install(context.Context, contract.Target) error { return nil }
func (a *emptyPolicyAdapter) Configure(context.Context, contract.Target, contract.Profile) error {
	return nil
}
func (a *emptyPolicyAdapter) Run(_ context.Context, _ contract.RunRequest) (contract.ResultEnvelope, error) {
	return contract.ResultEnvelope{Ok: true, PolicyDecision: ""}, nil
}
func (a *emptyPolicyAdapter) Health(context.Context) error { return nil }
func (a *emptyPolicyAdapter) Version(context.Context) (string, error) { return "v", nil }
func (a *emptyPolicyAdapter) Supports(contract.Capability) bool { return true }

func TestOrchestratorEmptyDecisionDefaultsAndAskBranch(t *testing.T) {
	t.Parallel()

	adapter := &emptyPolicyAdapter{}
	orch := NewOrchestrator(adapter, []Middleware{
		func(_ context.Context, _ contract.RunRequest) contract.PolicyDecisionEnvelope {
			return contract.PolicyDecisionEnvelope{}
		},
	})
	out, err := orch.Run(context.Background(), contract.RunRequest{Capability: contract.CapabilityReadFile})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out.PolicyDecision != contract.PolicyDecisionAllow {
		t.Fatalf("expected orchestrator to default empty policy decision to allow, got %+v", out)
	}

	askOrch := NewOrchestrator(adapter, []Middleware{
		func(_ context.Context, _ contract.RunRequest) contract.PolicyDecisionEnvelope {
			return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAsk, Reason: "confirm"}
		},
	})
	askOut, askErr := askOrch.Run(context.Background(), contract.RunRequest{Capability: contract.CapabilityRunShell, Command: "curl"})
	if askErr != nil {
		t.Fatalf("Run returned error: %v", askErr)
	}
	if askOut.Ok {
		t.Fatalf("expected ok=false on ask decision, got %+v", askOut)
	}
	if askOut.PolicyDecision != contract.PolicyDecisionAsk || askOut.PolicyReason != "confirm" {
		t.Fatalf("unexpected ask decision payload: %+v", askOut)
	}
}
