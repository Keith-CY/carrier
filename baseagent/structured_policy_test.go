package baseagent

import "testing"

func TestStructuredPolicyClassifiesExecCommandsDifferently(t *testing.T) {
	allow := evaluateStructuredToolPolicy("exec", map[string]any{"command": "go test ./..."}, structuredToolDecisionAsk)
	deny := evaluateStructuredToolPolicy("exec", map[string]any{"command": "rm -rf /"}, structuredToolDecisionAsk)

	if allow.Decision != structuredToolDecisionAsk || allow.RuleID == "" || allow.Reason == "" {
		t.Fatalf("expected bounded test command to require confirmation with audit metadata, got %+v", allow)
	}
	if deny.Decision != structuredToolDecisionDeny || deny.RuleID == "" || deny.Reason == "" {
		t.Fatalf("expected dangerous command deny with audit metadata, got %+v", deny)
	}
}
