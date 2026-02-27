package runtime

import (
	"context"
	"testing"

	"carrier/codeagent/contract"
)

type fakeAdapter struct {
	called bool
}

func (f *fakeAdapter) Install(context.Context, contract.Target) error {
	return nil
}

func (f *fakeAdapter) Configure(context.Context, contract.Target, contract.Profile) error {
	return nil
}

func (f *fakeAdapter) Run(_ context.Context, req contract.RunRequest) (contract.ResultEnvelope, error) {
	f.called = true
	return contract.ResultEnvelope{
		Ok:             true,
		PolicyDecision: contract.PolicyDecisionAllow,
		FilesTouched:   []string{req.Path},
	}, nil
}

func (f *fakeAdapter) Health(context.Context) error {
	return nil
}

func (f *fakeAdapter) Version(context.Context) (string, error) {
	return "test", nil
}

func (f *fakeAdapter) Supports(contract.Capability) bool {
	return true
}

func TestOrchestratorRunsMiddlewareInOrder(t *testing.T) {
	t.Parallel()

	adapter := &fakeAdapter{}
	order := []string{}
	orch := NewOrchestrator(adapter, []Middleware{
		func(_ context.Context, _ contract.RunRequest) contract.PolicyDecisionEnvelope {
			order = append(order, "security")
			return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
		},
		func(_ context.Context, _ contract.RunRequest) contract.PolicyDecisionEnvelope {
			order = append(order, "authz")
			return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
		},
		func(_ context.Context, _ contract.RunRequest) contract.PolicyDecisionEnvelope {
			order = append(order, "rate")
			return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionAllow}
		},
	})

	_, err := orch.Run(context.Background(), contract.RunRequest{
		Capability: contract.CapabilityReadFile,
		Path:       "/workspace/README.md",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !adapter.called {
		t.Fatalf("expected adapter to be called")
	}
	want := []string{"security", "authz", "rate"}
	if len(order) != len(want) {
		t.Fatalf("middleware execution count mismatch: got=%v want=%v", order, want)
	}
	for idx := range want {
		if order[idx] != want[idx] {
			t.Fatalf("middleware order mismatch: got=%v want=%v", order, want)
		}
	}
}

func TestOrchestratorStopsOnDeny(t *testing.T) {
	t.Parallel()

	adapter := &fakeAdapter{}
	orch := NewOrchestrator(adapter, []Middleware{
		func(_ context.Context, _ contract.RunRequest) contract.PolicyDecisionEnvelope {
			return contract.PolicyDecisionEnvelope{Action: contract.PolicyDecisionDeny, Reason: "blocked"}
		},
	})

	out, err := orch.Run(context.Background(), contract.RunRequest{
		Capability: contract.CapabilityRunShell,
		Command:    "rm -rf /",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out.Ok {
		t.Fatalf("expected ok=false for deny decision")
	}
	if out.PolicyDecision != contract.PolicyDecisionDeny {
		t.Fatalf("expected deny policy decision, got %+v", out)
	}
	if adapter.called {
		t.Fatalf("adapter should not be called when decision is deny")
	}
}
