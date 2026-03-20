package gateway

import (
	"carrier/baseagent"
	"fmt"
	"sort"
	"strings"
)

func buildOrchestratorExecutionRuntimeContextEntries(execution OrchestratorExecution) []baseagent.RuntimeContextEntry {
	work := normalizeOrchestratorExecutionWorkContext(execution.Work)
	toolPolicy := normalizeOrchestratorToolPolicy(execution.ToolPolicy)
	entries := []baseagent.RuntimeContextEntry{
		orchestratorRuntimeContextEntry("execution.id", execution.ID, "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("execution.mode", string(execution.Mode), "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("execution.team", execution.Team, "orchestrator", "identity"),
		orchestratorRuntimeContextEntry("execution.project", execution.Project, "orchestrator", "identity"),
		orchestratorRuntimeContextEntry("execution.environment", execution.Environment, "orchestrator", "identity"),
		orchestratorRuntimeContextEntry("execution.template_id", execution.TemplateID, "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("execution.trigger_source", execution.TriggerSource, "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("execution.trigger_id", execution.TriggerID, "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("execution.initiator", execution.Initiator, "orchestrator", "identity"),
		orchestratorRuntimeContextEntry("execution.requested_provider", execution.RequestedProvider, "orchestrator", "integration"),
		orchestratorRuntimeContextEntry("policy.decision", execution.Policy.Decision, "policy", "policy"),
		orchestratorRuntimeContextEntry("policy.rule_id", execution.Policy.MatchedRuleID, "policy", "policy"),
		orchestratorRuntimeContextEntry("policy.tool_mode", toolPolicy.Mode, "policy", "policy"),
		orchestratorRuntimeContextEntry("work.project_id", work.ProjectID, "work", "workflow"),
		orchestratorRuntimeContextEntry("work.work_item_id", work.WorkItemID, "work", "workflow"),
		orchestratorRuntimeContextEntry("work.run_id", work.RunID, "work", "workflow"),
		orchestratorRuntimeContextEntry("work.workspace_id", work.WorkspaceID, "work", "workspace"),
		orchestratorRuntimeContextEntry("work.workspace_path", work.WorkspacePath, "work", "workspace"),
		orchestratorRuntimeContextEntry("work.backend", work.Backend, "work", "workspace"),
	}
	if len(toolPolicy.AllowedTools) > 0 {
		entries = append(entries, baseagent.RuntimeContextEntry{
			Key:           "policy.allowed_tools",
			Value:         append([]string(nil), toolPolicy.AllowedTools...),
			Source:        "policy",
			Class:         "policy",
			RedactionMode: "hidden",
		})
	}
	return baseagent.NormalizeRuntimeContextEntries(entries)
}

func buildOrchestratorTaskRuntimeContextEntries(execution OrchestratorExecution, task OrchestratorTaskUnit, lease OrchestratorWorkerLease) []baseagent.RuntimeContextEntry {
	entries := buildOrchestratorExecutionRuntimeContextEntries(execution)
	entries = append(entries,
		orchestratorRuntimeContextEntry("task.id", task.ID, "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("task.agent_id", task.AgentID, "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("task.host_id", firstNonEmptyOrchestratorValue(task.HostID, lease.HostID), "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("task.session_id", task.SessionID, "orchestrator", "workflow"),
		orchestratorRuntimeContextEntry("worker.lease_id", lease.ID, "orchestrator", "workflow"),
	)
	return baseagent.NormalizeRuntimeContextEntries(entries)
}

func buildOrchestratorExecutionRuntimeContextManifest(execution OrchestratorExecution) baseagent.RuntimeContextManifest {
	return baseagent.BuildRuntimeContextManifest(buildOrchestratorExecutionRuntimeContextEntries(execution))
}

func mergeOrchestratorRuntimeContextManifest(base, overlay baseagent.RuntimeContextManifest) baseagent.RuntimeContextManifest {
	merged := baseagent.RuntimeContextManifest{
		Entries: append(append([]baseagent.RuntimeContextManifestEntry(nil), base.Entries...), overlay.Entries...),
	}
	return baseagent.NormalizeRuntimeContextManifest(merged)
}

func normalizeOrchestratorExecutionGuardrailState(in OrchestratorExecutionGuardrails) OrchestratorExecutionGuardrails {
	events := baseagent.NormalizeGuardrailEvents(in.Events)
	return OrchestratorExecutionGuardrails{
		Summary: summarizeOrchestratorGuardrailEvents(events),
		Events:  events,
	}
}

func normalizeStoredOrchestratorExecutionGuardrails(execution OrchestratorExecution) OrchestratorExecutionGuardrails {
	events := baseagent.NormalizeGuardrailEvents(execution.Guardrails.Events)
	if !hasExecutionLaunchGuardrailEvent(events) {
		events = append(events, buildExecutionLaunchGuardrailEvent(execution))
	}
	events = baseagent.NormalizeGuardrailEvents(events)
	return OrchestratorExecutionGuardrails{
		Summary: summarizeOrchestratorGuardrailEvents(events),
		Events:  events,
	}
}

func hasExecutionLaunchGuardrailEvent(events []baseagent.GuardrailEvent) bool {
	for _, event := range events {
		if event.Scope == baseagent.GuardrailScopeExecutionLaunch {
			return true
		}
	}
	return false
}

func buildExecutionLaunchGuardrailEvent(execution OrchestratorExecution) baseagent.GuardrailEvent {
	return baseagent.GuardrailEvent{
		Scope:       baseagent.GuardrailScopeExecutionLaunch,
		Decision:    policyDecisionToGuardrailDecision(execution.Policy.Decision),
		RuleID:      strings.TrimSpace(execution.Policy.MatchedRuleID),
		Reason:      strings.TrimSpace(execution.Policy.Reason),
		TriggeredAt: firstNonEmptyOrchestratorValue(execution.CreatedAt, execution.UpdatedAt),
	}
}

func appendExecutionLaunchGuardrailResolution(execution *OrchestratorExecution, resolution string) {
	if execution == nil {
		return
	}
	resolvedAt := nowTimestamp()
	triggeredAt := firstNonEmptyOrchestratorValue(execution.CreatedAt, execution.UpdatedAt)
	for _, event := range execution.Guardrails.Events {
		if event.Scope == baseagent.GuardrailScopeExecutionLaunch && strings.TrimSpace(event.TriggeredAt) != "" {
			triggeredAt = strings.TrimSpace(event.TriggeredAt)
			break
		}
	}
	execution.Guardrails.Events = append(baseagent.NormalizeGuardrailEvents(execution.Guardrails.Events), baseagent.GuardrailEvent{
		Scope:       baseagent.GuardrailScopeExecutionLaunch,
		Decision:    policyDecisionToGuardrailDecision(execution.Policy.Decision),
		RuleID:      strings.TrimSpace(execution.Policy.MatchedRuleID),
		Reason:      strings.TrimSpace(execution.Policy.Reason),
		TriggeredAt: triggeredAt,
		ResolvedAt:  resolvedAt,
		Resolution:  strings.TrimSpace(resolution),
	})
	execution.Guardrails = normalizeStoredOrchestratorExecutionGuardrails(*execution)
}

func summarizeOrchestratorGuardrailEvents(events []baseagent.GuardrailEvent) OrchestratorGuardrailSummary {
	summary := OrchestratorGuardrailSummary{Total: len(events)}
	for _, event := range events {
		switch event.Decision {
		case baseagent.GuardrailDecisionWarn:
			summary.WarnCount++
		case baseagent.GuardrailDecisionAsk:
			summary.AskCount++
		case baseagent.GuardrailDecisionDeny:
			summary.DenyCount++
		default:
			summary.AllowCount++
		}
		summary.LastDecision = strings.TrimSpace(string(event.Decision))
	}
	return summary
}

func policyDecisionToGuardrailDecision(decision string) baseagent.GuardrailDecision {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case orchestratorPolicyDecisionAsk:
		return baseagent.GuardrailDecisionAsk
	case orchestratorPolicyDecisionDeny:
		return baseagent.GuardrailDecisionDeny
	default:
		return baseagent.GuardrailDecisionAllow
	}
}

func buildOrchestratorExecutionMetadataSnapshot(execution OrchestratorExecution) OrchestratorExecutionMetadataSnapshot {
	normalized := normalizeOrchestratorExecutionForStore(execution)
	nodes, edges := buildOrchestratorExecutionGraph(normalized)
	return OrchestratorExecutionMetadataSnapshot{
		ExecutionID:            strings.TrimSpace(normalized.ID),
		Mode:                   normalized.Mode,
		RequestedProvider:      strings.TrimSpace(normalized.RequestedProvider),
		Work:                   normalizeOrchestratorExecutionWorkContext(normalized.Work),
		ToolPolicy:             normalizeOrchestratorToolPolicy(normalized.ToolPolicy),
		RequiredMemory:         append([]string(nil), normalized.RequiredMemory...),
		MemoryContractDigest:   strings.TrimSpace(normalized.MemoryContractDigest),
		MemoryProvenance:       append([]string(nil), normalized.MemoryProvenance...),
		SharedInstructions:     baseagent.NormalizeSharedInstructions(normalized.SharedInstructions),
		RuntimeContextManifest: baseagent.NormalizeRuntimeContextManifest(normalized.RuntimeContextManifest),
		Guardrails:             normalizeStoredOrchestratorExecutionGuardrails(normalized),
		ProviderResolutions:    append([]ProviderGovernanceResolution(nil), normalized.Governance.ProviderResolutions...),
		Nodes:                  nodes,
		Edges:                  edges,
	}
}

func buildOrchestratorExecutionGraph(execution OrchestratorExecution) ([]OrchestratorExecutionGraphNode, []OrchestratorExecutionGraphEdge) {
	nodeIndex := map[string]OrchestratorExecutionGraphNode{}
	edgeIndex := map[string]OrchestratorExecutionGraphEdge{}

	addNode := func(node OrchestratorExecutionGraphNode) {
		node.ID = strings.TrimSpace(node.ID)
		node.Kind = strings.TrimSpace(node.Kind)
		if node.ID == "" || node.Kind == "" {
			return
		}
		nodeIndex[node.ID] = node
	}
	addEdge := func(edge OrchestratorExecutionGraphEdge) {
		edge.FromID = strings.TrimSpace(edge.FromID)
		edge.ToID = strings.TrimSpace(edge.ToID)
		edge.Kind = strings.TrimSpace(edge.Kind)
		if edge.FromID == "" || edge.ToID == "" || edge.Kind == "" {
			return
		}
		edgeIndex[edge.FromID+"|"+edge.Kind+"|"+edge.ToID] = edge
	}

	executionNodeID := "execution:" + strings.TrimSpace(execution.ID)
	addNode(OrchestratorExecutionGraphNode{
		ID:     executionNodeID,
		Kind:   "execution",
		Label:  firstNonEmptyOrchestratorValue(execution.Goal, execution.ID),
		Status: strings.TrimSpace(string(execution.Status)),
	})

	for _, task := range execution.TaskUnits {
		taskID := "task:" + strings.TrimSpace(task.ID)
		addNode(OrchestratorExecutionGraphNode{
			ID:     taskID,
			Kind:   "task",
			Label:  strings.TrimSpace(task.Input),
			Status: strings.TrimSpace(task.AgentID),
			Detail: strings.TrimSpace(firstNonEmptyOrchestratorValue(task.HostID, strings.Join(task.HostLabels, ","))),
		})
		addEdge(OrchestratorExecutionGraphEdge{
			FromID: executionNodeID,
			ToID:   taskID,
			Kind:   "has_task",
			Label:  strings.TrimSpace(task.ID),
		})
	}

	for _, result := range execution.Results {
		taskID := "task:" + strings.TrimSpace(result.TaskID)
		workerLabel := strings.TrimSpace(firstNonEmptyOrchestratorValue(result.WorkerID, result.HostID+":"+result.AgentID))
		if workerLabel == "" {
			continue
		}
		workerID := "worker:" + workerLabel
		addNode(OrchestratorExecutionGraphNode{
			ID:     workerID,
			Kind:   "worker",
			Label:  workerLabel,
			Status: strings.TrimSpace(string(result.Status)),
			Detail: strings.TrimSpace(firstNonEmptyOrchestratorValue(result.HostID, result.AgentID)),
		})
		if taskID != "task:" {
			addEdge(OrchestratorExecutionGraphEdge{
				FromID: taskID,
				ToID:   workerID,
				Kind:   "assigned_worker",
				Label:  strings.TrimSpace(result.AgentID),
			})
		}
	}

	for _, artifact := range execution.Outcome.Artifacts {
		artifactID := "artifact:" + strings.TrimSpace(artifact.ID)
		addNode(OrchestratorExecutionGraphNode{
			ID:     artifactID,
			Kind:   "artifact",
			Label:  strings.TrimSpace(firstNonEmptyOrchestratorValue(artifact.Name, artifact.ID)),
			Status: strings.TrimSpace(artifact.Kind),
			Detail: strings.TrimSpace(firstNonEmptyOrchestratorValue(artifact.MediaType, artifact.Source)),
		})
		addEdge(OrchestratorExecutionGraphEdge{
			FromID: executionNodeID,
			ToID:   artifactID,
			Kind:   "has_artifact",
			Label:  strings.TrimSpace(artifact.OutputRole),
		})
		if strings.TrimSpace(artifact.TaskID) != "" {
			addEdge(OrchestratorExecutionGraphEdge{
				FromID: "task:" + strings.TrimSpace(artifact.TaskID),
				ToID:   artifactID,
				Kind:   "produces_artifact",
			})
		}
	}

	if strings.TrimSpace(execution.SnapshotID) != "" || strings.TrimSpace(execution.SnapshotDigest) != "" {
		memoryID := "memory:" + strings.TrimSpace(firstNonEmptyOrchestratorValue(execution.SnapshotID, execution.SnapshotDigest))
		addNode(OrchestratorExecutionGraphNode{
			ID:     memoryID,
			Kind:   "memory_snapshot",
			Label:  strings.TrimSpace(firstNonEmptyOrchestratorValue(execution.SnapshotID, execution.SnapshotDigest)),
			Status: strings.TrimSpace(execution.CleanupStatus),
			Detail: strings.TrimSpace(execution.MemoryContractDigest),
		})
		addEdge(OrchestratorExecutionGraphEdge{
			FromID: executionNodeID,
			ToID:   memoryID,
			Kind:   "binds_memory",
		})
	}

	nodes := make([]OrchestratorExecutionGraphNode, 0, len(nodeIndex))
	for _, node := range nodeIndex {
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		left := nodes[i].Kind + ":" + nodes[i].ID
		right := nodes[j].Kind + ":" + nodes[j].ID
		return left < right
	})

	edges := make([]OrchestratorExecutionGraphEdge, 0, len(edgeIndex))
	for _, edge := range edgeIndex {
		edges = append(edges, edge)
	}
	sort.SliceStable(edges, func(i, j int) bool {
		left := edges[i].FromID + ":" + edges[i].Kind + ":" + edges[i].ToID
		right := edges[j].FromID + ":" + edges[j].Kind + ":" + edges[j].ToID
		return left < right
	})

	return nodes, edges
}

func orchestratorRuntimeContextEntry(key string, value any, source, class string) baseagent.RuntimeContextEntry {
	return baseagent.RuntimeContextEntry{
		Key:           strings.TrimSpace(key),
		Value:         value,
		Source:        strings.TrimSpace(source),
		Class:         strings.TrimSpace(class),
		RedactionMode: "hidden",
	}
}

func firstNonEmptyOrchestratorValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func guardrailTriggeredAuditDetails(execution OrchestratorExecution) map[string]interface{} {
	return map[string]interface{}{
		"executionId":   strings.TrimSpace(execution.ID),
		"scope":         string(baseagent.GuardrailScopeExecutionLaunch),
		"decision":      strings.TrimSpace(string(policyDecisionToGuardrailDecision(execution.Policy.Decision))),
		"ruleId":        strings.TrimSpace(execution.Policy.MatchedRuleID),
		"reason":        strings.TrimSpace(execution.Policy.Reason),
		"templateId":    strings.TrimSpace(execution.TemplateID),
		"triggerSource": strings.TrimSpace(execution.TriggerSource),
	}
}

func guardrailResolvedAuditDetails(execution OrchestratorExecution, resolution string, actor string) map[string]interface{} {
	return map[string]interface{}{
		"executionId": strings.TrimSpace(execution.ID),
		"scope":       string(baseagent.GuardrailScopeExecutionLaunch),
		"decision":    strings.TrimSpace(string(policyDecisionToGuardrailDecision(execution.Policy.Decision))),
		"ruleId":      strings.TrimSpace(execution.Policy.MatchedRuleID),
		"resolution":  strings.TrimSpace(resolution),
		"actor":       strings.TrimSpace(actor),
	}
}

func formatExecutionWorkerLabel(result OrchestratorTaskResult) string {
	label := firstNonEmptyOrchestratorValue(result.WorkerID, result.HostID, result.AgentID)
	if label == "" {
		return ""
	}
	if strings.TrimSpace(result.WorkerID) != "" {
		return result.WorkerID
	}
	return fmt.Sprintf("%s:%s", firstNonEmptyOrchestratorValue(result.HostID, "worker"), firstNonEmptyOrchestratorValue(result.AgentID, "agent"))
}
