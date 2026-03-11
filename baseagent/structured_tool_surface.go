package baseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type structuredToolTier string
type structuredToolDecision string

const (
	structuredToolTierMetadataRead      structuredToolTier = "metadata_read"
	structuredToolTierOperationalRead   structuredToolTier = "operational_read"
	structuredToolTierWorkspaceRead     structuredToolTier = "workspace_read"
	structuredToolTierWorkspaceMutation structuredToolTier = "workspace_mutation"
	structuredToolTierHighRisk          structuredToolTier = "high_risk"

	structuredToolDecisionAllow structuredToolDecision = "allow"
	structuredToolDecisionAsk   structuredToolDecision = "ask"
	structuredToolDecisionDeny  structuredToolDecision = "deny"
)

type structuredToolPolicy struct {
	decisions map[structuredToolTier]structuredToolDecision
}

func defaultStructuredToolPolicySpec(hasWorkspace bool) StructuredToolPolicySpec {
	spec := StructuredToolPolicySpec{
		MetadataReadDecision:    string(structuredToolDecisionAllow),
		OperationalReadDecision: string(structuredToolDecisionAllow),
		HighRiskDecision:        string(structuredToolDecisionAsk),
	}
	if hasWorkspace {
		spec.WorkspaceReadDecision = string(structuredToolDecisionAllow)
		spec.WorkspaceMutationDecision = string(structuredToolDecisionAllow)
	} else {
		spec.WorkspaceReadDecision = string(structuredToolDecisionDeny)
		spec.WorkspaceMutationDecision = string(structuredToolDecisionDeny)
	}
	return spec
}

func parseStructuredToolDecision(raw string) structuredToolDecision {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(structuredToolDecisionAllow):
		return structuredToolDecisionAllow
	case string(structuredToolDecisionAsk):
		return structuredToolDecisionAsk
	case string(structuredToolDecisionDeny):
		return structuredToolDecisionDeny
	default:
		return ""
	}
}

func structuredToolPolicyFromSpec(spec StructuredToolPolicySpec, hasWorkspace bool) structuredToolPolicy {
	resolved := defaultStructuredToolPolicySpec(hasWorkspace)
	if decision := parseStructuredToolDecision(spec.MetadataReadDecision); decision != "" {
		resolved.MetadataReadDecision = string(decision)
	}
	if decision := parseStructuredToolDecision(spec.OperationalReadDecision); decision != "" {
		resolved.OperationalReadDecision = string(decision)
	}
	if hasWorkspace {
		if decision := parseStructuredToolDecision(spec.WorkspaceReadDecision); decision != "" {
			resolved.WorkspaceReadDecision = string(decision)
		}
		if decision := parseStructuredToolDecision(spec.WorkspaceMutationDecision); decision != "" {
			resolved.WorkspaceMutationDecision = string(decision)
		}
	}
	if decision := parseStructuredToolDecision(spec.HighRiskDecision); decision != "" {
		resolved.HighRiskDecision = string(decision)
	}
	return structuredToolPolicy{
		decisions: map[structuredToolTier]structuredToolDecision{
			structuredToolTierMetadataRead:      parseStructuredToolDecision(resolved.MetadataReadDecision),
			structuredToolTierOperationalRead:   parseStructuredToolDecision(resolved.OperationalReadDecision),
			structuredToolTierWorkspaceRead:     parseStructuredToolDecision(resolved.WorkspaceReadDecision),
			structuredToolTierWorkspaceMutation: parseStructuredToolDecision(resolved.WorkspaceMutationDecision),
			structuredToolTierHighRisk:          parseStructuredToolDecision(resolved.HighRiskDecision),
		},
	}
}

func (p structuredToolPolicy) decision(tier structuredToolTier) structuredToolDecision {
	decision := p.decisions[tier]
	if decision == "" {
		return structuredToolDecisionDeny
	}
	return decision
}

type structuredToolDefinition struct {
	descriptor StructuredToolDescriptor
	tier       structuredToolTier
	execute    func(context.Context, map[string]any) ExecutionToolResult
}

type structuredToolSurface struct {
	policy structuredToolPolicy
	order  []string
	tools  map[string]structuredToolDefinition
}

var structuredBuiltinPassthroughTiers = map[string]structuredToolTier{
	"help":            structuredToolTierMetadataRead,
	"list_agents":     structuredToolTierOperationalRead,
	"list_tools":      structuredToolTierMetadataRead,
	"list_providers":  structuredToolTierMetadataRead,
	"list_sessions":   structuredToolTierMetadataRead,
	"show_boundaries": structuredToolTierMetadataRead,
}

func newStructuredToolSurface(builtin *ToolRegistry, workspace *ExecutionToolRegistry) *structuredToolSurface {
	return newStructuredToolSurfaceWithPolicy(builtin, workspace, nil, nil, StructuredToolPolicySpec{})
}

func newStructuredToolSurfaceWithPolicy(builtin *ToolRegistry, workspace *ExecutionToolRegistry, mcpManager MCPManager, subagentManager SubagentManager, policySpec StructuredToolPolicySpec) *structuredToolSurface {
	// TODO(baseagent): add real confirmation handshakes for ask decisions and
	// introduce argument-level controls.
	hasWorkspace := workspace != nil && workspace.HasWorkspaceRoot()
	surface := &structuredToolSurface{
		policy: structuredToolPolicyFromSpec(policySpec, hasWorkspace),
		order:  []string{},
		tools:  map[string]structuredToolDefinition{},
	}

	surface.registerBuiltinStructuredTools(builtin)
	surface.registerWorkspaceStructuredTools(workspace)
	surface.registerMCPStructuredTools(mcpManager)
	surface.registerSubagentStructuredTools(subagentManager)

	if len(surface.order) == 0 {
		return nil
	}
	return surface
}

func (s *structuredToolSurface) registerBuiltinStructuredTools(builtin *ToolRegistry) {
	if s == nil || builtin == nil {
		return
	}

	for _, descriptor := range builtin.ListStructuredToolDescriptors() {
		tier, ok := structuredBuiltinPassthroughTiers[strings.TrimSpace(descriptor.Name)]
		if !ok {
			continue
		}
		name := strings.TrimSpace(descriptor.Name)
		s.register(structuredToolDefinition{
			descriptor: descriptor,
			tier:       tier,
			execute: func(ctx context.Context, args map[string]any) ExecutionToolResult {
				return executeStructuredBuiltinTool(ctx, builtin, name, name, args)
			},
		})
	}

	s.registerStructuredAgentAction(builtin, "agent_status", "status", structuredToolTierOperationalRead, "Read the current install/runtime/health state for an agent.")
	s.registerStructuredAgentAction(builtin, "agent_logs", "logs", structuredToolTierOperationalRead, "Read the recent logs for an agent.")
	s.registerStructuredAgentAction(builtin, "agent_start", "start", structuredToolTierHighRisk, "Start an agent runtime.")
	s.registerStructuredAgentAction(builtin, "agent_stop", "stop", structuredToolTierHighRisk, "Stop an agent runtime.")
	s.registerStructuredAgentAction(builtin, "agent_upgrade", "upgrade", structuredToolTierHighRisk, "Upgrade an agent runtime.")
	s.registerStructuredAgentAction(builtin, "agent_uninstall", "uninstall", structuredToolTierHighRisk, "Uninstall an agent runtime.")
	s.registerStructuredAgentAction(builtin, "agent_diagnose", "diagnose", structuredToolTierHighRisk, "Collect diagnose artifacts for an agent runtime.")
}

func (s *structuredToolSurface) registerStructuredAgentAction(builtin *ToolRegistry, name, action string, tier structuredToolTier, description string) {
	if s == nil || builtin == nil {
		return
	}
	s.register(structuredToolDefinition{
		descriptor: StructuredToolDescriptor{
			Name:        name,
			Description: description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_id": map[string]any{
						"type": "string",
					},
				},
				"required": []string{"agent_id"},
			},
		},
		tier: tier,
		execute: func(ctx context.Context, args map[string]any) ExecutionToolResult {
			params := stringifyStructuredToolArgs(args)
			if strings.TrimSpace(params["agent_id"]) == "" {
				return executionError("agent_id is required")
			}
			params["action"] = action
			return executeStructuredBuiltinTool(ctx, builtin, "agent_action", name, map[string]any{
				"agent_id": params["agent_id"],
				"action":   action,
			})
		},
	})
}

func executeStructuredBuiltinTool(ctx context.Context, builtin *ToolRegistry, toolName, displayName string, args map[string]any) ExecutionToolResult {
	if builtin == nil {
		return executionError("tool registry is unavailable")
	}
	resp, err := builtin.ExecuteTool(ctx, toolName, stringifyStructuredToolArgs(args))
	if err != nil {
		return executionError(err.Error())
	}
	output := strings.TrimSpace(resp.Message)
	if output == "" {
		output = strings.TrimSpace(resp.Action)
	}
	if output == "" {
		output = fmt.Sprintf("tool %s completed", displayName)
	}
	return ExecutionToolResult{Output: output}
}

func (s *structuredToolSurface) registerWorkspaceStructuredTools(workspace *ExecutionToolRegistry) {
	if s == nil || workspace == nil {
		return
	}
	for _, descriptor := range workspace.Descriptors() {
		name := strings.TrimSpace(descriptor.Name)
		if name == "" {
			continue
		}
		tier := structuredWorkspaceToolTier(name)
		s.register(structuredToolDefinition{
			descriptor: descriptor,
			tier:       tier,
			execute: func(ctx context.Context, args map[string]any) ExecutionToolResult {
				return workspace.Execute(ctx, name, args)
			},
		})
	}
}

func (s *structuredToolSurface) registerMCPStructuredTools(mcpManager MCPManager) {
	if s == nil || mcpManager == nil {
		return
	}
	for _, descriptor := range mcpManager.ListStructuredTools() {
		name := strings.TrimSpace(descriptor.Name)
		if name == "" {
			continue
		}
		s.register(structuredToolDefinition{
			descriptor: descriptor,
			tier:       structuredMCPToolTier(name),
			execute: func(ctx context.Context, args map[string]any) ExecutionToolResult {
				return mcpManager.ExecuteTool(ctx, name, args)
			},
		})
	}
}

func (s *structuredToolSurface) registerSubagentStructuredTools(manager SubagentManager) {
	if s == nil || manager == nil {
		return
	}
	s.register(structuredToolDefinition{
		descriptor: StructuredToolDescriptor{
			Name:        "subagent_result",
			Description: "Read the current status and result of a delegated subagent job.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"job_id": map[string]any{
						"type": "string",
					},
				},
				"required": []string{"job_id"},
			},
		},
		tier: structuredToolTierOperationalRead,
		execute: func(ctx context.Context, args map[string]any) ExecutionToolResult {
			jobID := strings.TrimSpace(stringifyStructuredToolArg(args["job_id"]))
			if jobID == "" {
				return executionError("job_id is required")
			}
			job, err := manager.Job(ctx, jobID)
			if err != nil {
				return executionError(err.Error())
			}
			output := fmt.Sprintf("subagent job %s status=%s", job.JobID, job.Status)
			if summary := strings.TrimSpace(job.Summary); summary != "" {
				output += "\nsummary: " + summary
			}
			if result := strings.TrimSpace(job.Result); result != "" {
				output += "\n" + result
			}
			if failure := strings.TrimSpace(job.Error); failure != "" {
				output += "\nerror: " + failure
			}
			return ExecutionToolResult{
				Output: output,
				Metadata: map[string]any{
					"delegation": job,
				},
			}
		},
	})
}

func structuredWorkspaceToolTier(name string) structuredToolTier {
	switch strings.TrimSpace(name) {
	case "web_fetch", "web_search":
		return structuredToolTierMetadataRead
	case "read_file", "list_dir":
		return structuredToolTierWorkspaceRead
	case "write_file", "append_file", "edit_file":
		return structuredToolTierWorkspaceMutation
	case "exec", "send_file", "spawn_subagent":
		return structuredToolTierHighRisk
	default:
		return structuredToolTierWorkspaceRead
	}
}

func structuredMCPToolTier(name string) structuredToolTier {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(lower, "get_"),
		strings.HasPrefix(lower, "list_"),
		strings.HasPrefix(lower, "read_"),
		strings.Contains(lower, "search"):
		return structuredToolTierMetadataRead
	default:
		return structuredToolTierHighRisk
	}
}

func (s *structuredToolSurface) register(def structuredToolDefinition) {
	if s == nil || def.execute == nil {
		return
	}
	name := strings.TrimSpace(def.descriptor.Name)
	if name == "" {
		return
	}
	if _, exists := s.tools[name]; exists {
		return
	}
	def.descriptor.Name = name
	def.descriptor.Parameters = cloneToolSchema(def.descriptor.Parameters)
	s.order = append(s.order, name)
	s.tools[name] = def
}

func (s *structuredToolSurface) Descriptors() []StructuredToolDescriptor {
	if s == nil {
		return nil
	}
	out := make([]StructuredToolDescriptor, 0, len(s.order))
	for _, name := range s.order {
		def := s.tools[name]
		if s.policy.decision(def.tier) != structuredToolDecisionAllow {
			continue
		}
		out = append(out, StructuredToolDescriptor{
			Name:        def.descriptor.Name,
			Description: def.descriptor.Description,
			Parameters:  cloneToolSchema(def.descriptor.Parameters),
		})
	}
	return out
}

func (s *structuredToolSurface) Execute(ctx context.Context, name string, args map[string]any) ExecutionToolResult {
	if s == nil {
		return executionError("structured tool surface is unavailable")
	}
	def, ok := s.tools[strings.TrimSpace(name)]
	if !ok {
		return executionError(fmt.Sprintf("unknown tool %q", name))
	}
	decision := evaluateStructuredToolPolicy(name, args, s.policy.decision(def.tier))
	switch decision.Decision {
	case structuredToolDecisionAllow:
		return applyStructuredPolicyMetadata(def.execute(ctx, args), decision)
	case structuredToolDecisionAsk:
		return executionAskWithPolicy(fmt.Sprintf("tool %s requires confirmation before automatic structured execution", strings.TrimSpace(name)), decision)
	default:
		return executionDenyWithPolicy(fmt.Sprintf("tool %s requires higher permissions and is not available for automatic structured execution", strings.TrimSpace(name)), decision)
	}
}

func (s *structuredToolSurface) ExecuteApproved(ctx context.Context, name string, args map[string]any) ExecutionToolResult {
	if s == nil {
		return executionError("structured tool surface is unavailable")
	}
	def, ok := s.tools[strings.TrimSpace(name)]
	if !ok {
		return executionError(fmt.Sprintf("unknown tool %q", name))
	}
	decision := evaluateStructuredToolPolicy(name, args, s.policy.decision(def.tier))
	if decision.Decision == structuredToolDecisionDeny {
		return executionDenyWithPolicy(fmt.Sprintf("tool %s requires higher permissions and is not available for automatic structured execution", strings.TrimSpace(name)), decision)
	}
	return applyStructuredPolicyMetadata(def.execute(ctx, args), decision)
}

func stringifyStructuredToolArgs(args map[string]any) map[string]string {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]string, len(args))
	for key, value := range args {
		out[key] = stringifyStructuredToolArg(value)
	}
	return out
}

func stringifyStructuredToolArg(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		if raw, err := json.Marshal(typed); err == nil {
			return string(raw)
		}
		return fmt.Sprint(typed)
	}
}
