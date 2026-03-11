package baseagent

import (
	"context"
	"strings"
	"testing"
)

func TestActiveBoundarySpec_EmbeddedPolicyIsValid(t *testing.T) {
	spec := ActiveBoundarySpec()

	if err := ValidateBoundarySpec(spec); err != nil {
		t.Fatalf("embedded boundary spec validation failed: %v", err)
	}
	if spec.SchemaVersion != boundarySpecSchemaV1 {
		t.Fatalf("schema_version = %q, want %q", spec.SchemaVersion, boundarySpecSchemaV1)
	}
	if spec.RepairRoundBudget() <= 0 {
		t.Fatalf("repair round budget = %d, want > 0", spec.RepairRoundBudget())
	}
}

func TestBoundarySpec_RenderSummary(t *testing.T) {
	summary := ActiveBoundarySpec().RenderSummary()

	wants := []string{
		"BaseAgent boundaries:",
		"Chat install policy:",
		"Structured tool policy:",
		"Workflow policies:",
		"install_openclaw_remote_vps",
	}
	for _, want := range wants {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q\nsummary:\n%s", want, summary)
		}
	}
}

func TestStructuredToolSurfaceReadsPolicyFromBoundarySpec(t *testing.T) {
	spec := ActiveBoundarySpec()
	spec.StructuredToolPolicy.HighRiskDecision = string(structuredToolDecisionDeny)

	surface := newStructuredToolSurfaceWithPolicy(NewToolRegistry(), NewExecutionToolRegistry(t.TempDir()), nil, spec.StructuredToolPolicy)
	result := surface.Execute(context.Background(), "exec", map[string]any{"command": "go test ./..."})
	if result.Status != ExecutionToolResultStatusDeny {
		t.Fatalf("expected boundary policy to drive deny, got %+v", result)
	}
}

func TestParseBoundarySpec_RejectsInvalidChatInstallMode(t *testing.T) {
	raw := []byte(`{
		"schema_version":"carrier.baseagent.boundary.v1",
		"assistant_role":"x",
		"in_scope":["a"],
		"out_of_scope":["b"],
		"boundary_sources":["c"],
		"design_principles":["d"],
		"structured_tool_policy":{
			"metadata_read_decision":"allow",
			"operational_read_decision":"allow",
			"workspace_read_decision":"allow",
			"workspace_mutation_decision":"allow",
			"high_risk_decision":"ask"
		},
		"command_policies":{"chat_install":"invalid","chat_onboard":"disabled","requires_explicit_host_for_remote_workflows":true},
		"workflow_policies":{"wf":{"enabled":true,"requires_host_binding":true,"requires_preflight":true,"max_attempts":1,"auto_escalate_to_diagnose":true,"high_risk_actions_require_confirmation":true}},
		"repair_policy":{"max_auto_repair_rounds":1,"high_risk_path_prefixes":["/etc"],"blocked_substrings":[],"high_risk_requires_confirmation":true}
	}`)

	_, err := ParseBoundarySpec(raw)
	if err == nil || !strings.Contains(err.Error(), "chat_install") {
		t.Fatalf("expected chat_install validation error, got %v", err)
	}
}
