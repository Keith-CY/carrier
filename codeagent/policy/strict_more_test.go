package policy

import (
	"testing"

	"carrier/codeagent/contract"
)

func TestStrictPolicyAdditionalBranches(t *testing.T) {
	t.Parallel()

	pNoRoot := NewStrictPolicy(".")
	if got := pNoRoot.Decide(contract.RunRequest{Capability: contract.CapabilityReadFile, Path: "/etc/passwd"}); got.Action != contract.PolicyDecisionAllow {
		t.Fatalf("expected allow when workspace root is '.', got %+v", got)
	}

	p := NewStrictPolicy("/workspace")
	if got := p.Decide(contract.RunRequest{Capability: contract.CapabilityReadFile, Path: ""}); got.Action != contract.PolicyDecisionAllow {
		t.Fatalf("expected allow for empty path, got %+v", got)
	}
	if got := p.Decide(contract.RunRequest{Capability: contract.CapabilityReadFile, Path: "sub/file.txt"}); got.Action != contract.PolicyDecisionAllow {
		t.Fatalf("expected allow for relative path in workspace, got %+v", got)
	}
	if got := p.Decide(contract.RunRequest{Capability: contract.CapabilityRunShell, Command: "   "}); got.Action != contract.PolicyDecisionDeny {
		t.Fatalf("expected deny for empty shell command, got %+v", got)
	}

	if got := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityRunShellRedirect,
		Command:    "curl -I https://example.com",
		StdoutPath: "/workspace/out.log",
		StderrPath: "/workspace/err.log",
	}); got.Action != contract.PolicyDecisionAsk {
		t.Fatalf("expected ask for network redirect command, got %+v", got)
	}

	if got := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityRunShellRedirect,
		Command:    "ls -la",
		StdoutPath: "/workspace/../etc/out.log",
		StderrPath: "/workspace/err.log",
	}); got.Action != contract.PolicyDecisionDeny {
		t.Fatalf("expected deny for stdout path escape, got %+v", got)
	}

	if got := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityRunShellRedirect,
		Command:    "ls -la",
		StdoutPath: "/workspace/out.log",
		StderrPath: "/workspace/err.log",
	}); got.Action != contract.PolicyDecisionAllow {
		t.Fatalf("expected allow for safe redirect command/paths, got %+v", got)
	}

	if got := p.Decide(contract.RunRequest{
		Capability: contract.CapabilityRunShellRedirect,
		Command:    "ls -la",
		StdoutPath: "/workspace/out.log",
		StderrPath: "/workspace/../etc/err.log",
	}); got.Action != contract.PolicyDecisionDeny {
		t.Fatalf("expected deny for stderr path escape, got %+v", got)
	}

	if got := p.Decide(contract.RunRequest{Capability: contract.Capability("other")}); got.Action != contract.PolicyDecisionAllow {
		t.Fatalf("expected allow for unknown capability, got %+v", got)
	}

	if !containsAny("ssh host", []string{"ssh "}) {
		t.Fatalf("expected containsAny true")
	}
	if containsAny("echo hi", []string{"ssh "}) {
		t.Fatalf("expected containsAny false")
	}
}
