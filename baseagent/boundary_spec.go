package baseagent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	boundarySpecSchemaV1 = "carrier.baseagent.boundary.v1"

	chatPolicyDisabled            = "disabled"
	chatPolicyEnabled             = "enabled"
	chatPolicyRequiresHostBinding = "requires_host_binding"

	defaultInstallAutoRepairRoundBudget = 3
)

type CommandPolicies struct {
	ChatInstall                      string `json:"chat_install"`
	ChatOnboard                      string `json:"chat_onboard"`
	RequiresExplicitHostForWorkflows bool   `json:"requires_explicit_host_for_remote_workflows"`
}

type WorkflowPolicy struct {
	Enabled                            bool `json:"enabled"`
	RequiresHostBinding                bool `json:"requires_host_binding"`
	RequiresPreflight                  bool `json:"requires_preflight"`
	MaxAttempts                        int  `json:"max_attempts"`
	AutoEscalateToDiagnose             bool `json:"auto_escalate_to_diagnose"`
	HighRiskActionsRequireConfirmation bool `json:"high_risk_actions_require_confirmation"`
}

type RepairPolicy struct {
	MaxAutoRepairRounds          int      `json:"max_auto_repair_rounds"`
	HighRiskPathPrefixes         []string `json:"high_risk_path_prefixes"`
	BlockedSubstrings            []string `json:"blocked_substrings"`
	HighRiskRequiresConfirmation bool     `json:"high_risk_requires_confirmation"`
}

type StructuredToolPolicySpec struct {
	MetadataReadDecision      string `json:"metadata_read_decision"`
	OperationalReadDecision   string `json:"operational_read_decision"`
	WorkspaceReadDecision     string `json:"workspace_read_decision"`
	WorkspaceMutationDecision string `json:"workspace_mutation_decision"`
	HighRiskDecision          string `json:"high_risk_decision"`
}

type BoundarySpec struct {
	SchemaVersion        string                    `json:"schema_version"`
	AssistantRole        string                    `json:"assistant_role"`
	InScope              []string                  `json:"in_scope"`
	OutOfScope           []string                  `json:"out_of_scope"`
	BoundarySources      []string                  `json:"boundary_sources"`
	DesignPrinciples     []string                  `json:"design_principles"`
	StructuredToolPolicy StructuredToolPolicySpec  `json:"structured_tool_policy"`
	CommandPolicies      CommandPolicies           `json:"command_policies"`
	WorkflowPolicies     map[string]WorkflowPolicy `json:"workflow_policies"`
	RepairPolicy         RepairPolicy              `json:"repair_policy"`
}

//go:embed spec/baseagent-boundary.v1.json
var embeddedBoundarySpecRaw []byte

var (
	activeBoundarySpecOnce sync.Once
	activeBoundarySpec     BoundarySpec
	activeBoundarySpecErr  error
)

func ActiveBoundarySpec() BoundarySpec {
	activeBoundarySpecOnce.Do(func() {
		activeBoundarySpec, activeBoundarySpecErr = ParseBoundarySpec(embeddedBoundarySpecRaw)
	})
	if activeBoundarySpecErr != nil {
		return fallbackBoundarySpec()
	}
	return activeBoundarySpec
}

func ParseBoundarySpec(raw []byte) (BoundarySpec, error) {
	var spec BoundarySpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return BoundarySpec{}, fmt.Errorf("parse boundary spec: %w", err)
	}
	if err := ValidateBoundarySpec(spec); err != nil {
		return BoundarySpec{}, err
	}
	return spec, nil
}

func ValidateBoundarySpec(spec BoundarySpec) error {
	if strings.TrimSpace(spec.SchemaVersion) != boundarySpecSchemaV1 {
		return fmt.Errorf("invalid schema_version %q", strings.TrimSpace(spec.SchemaVersion))
	}
	if strings.TrimSpace(spec.AssistantRole) == "" {
		return fmt.Errorf("assistant_role is required")
	}
	if len(spec.InScope) == 0 {
		return fmt.Errorf("in_scope must not be empty")
	}
	if len(spec.OutOfScope) == 0 {
		return fmt.Errorf("out_of_scope must not be empty")
	}
	if len(spec.BoundarySources) == 0 {
		return fmt.Errorf("boundary_sources must not be empty")
	}
	if len(spec.DesignPrinciples) == 0 {
		return fmt.Errorf("design_principles must not be empty")
	}
	if err := ValidateStructuredToolPolicySpec(spec.StructuredToolPolicy); err != nil {
		return err
	}
	if !isValidChatPolicyMode(spec.CommandPolicies.ChatInstall) {
		return fmt.Errorf("command_policies.chat_install has invalid mode %q", spec.CommandPolicies.ChatInstall)
	}
	if !isValidChatPolicyMode(spec.CommandPolicies.ChatOnboard) {
		return fmt.Errorf("command_policies.chat_onboard has invalid mode %q", spec.CommandPolicies.ChatOnboard)
	}
	if len(spec.WorkflowPolicies) == 0 {
		return fmt.Errorf("workflow_policies must not be empty")
	}
	for name, wf := range spec.WorkflowPolicies {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("workflow_policies contains empty key")
		}
		if wf.MaxAttempts <= 0 {
			return fmt.Errorf("workflow %q max_attempts must be > 0", name)
		}
	}
	if spec.RepairPolicy.MaxAutoRepairRounds <= 0 {
		return fmt.Errorf("repair_policy.max_auto_repair_rounds must be > 0")
	}
	if len(spec.RepairPolicy.HighRiskPathPrefixes) == 0 {
		return fmt.Errorf("repair_policy.high_risk_path_prefixes must not be empty")
	}
	return nil
}

func InstallAutoRepairRoundBudget() int {
	return ActiveBoundarySpec().RepairRoundBudget()
}

func (s BoundarySpec) RepairRoundBudget() int {
	if s.RepairPolicy.MaxAutoRepairRounds <= 0 {
		return defaultInstallAutoRepairRoundBudget
	}
	return s.RepairPolicy.MaxAutoRepairRounds
}

func (s BoundarySpec) RenderSummary() string {
	spec := s
	if err := ValidateBoundarySpec(spec); err != nil {
		spec = fallbackBoundarySpec()
	}

	lines := []string{
		"BaseAgent boundaries:",
		"Role: " + spec.AssistantRole,
		"In scope:",
	}
	lines = append(lines, prefixLines(spec.InScope)...)
	lines = append(lines, "Out of scope:")
	lines = append(lines, prefixLines(spec.OutOfScope)...)
	lines = append(lines, "Boundary sources:")
	lines = append(lines, prefixLines(spec.BoundarySources)...)
	lines = append(lines, "Design principles:")
	lines = append(lines, prefixLines(spec.DesignPrinciples)...)
	lines = append(lines, "Structured tool policy:")
	lines = append(lines,
		fmt.Sprintf("- metadata_read=%s", spec.StructuredToolPolicy.MetadataReadDecision),
		fmt.Sprintf("- operational_read=%s", spec.StructuredToolPolicy.OperationalReadDecision),
		fmt.Sprintf("- workspace_read=%s", spec.StructuredToolPolicy.WorkspaceReadDecision),
		fmt.Sprintf("- workspace_mutation=%s", spec.StructuredToolPolicy.WorkspaceMutationDecision),
		fmt.Sprintf("- high_risk=%s", spec.StructuredToolPolicy.HighRiskDecision),
	)
	lines = append(lines,
		fmt.Sprintf("Chat install policy: %s", spec.CommandPolicies.ChatInstall),
		fmt.Sprintf("Chat onboard policy: %s", spec.CommandPolicies.ChatOnboard),
		fmt.Sprintf("Remote workflow host binding required: %t", spec.CommandPolicies.RequiresExplicitHostForWorkflows),
		fmt.Sprintf("Install auto-repair round budget: %d", spec.RepairRoundBudget()),
		"Workflow policies:",
	)

	workflowNames := make([]string, 0, len(spec.WorkflowPolicies))
	for name := range spec.WorkflowPolicies {
		workflowNames = append(workflowNames, name)
	}
	sort.Strings(workflowNames)
	for _, name := range workflowNames {
		wf := spec.WorkflowPolicies[name]
		lines = append(lines, fmt.Sprintf(
			"- %s: enabled=%t host_binding=%t preflight=%t max_attempts=%d auto_escalate=%t high_risk_confirm=%t",
			name, wf.Enabled, wf.RequiresHostBinding, wf.RequiresPreflight, wf.MaxAttempts, wf.AutoEscalateToDiagnose, wf.HighRiskActionsRequireConfirmation,
		))
	}

	return strings.Join(lines, "\n")
}

func isValidChatPolicyMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case chatPolicyDisabled, chatPolicyEnabled, chatPolicyRequiresHostBinding:
		return true
	default:
		return false
	}
}

func ValidateStructuredToolPolicySpec(spec StructuredToolPolicySpec) error {
	if !isValidStructuredToolDecisionMode(spec.MetadataReadDecision) {
		return fmt.Errorf("structured_tool_policy.metadata_read_decision has invalid mode %q", spec.MetadataReadDecision)
	}
	if !isValidStructuredToolDecisionMode(spec.OperationalReadDecision) {
		return fmt.Errorf("structured_tool_policy.operational_read_decision has invalid mode %q", spec.OperationalReadDecision)
	}
	if !isValidStructuredToolDecisionMode(spec.WorkspaceReadDecision) {
		return fmt.Errorf("structured_tool_policy.workspace_read_decision has invalid mode %q", spec.WorkspaceReadDecision)
	}
	if !isValidStructuredToolDecisionMode(spec.WorkspaceMutationDecision) {
		return fmt.Errorf("structured_tool_policy.workspace_mutation_decision has invalid mode %q", spec.WorkspaceMutationDecision)
	}
	if !isValidStructuredToolDecisionMode(spec.HighRiskDecision) {
		return fmt.Errorf("structured_tool_policy.high_risk_decision has invalid mode %q", spec.HighRiskDecision)
	}
	return nil
}

func isValidStructuredToolDecisionMode(mode string) bool {
	switch parseStructuredToolDecision(mode) {
	case structuredToolDecisionAllow, structuredToolDecisionAsk, structuredToolDecisionDeny:
		return true
	default:
		return false
	}
}

func fallbackBoundarySpec() BoundarySpec {
	return BoundarySpec{
		SchemaVersion: boundarySpecSchemaV1,
		AssistantRole: "Carrier product-specific assistant for controlled agent maintenance and bounded workspace automation.",
		InScope: []string{
			"Understand user intent and map it to approved install/maintenance workflows.",
			"Run policy-bounded lifecycle operations for agent instances.",
			"Inspect, create, edit, append, and list files inside the configured workspace root.",
			"Run bounded shell commands inside the configured workspace root when a chat task needs local execution.",
			"Collect logs and escalate unresolved failures with diagnose artifacts.",
		},
		OutOfScope: []string{
			"Arbitrary shell or file execution outside the configured workspace root.",
			"Automatic destructive changes without explicit user confirmation.",
		},
		BoundarySources: []string{
			"System/runtime constraints.",
			"Tool allowlists, workspace confinement, and execution safety guards.",
			"Daemon API contracts and workflow checks.",
		},
		DesignPrinciples: []string{
			"Least privilege by default.",
			"Deterministic command surface with workspace confinement.",
			"Auditability for lifecycle and repair operations.",
		},
		StructuredToolPolicy: StructuredToolPolicySpec{
			MetadataReadDecision:      string(structuredToolDecisionAllow),
			OperationalReadDecision:   string(structuredToolDecisionAllow),
			WorkspaceReadDecision:     string(structuredToolDecisionAllow),
			WorkspaceMutationDecision: string(structuredToolDecisionAllow),
			HighRiskDecision:          string(structuredToolDecisionAsk),
		},
		CommandPolicies: CommandPolicies{
			ChatInstall:                      chatPolicyRequiresHostBinding,
			ChatOnboard:                      chatPolicyDisabled,
			RequiresExplicitHostForWorkflows: true,
		},
		WorkflowPolicies: map[string]WorkflowPolicy{
			"install_openclaw_remote_vps": {
				Enabled:                            true,
				RequiresHostBinding:                true,
				RequiresPreflight:                  true,
				MaxAttempts:                        3,
				AutoEscalateToDiagnose:             true,
				HighRiskActionsRequireConfirmation: true,
			},
		},
		RepairPolicy: RepairPolicy{
			MaxAutoRepairRounds:          defaultInstallAutoRepairRoundBudget,
			HighRiskPathPrefixes:         []string{"/etc", "/usr", "/var"},
			BlockedSubstrings:            []string{"rm -rf /", "mkfs", "dd if=", "shutdown", "reboot"},
			HighRiskRequiresConfirmation: true,
		},
	}
}

func prefixLines(items []string) []string {
	if len(items) == 0 {
		return []string{"- (none)"}
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		lines = append(lines, "- "+trimmed)
	}
	if len(lines) == 0 {
		return []string{"- (none)"}
	}
	return lines
}
