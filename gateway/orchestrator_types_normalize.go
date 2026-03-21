package gateway

import (
	"carrier/baseagent"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

func normalizeOrchestratorExecution(in OrchestratorExecution) (OrchestratorExecution, error) {
	out := in
	out.Mode = normalizeOrchestratorExecutionMode(out.Mode)
	out.Work = normalizeOrchestratorExecutionWorkContext(out.Work)
	out.Goal = strings.TrimSpace(out.Goal)
	out.Team = strings.TrimSpace(out.Team)
	out.Project = strings.TrimSpace(out.Project)
	out.Environment = strings.TrimSpace(out.Environment)
	out.TemplateID = strings.TrimSpace(out.TemplateID)
	out.TemplateVersion = strings.TrimSpace(out.TemplateVersion)
	out.TriggerSource = strings.TrimSpace(out.TriggerSource)
	out.TriggerID = strings.TrimSpace(out.TriggerID)
	out.TriggerEvent = strings.TrimSpace(out.TriggerEvent)
	out.TriggerPayloadDigest = strings.TrimSpace(out.TriggerPayloadDigest)
	out.Initiator = strings.TrimSpace(out.Initiator)
	out.RequestedProvider = strings.TrimSpace(out.RequestedProvider)
	out.RequiredMemory = normalizeStringSelectorList(out.RequiredMemory, true)
	out.MemoryContractDigest = strings.TrimSpace(out.MemoryContractDigest)
	out.MemoryProvenance = normalizeStringSelectorList(out.MemoryProvenance, true)
	out = normalizeOrchestratorDelegatedMemoryState(out)
	out.DistillOutputs = normalizeStringSelectorList(out.DistillOutputs, true)
	out.SharedInstructions = baseagent.NormalizeSharedInstructions(out.SharedInstructions)
	out.RuntimeContextManifest = baseagent.NormalizeRuntimeContextManifest(out.RuntimeContextManifest)
	out.Guardrails = normalizeOrchestratorExecutionGuardrailState(out.Guardrails)
	if len(out.MemoryProvenance) == 0 {
		out.MemoryProvenance = append([]string(nil), out.RequiredMemory...)
	}
	if out.MemoryContractDigest == "" {
		out.MemoryContractDigest = buildMemoryContractDigest(out.MemoryProvenance)
	}
	if out.Goal == "" {
		return OrchestratorExecution{}, errOrchestratorValidation("goal is required", -1)
	}
	if out.Mode == OrchestratorExecutionModeWork && strings.TrimSpace(out.Work.WorkItemID) == "" {
		return OrchestratorExecution{}, errOrchestratorValidation("work.workItemId is required for work executions", -1)
	}
	out.IdempotencyKey = strings.TrimSpace(out.IdempotencyKey)
	out.ApprovalScope = strings.TrimSpace(out.ApprovalScope)
	if out.ApprovalScope == "" {
		out.ApprovalScope = "infrastructure_only"
	}
	if out.ApprovalScope != "infrastructure_only" {
		return OrchestratorExecution{}, errOrchestratorValidation("approvalScope must be infrastructure_only", -1)
	}
	if len(out.RequiredWorkers) == 0 {
		return OrchestratorExecution{}, errOrchestratorValidation("requiredWorkers is required", -1)
	}
	for i := range out.RequiredWorkers {
		worker, err := normalizeOrchestratorRequiredWorker(out.RequiredWorkers[i])
		if err != nil {
			return OrchestratorExecution{}, errOrchestratorValidation("invalid requiredWorkers entry: "+err.Error(), i)
		}
		if worker.HostID == "" && len(worker.HostLabels) == 0 {
			return OrchestratorExecution{}, errOrchestratorValidation("requiredWorkers.hostId or requiredWorkers.hostLabels is required", i)
		}
		out.RequiredWorkers[i] = worker
	}
	if len(out.TaskUnits) == 0 {
		return OrchestratorExecution{}, errOrchestratorValidation("taskUnits is required", -1)
	}
	for i := range out.TaskUnits {
		task, err := normalizeOrchestratorTask(out.TaskUnits[i], i)
		if err != nil {
			return OrchestratorExecution{}, err
		}
		out.TaskUnits[i] = task
	}
	if out.MaxConcurrency <= 0 {
		out.MaxConcurrency = defaultOrchestratorMaxConcurrency
	}
	if out.MaxConcurrency > 64 {
		out.MaxConcurrency = 64
	}
	out.ToolPolicy = normalizeOrchestratorToolPolicy(out.ToolPolicy)
	return out, nil
}

func normalizeOrchestratorDelegatedMemoryState(in OrchestratorExecution) OrchestratorExecution {
	out := in
	out.AgentLifecycleMode = normalizeManagedEnumValue(out.AgentLifecycleMode)
	if out.AgentLifecycleMode != orchestratorAgentLifecycleMode {
		out.AgentLifecycleMode = orchestratorAgentLifecycleMode
	}
	out.MemoryBindingMode = normalizeManagedEnumValue(out.MemoryBindingMode)
	if out.MemoryBindingMode != orchestratorMemoryBindingMode {
		out.MemoryBindingMode = orchestratorMemoryBindingMode
	}
	out.SourceScopes = append([]string(nil), out.RequiredMemory...)
	out.SnapshotID = strings.TrimSpace(out.SnapshotID)
	out.SnapshotDigest = strings.TrimSpace(out.SnapshotDigest)
	if out.SnapshotDigest == "" && len(out.SourceScopes) > 0 {
		out.SnapshotDigest = buildMemoryContractDigest(out.SourceScopes)
	}
	out.ChildAgentID = strings.TrimSpace(out.ChildAgentID)
	out.ChildPerAgentMemoryID = strings.TrimSpace(out.ChildPerAgentMemoryID)
	out.DistillRunID = strings.TrimSpace(out.DistillRunID)
	out.CleanupStatus = normalizeManagedEnumValue(out.CleanupStatus)
	return out
}

func resetOrchestratorDelegatedMemoryProgress(in OrchestratorExecution) OrchestratorExecution {
	out := in
	out.SnapshotID = ""
	out.SnapshotDigest = ""
	if len(out.SourceScopes) > 0 {
		out.SnapshotDigest = buildMemoryContractDigest(out.SourceScopes)
	}
	out.ChildAgentID = ""
	out.ChildPerAgentMemoryID = ""
	out.DistillRunID = ""
	out.CleanupStatus = ""
	return out
}

func normalizeOrchestratorExecutionMode(in OrchestratorExecutionMode) OrchestratorExecutionMode {
	switch OrchestratorExecutionMode(strings.ToLower(strings.TrimSpace(string(in)))) {
	case OrchestratorExecutionModeWork:
		return OrchestratorExecutionModeWork
	default:
		return OrchestratorExecutionModeTask
	}
}

func normalizeOrchestratorExecutionWorkContext(in OrchestratorExecutionWorkContext) OrchestratorExecutionWorkContext {
	out := in
	out.ProjectID = strings.TrimSpace(out.ProjectID)
	out.WorkItemID = strings.TrimSpace(out.WorkItemID)
	out.RunID = strings.TrimSpace(out.RunID)
	out.WorkspaceID = strings.TrimSpace(out.WorkspaceID)
	out.WorkspacePath = strings.TrimSpace(out.WorkspacePath)
	out.Backend = strings.TrimSpace(out.Backend)
	out.WorkflowDigest = strings.TrimSpace(out.WorkflowDigest)
	out.Phase = strings.TrimSpace(out.Phase)
	out.VerificationStatus = strings.TrimSpace(out.VerificationStatus)
	out.PublishStatus = strings.TrimSpace(out.PublishStatus)
	return out
}

func normalizeOrchestratorToolPolicy(in OrchestratorToolPolicy) OrchestratorToolPolicy {
	out := in
	out.Mode = strings.TrimSpace(out.Mode)
	if out.Mode == "" {
		out.Mode = "restricted"
	}
	seen := map[string]struct{}{}
	allowed := make([]string, 0, len(out.AllowedTools))
	for _, tool := range out.AllowedTools {
		trimmed := strings.TrimSpace(tool)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		allowed = append(allowed, trimmed)
	}
	sort.Strings(allowed)
	out.AllowedTools = allowed
	return out
}

func buildMemoryContractDigest(scopes []string) string {
	normalized := normalizeStringSelectorList(scopes, true)
	if len(normalized) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return "mem-" + hex.EncodeToString(sum[:])[:12]
}

func normalizeOrchestratorTaskResult(in OrchestratorTaskResult) OrchestratorTaskResult {
	out := in
	out.TaskID = strings.TrimSpace(out.TaskID)
	out.WorkerID = strings.TrimSpace(out.WorkerID)
	out.HostID = strings.TrimSpace(out.HostID)
	out.AgentID = strings.TrimSpace(out.AgentID)
	out.Summary = strings.TrimSpace(out.Summary)
	out.Output = strings.TrimSpace(out.Output)
	out.Error = strings.TrimSpace(out.Error)
	out.FailureReason = strings.TrimSpace(out.FailureReason)
	out.FailureCategory = strings.TrimSpace(out.FailureCategory)
	out.StartedAt = strings.TrimSpace(out.StartedAt)
	out.CompletedAt = strings.TrimSpace(out.CompletedAt)
	out.DelegatedMemory = normalizeOrchestratorDelegatedTaskMemoryState(out.DelegatedMemory)
	return out
}

func normalizeOrchestratorDelegatedTaskMemoryState(in *OrchestratorDelegatedTaskMemoryState) *OrchestratorDelegatedTaskMemoryState {
	if in == nil {
		return nil
	}
	out := *in
	out.ChildAgentID = strings.TrimSpace(out.ChildAgentID)
	out.ChildPerAgentMemoryID = strings.TrimSpace(out.ChildPerAgentMemoryID)
	out.SnapshotID = strings.TrimSpace(out.SnapshotID)
	out.SnapshotDigest = strings.TrimSpace(out.SnapshotDigest)
	out.DistillRunID = strings.TrimSpace(out.DistillRunID)
	out.CleanupStatus = normalizeManagedEnumValue(out.CleanupStatus)
	out.ParentRecordIDs = normalizeStringSelectorList(out.ParentRecordIDs, true)
	return &out
}

func normalizeOrchestratorArtifact(in OrchestratorArtifact) OrchestratorArtifact {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	out.AttachmentID = strings.TrimSpace(out.AttachmentID)
	out.TaskID = strings.TrimSpace(out.TaskID)
	out.Name = strings.TrimSpace(out.Name)
	out.Kind = strings.TrimSpace(out.Kind)
	out.OutputRole = strings.TrimSpace(strings.ToLower(out.OutputRole))
	out.MediaType = strings.TrimSpace(out.MediaType)
	out.ContentType = strings.TrimSpace(out.ContentType)
	out.Path = strings.TrimSpace(out.Path)
	out.Source = strings.TrimSpace(out.Source)
	out.Transport = strings.TrimSpace(out.Transport)
	out.DeliveryMethod = strings.TrimSpace(out.DeliveryMethod)
	out.PreviewText = strings.TrimSpace(out.PreviewText)
	out.ExternalID = strings.TrimSpace(out.ExternalID)
	out.DownloadURL = strings.TrimSpace(out.DownloadURL)
	out.CreatedAt = strings.TrimSpace(out.CreatedAt)
	if out.MediaType == "" {
		out.MediaType = out.ContentType
	}
	if out.Transport == "" {
		out.Transport = out.Source
	}
	if out.SizeBytes < 0 {
		out.SizeBytes = 0
	}
	return out
}

func normalizeOrchestratorExecutionOutcome(in OrchestratorExecutionOutcome) OrchestratorExecutionOutcome {
	out := in
	out.Summary = strings.TrimSpace(out.Summary)
	out.FailureReason = strings.TrimSpace(out.FailureReason)
	out.FailureCategory = strings.TrimSpace(out.FailureCategory)
	out.RenderMode = strings.TrimSpace(strings.ToLower(out.RenderMode))
	if out.Artifacts == nil {
		out.Artifacts = []OrchestratorArtifact{}
	} else {
		artifacts := make([]OrchestratorArtifact, 0, len(out.Artifacts))
		for _, artifact := range out.Artifacts {
			artifacts = append(artifacts, normalizeOrchestratorArtifact(artifact))
		}
		out.Artifacts = artifacts
	}
	return out
}

func buildOrchestratorExecutionPolicySnapshot(in OrchestratorExecution) OrchestratorExecutionPolicySnapshot {
	configuredMaxConcurrency := in.MaxConcurrency
	if configuredMaxConcurrency <= 0 {
		configuredMaxConcurrency = defaultOrchestratorMaxConcurrency
	}
	if configuredMaxConcurrency > 64 {
		configuredMaxConcurrency = 64
	}
	effectiveMaxConcurrency := configuredMaxConcurrency
	if taskCount := len(in.TaskUnits); taskCount > 0 && effectiveMaxConcurrency > taskCount {
		effectiveMaxConcurrency = taskCount
	}

	maxTaskTimeoutMs := 0
	maxRetryBudget := 0
	for _, task := range in.TaskUnits {
		if task.TimeoutMs > maxTaskTimeoutMs {
			maxTaskTimeoutMs = task.TimeoutMs
		}
		if task.RetryBudget > maxRetryBudget {
			maxRetryBudget = task.RetryBudget
		}
	}

	targets := make([]OrchestratorExecutionPolicyTarget, 0, len(in.RequiredWorkers))
	for _, worker := range in.RequiredWorkers {
		hostID := strings.TrimSpace(worker.HostID)
		agentID := strings.ToLower(strings.TrimSpace(worker.AgentID))
		if agentID == "" {
			agentID = "zeroclaw"
		}
		count := worker.Count
		if count <= 0 {
			count = 1
		}
		targets = append(targets, OrchestratorExecutionPolicyTarget{
			HostID:     hostID,
			HostLabels: normalizeStringSelectorList(worker.HostLabels, true),
			AgentID:    agentID,
			Count:      count,
		})
	}
	sort.SliceStable(targets, func(i, j int) bool {
		left := strings.ToLower(targets[i].HostID + ":" + strings.Join(targets[i].HostLabels, ",") + ":" + targets[i].AgentID)
		right := strings.ToLower(targets[j].HostID + ":" + strings.Join(targets[j].HostLabels, ",") + ":" + targets[j].AgentID)
		if left != right {
			return left < right
		}
		return targets[i].Count < targets[j].Count
	})

	toolPolicy := normalizeOrchestratorToolPolicy(in.ToolPolicy)
	policy := OrchestratorExecutionPolicySnapshot{
		Decision:                       firstNonEmptyPolicyValue(strings.TrimSpace(in.Policy.Decision), orchestratorPolicyDecisionAllow),
		Reason:                         strings.TrimSpace(in.Policy.Reason),
		RequiresInfrastructureApproval: strings.EqualFold(strings.TrimSpace(in.ApprovalScope), "infrastructure_only"),
		ConfiguredMaxConcurrency:       configuredMaxConcurrency,
		EffectiveMaxConcurrency:        effectiveMaxConcurrency,
		ToolPolicy:                     toolPolicy,
		MaxTaskTimeoutMs:               maxTaskTimeoutMs,
		MaxRetryBudget:                 maxRetryBudget,
		MatchedRuleID:                  strings.TrimSpace(in.Policy.MatchedRuleID),
		MatchedRuleName:                strings.TrimSpace(in.Policy.MatchedRuleName),
		ApprovedBy:                     strings.TrimSpace(in.Policy.ApprovedBy),
		ApprovedAt:                     strings.TrimSpace(in.Policy.ApprovedAt),
		Targets:                        targets,
	}

	summaryParts := make([]string, 0, 5)
	if policy.RequiresInfrastructureApproval {
		summaryParts = append(summaryParts, "infrastructure approval required")
	}
	if toolPolicy.Mode != "" {
		summaryParts = append(summaryParts, "tool mode "+toolPolicy.Mode)
	}
	if policy.EffectiveMaxConcurrency > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("effective concurrency %d", policy.EffectiveMaxConcurrency))
	}
	if policy.MaxTaskTimeoutMs > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("max timeout %dms", policy.MaxTaskTimeoutMs))
	}
	summaryParts = append(summaryParts, fmt.Sprintf("max retry %d", policy.MaxRetryBudget))
	policy.Summary = strings.Join(summaryParts, "; ")
	return policy
}

func firstNonEmptyPolicyValue(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func errOrchestratorValidation(message string, index int) error {
	if index >= 0 {
		return fmt.Errorf("item %d: %s", index, strings.TrimSpace(message))
	}
	return fmt.Errorf(strings.TrimSpace(message))
}
