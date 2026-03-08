package gateway

import (
	"fmt"
	"sort"
	"strings"
)

const (
	defaultOrchestratorTaskTimeoutMs = 60_000
	maxOrchestratorTaskTimeoutMs     = 300_000
	maxOrchestratorTaskRetryBudget   = 5
)

type OrchestratorExecutionStatus string

const (
	OrchestratorExecutionStatusPendingAuthorization OrchestratorExecutionStatus = "pending_authorization"
	OrchestratorExecutionStatusProvisioning         OrchestratorExecutionStatus = "provisioning"
	OrchestratorExecutionStatusRunning              OrchestratorExecutionStatus = "running"
	OrchestratorExecutionStatusCompleted            OrchestratorExecutionStatus = "completed"
	OrchestratorExecutionStatusFailed               OrchestratorExecutionStatus = "failed"
	OrchestratorExecutionStatusCancelled            OrchestratorExecutionStatus = "cancelled"
	OrchestratorExecutionStatusDeclined             OrchestratorExecutionStatus = "declined"
)

type OrchestratorTaskStatus string

const (
	OrchestratorTaskStatusCompleted OrchestratorTaskStatus = "completed"
	OrchestratorTaskStatusFailed    OrchestratorTaskStatus = "failed"
)

type OrchestratorWorkerState string

const (
	OrchestratorWorkerStateProvisioning OrchestratorWorkerState = "provisioning"
	OrchestratorWorkerStateReady        OrchestratorWorkerState = "ready"
	OrchestratorWorkerStateBusy         OrchestratorWorkerState = "busy"
	OrchestratorWorkerStateReclaiming   OrchestratorWorkerState = "reclaiming"
	OrchestratorWorkerStateReclaimed    OrchestratorWorkerState = "reclaimed"
	OrchestratorWorkerStateError        OrchestratorWorkerState = "error"
)

type OrchestratorToolPolicy struct {
	Mode         string   `json:"mode,omitempty"`
	AllowedTools []string `json:"allowedTools,omitempty"`
}

type OrchestratorRequiredWorker struct {
	HostID     string   `json:"hostId,omitempty"`
	HostLabels []string `json:"hostLabels,omitempty"`
	AgentID    string   `json:"agentId"`
	Count      int      `json:"count"`
}

type OrchestratorTaskUnit struct {
	ID             string                 `json:"id"`
	Input          string                 `json:"input"`
	ExpectedSchema map[string]interface{} `json:"expectedSchema,omitempty"`
	TimeoutMs      int                    `json:"timeoutMs,omitempty"`
	RetryBudget    int                    `json:"retryBudget,omitempty"`
	ToolPolicy     string                 `json:"toolPolicy,omitempty"`
	HostID         string                 `json:"hostId,omitempty"`
	HostLabels     []string               `json:"hostLabels,omitempty"`
	AgentID        string                 `json:"agentId,omitempty"`
	SessionID      string                 `json:"sessionId,omitempty"`
}

type OrchestratorAuthorization struct {
	InfrastructureApproved bool   `json:"infrastructureApproved"`
	ApprovedBy             string `json:"approvedBy,omitempty"`
	ApprovedAt             string `json:"approvedAt,omitempty"`
}

type OrchestratorExecutionPolicyTarget struct {
	HostID     string   `json:"hostId,omitempty"`
	HostLabels []string `json:"hostLabels,omitempty"`
	AgentID    string   `json:"agentId"`
	Count      int      `json:"count"`
}

type OrchestratorExecutionPolicySnapshot struct {
	Decision                       string                              `json:"decision"`
	Reason                         string                              `json:"reason,omitempty"`
	Summary                        string                              `json:"summary,omitempty"`
	RequiresInfrastructureApproval bool                                `json:"requiresInfrastructureApproval"`
	ConfiguredMaxConcurrency       int                                 `json:"configuredMaxConcurrency,omitempty"`
	EffectiveMaxConcurrency        int                                 `json:"effectiveMaxConcurrency,omitempty"`
	ToolPolicy                     OrchestratorToolPolicy              `json:"toolPolicy,omitempty"`
	MaxTaskTimeoutMs               int                                 `json:"maxTaskTimeoutMs,omitempty"`
	MaxRetryBudget                 int                                 `json:"maxRetryBudget,omitempty"`
	MatchedRuleID                  string                              `json:"matchedRuleId,omitempty"`
	MatchedRuleName                string                              `json:"matchedRuleName,omitempty"`
	ApprovedBy                     string                              `json:"approvedBy,omitempty"`
	ApprovedAt                     string                              `json:"approvedAt,omitempty"`
	Targets                        []OrchestratorExecutionPolicyTarget `json:"targets,omitempty"`
}

type OrchestratorTaskResult struct {
	TaskID      string                 `json:"taskId"`
	Status      OrchestratorTaskStatus `json:"status"`
	WorkerID    string                 `json:"workerId,omitempty"`
	HostID      string                 `json:"hostId,omitempty"`
	AgentID     string                 `json:"agentId,omitempty"`
	Attempts    int                    `json:"attempts"`
	Output      string                 `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	StartedAt   string                 `json:"startedAt,omitempty"`
	CompletedAt string                 `json:"completedAt,omitempty"`
	LatencyMs   int64                  `json:"latencyMs,omitempty"`
}

type OrchestratorExecution struct {
	ID                string                              `json:"id"`
	Goal              string                              `json:"goal"`
	RequestedProvider string                              `json:"requestedProvider,omitempty"`
	IdempotencyKey    string                              `json:"idempotencyKey,omitempty"`
	ApprovalScope     string                              `json:"approvalScope"`
	ToolPolicy        OrchestratorToolPolicy              `json:"toolPolicy,omitempty"`
	RequiredWorkers   []OrchestratorRequiredWorker        `json:"requiredWorkers"`
	TaskUnits         []OrchestratorTaskUnit              `json:"taskUnits"`
	Status            OrchestratorExecutionStatus         `json:"status"`
	MaxConcurrency    int                                 `json:"maxConcurrency,omitempty"`
	Authorization     OrchestratorAuthorization           `json:"authorization"`
	Policy            OrchestratorExecutionPolicySnapshot `json:"policy,omitempty"`
	Governance        OrchestratorExecutionGovernance     `json:"governance,omitempty"`
	Results           []OrchestratorTaskResult            `json:"results,omitempty"`
	Error             string                              `json:"error,omitempty"`
	CreatedAt         string                              `json:"createdAt"`
	StartedAt         string                              `json:"startedAt,omitempty"`
	CompletedAt       string                              `json:"completedAt,omitempty"`
	UpdatedAt         string                              `json:"updatedAt"`
}

type OrchestratorExecutionGovernance struct {
	ProviderResolutions []ProviderGovernanceResolution `json:"providerResolutions,omitempty"`
}

type OrchestratorWorkerLease struct {
	ID             string                  `json:"id"`
	ExecutionID    string                  `json:"executionId"`
	HostID         string                  `json:"hostId"`
	AgentID        string                  `json:"agentId"`
	State          OrchestratorWorkerState `json:"state"`
	Ephemeral      bool                    `json:"ephemeral"`
	InstalledByRun bool                    `json:"installedByRun"`
	TaskCount      int                     `json:"taskCount"`
	LastError      string                  `json:"lastError,omitempty"`
	LeaseExpireAt  string                  `json:"leaseExpireAt,omitempty"`
	HeartbeatAt    string                  `json:"heartbeatAt,omitempty"`
	CreatedAt      string                  `json:"createdAt"`
	UpdatedAt      string                  `json:"updatedAt"`
}

type OrchestratorWorkerInventoryItem struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	HostID         string `json:"hostId"`
	HostName       string `json:"hostName,omitempty"`
	AgentID        string `json:"agentId"`
	State          string `json:"state"`
	ExecutionID    string `json:"executionId,omitempty"`
	TaskCount      int    `json:"taskCount,omitempty"`
	Ephemeral      bool   `json:"ephemeral,omitempty"`
	InstalledByRun bool   `json:"installedByRun,omitempty"`
	RuntimeState   string `json:"runtimeState,omitempty"`
	RuntimeMode    string `json:"runtimeMode,omitempty"`
	Health         string `json:"health,omitempty"`
	DriftState     string `json:"driftState,omitempty"`
	LastSyncStatus string `json:"lastSyncStatus,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	LeaseExpireAt  string `json:"leaseExpireAt,omitempty"`
	HeartbeatAt    string `json:"heartbeatAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

type OrchestratorWorkerInventorySummary struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Busy   int `json:"busy"`
	Error  int `json:"error"`
	Local  int `json:"local"`
	Remote int `json:"remote"`
}

func normalizeOrchestratorExecutionForStore(in OrchestratorExecution) OrchestratorExecution {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	out.Goal = strings.TrimSpace(out.Goal)
	out.RequestedProvider = strings.TrimSpace(out.RequestedProvider)
	out.IdempotencyKey = strings.TrimSpace(out.IdempotencyKey)
	out.ApprovalScope = strings.TrimSpace(out.ApprovalScope)
	if out.ApprovalScope == "" {
		out.ApprovalScope = "infrastructure_only"
	}
	if out.MaxConcurrency <= 0 {
		out.MaxConcurrency = defaultOrchestratorMaxConcurrency
	}
	if out.MaxConcurrency > 64 {
		out.MaxConcurrency = 64
	}
	out.ToolPolicy = normalizeOrchestratorToolPolicy(out.ToolPolicy)
	if out.Results == nil {
		out.Results = []OrchestratorTaskResult{}
	}
	out.Error = strings.TrimSpace(out.Error)
	out.Policy = buildOrchestratorExecutionPolicySnapshot(out)
	return out
}

func normalizeOrchestratorWorkerLeaseForStore(in OrchestratorWorkerLease) OrchestratorWorkerLease {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	out.ExecutionID = strings.TrimSpace(out.ExecutionID)
	out.HostID = strings.TrimSpace(out.HostID)
	out.AgentID = strings.TrimSpace(out.AgentID)
	out.LastError = strings.TrimSpace(out.LastError)
	if out.State == "" {
		out.State = OrchestratorWorkerStateProvisioning
	}
	return out
}

func normalizeOrchestratorRequiredWorker(in OrchestratorRequiredWorker) (OrchestratorRequiredWorker, error) {
	out := in
	out.HostID = strings.TrimSpace(out.HostID)
	out.HostLabels = normalizeStringSelectorList(out.HostLabels, true)
	out.AgentID = strings.ToLower(strings.TrimSpace(out.AgentID))
	if out.AgentID == "" {
		out.AgentID = "zeroclaw"
	}
	if out.Count <= 0 {
		out.Count = 1
	}
	if out.Count > 64 {
		out.Count = 64
	}
	if err := validateAgentIdentifier(out.AgentID); err != nil {
		return OrchestratorRequiredWorker{}, err
	}
	return out, nil
}

func normalizeOrchestratorTask(in OrchestratorTaskUnit, idx int) (OrchestratorTaskUnit, error) {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	if out.ID == "" {
		out.ID = fmt.Sprintf("task-%d", idx+1)
	}
	out.Input = strings.TrimSpace(out.Input)
	if out.Input == "" {
		return OrchestratorTaskUnit{}, errOrchestratorValidation("task input is required", idx)
	}
	out.HostID = strings.TrimSpace(out.HostID)
	out.HostLabels = normalizeStringSelectorList(out.HostLabels, true)
	out.AgentID = strings.ToLower(strings.TrimSpace(out.AgentID))
	if out.AgentID != "" {
		if err := validateAgentIdentifier(out.AgentID); err != nil {
			return OrchestratorTaskUnit{}, errOrchestratorValidation(err.Error(), idx)
		}
	}
	out.SessionID = strings.TrimSpace(out.SessionID)
	if out.TimeoutMs <= 0 {
		out.TimeoutMs = defaultOrchestratorTaskTimeoutMs
	}
	if out.TimeoutMs > maxOrchestratorTaskTimeoutMs {
		out.TimeoutMs = maxOrchestratorTaskTimeoutMs
	}
	if out.RetryBudget < 0 {
		out.RetryBudget = 0
	}
	if out.RetryBudget > maxOrchestratorTaskRetryBudget {
		out.RetryBudget = maxOrchestratorTaskRetryBudget
	}
	out.ToolPolicy = strings.TrimSpace(out.ToolPolicy)
	return out, nil
}

func normalizeOrchestratorExecution(in OrchestratorExecution) (OrchestratorExecution, error) {
	out := in
	out.Goal = strings.TrimSpace(out.Goal)
	out.RequestedProvider = strings.TrimSpace(out.RequestedProvider)
	if out.Goal == "" {
		return OrchestratorExecution{}, errOrchestratorValidation("goal is required", -1)
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
