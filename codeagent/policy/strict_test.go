package policy

import (
	"testing"

	"carrier/codeagent/contract"
)

func TestStrictPolicyDenyPathEscape(t *testing.T) {
	t.Parallel()

	p := NewStrictPolicy("/workspace")
	decision := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityWriteFile,
		Path:       "/workspace/../etc/passwd",
	})
	if decision.Action != contract.PolicyDecisionDeny {
		t.Fatalf("expected deny decision, got %+v", decision)
	}
}

func TestStrictPolicyCommandClassification(t *testing.T) {
	t.Parallel()

	p := NewStrictPolicy("/workspace")

	deny := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityRunShell,
		Command:    "rm -rf /",
	})
	if deny.Action != contract.PolicyDecisionDeny {
		t.Fatalf("expected deny for destructive command, got %+v", deny)
	}

	ask := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityRunShell,
		Command:    "curl -I https://example.com",
	})
	if ask.Action != contract.PolicyDecisionAsk {
		t.Fatalf("expected ask for network command, got %+v", ask)
	}

	allow := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityRunShell,
		Command:    "ls -la",
	})
	if allow.Action != contract.PolicyDecisionAllow {
		t.Fatalf("expected allow for safe command, got %+v", allow)
	}
}
