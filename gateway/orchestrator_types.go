package gateway

import (
	"carrier/baseagent"
	"fmt"
	"strings"
)

const (
	defaultOrchestratorTaskTimeoutMs = 60_000
	maxOrchestratorTaskTimeoutMs     = 300_000
	maxOrchestratorTaskRetryBudget   = 5
	orchestratorAgentLifecycleMode   = "delegated"
	orchestratorMemoryBindingMode    = "snapshot"
)

type OrchestratorExecutionStatus string

const (
	OrchestratorExecutionStatusPendingAuthorization OrchestratorExecutionStatus = "pending_authorization"
	OrchestratorExecutionStatusProvisioning         OrchestratorExecutionStatus = "provisioning"
	OrchestratorExecutionStatusRunning              OrchestratorExecutionStatus = "running"
	OrchestratorExecutionStatusPauseRequested       OrchestratorExecutionStatus = "pause_requested"
	OrchestratorExecutionStatusPaused               OrchestratorExecutionStatus = "paused"
	OrchestratorExecutionStatusCompleted            OrchestratorExecutionStatus = "completed"
	OrchestratorExecutionStatusPartialCompleted     OrchestratorExecutionStatus = "partial_completed"
	OrchestratorExecutionStatusFailed               OrchestratorExecutionStatus = "failed"
	OrchestratorExecutionStatusRetryableFailed      OrchestratorExecutionStatus = "retryable_failed"
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
	TaskID          string                                `json:"taskId"`
	Status          OrchestratorTaskStatus                `json:"status"`
	WorkerID        string                                `json:"workerId,omitempty"`
	HostID          string                                `json:"hostId,omitempty"`
	AgentID         string                                `json:"agentId,omitempty"`
	Attempts        int                                   `json:"attempts"`
	Summary         string                                `json:"summary,omitempty"`
	Output          string                                `json:"output,omitempty"`
	Error           string                                `json:"error,omitempty"`
	FailureReason   string                                `json:"failureReason,omitempty"`
	FailureCategory string                                `json:"failureCategory,omitempty"`
	StartedAt       string                                `json:"startedAt,omitempty"`
	CompletedAt     string                                `json:"completedAt,omitempty"`
	LatencyMs       int64                                 `json:"latencyMs,omitempty"`
	DelegatedMemory *OrchestratorDelegatedTaskMemoryState `json:"delegatedMemory,omitempty"`
}

type OrchestratorDelegatedTaskMemoryState struct {
	ChildAgentID          string   `json:"childAgentId,omitempty"`
	ChildPerAgentMemoryID string   `json:"childPerAgentMemoryId,omitempty"`
	SnapshotID            string   `json:"snapshotId,omitempty"`
	SnapshotDigest        string   `json:"snapshotDigest,omitempty"`
	DistillRunID          string   `json:"distillRunId,omitempty"`
	CleanupStatus         string   `json:"cleanupStatus,omitempty"`
	ParentRecordIDs       []string `json:"parentRecordIds,omitempty"`
}

type OrchestratorArtifact struct {
	ID             string `json:"id"`
	AttachmentID   string `json:"attachmentId,omitempty"`
	TaskID         string `json:"taskId,omitempty"`
	Name           string `json:"name"`
	Kind           string `json:"kind,omitempty"`
	OutputRole     string `json:"outputRole,omitempty"`
	MediaType      string `json:"mediaType,omitempty"`
	ContentType    string `json:"contentType,omitempty"`
	SizeBytes      int64  `json:"sizeBytes,omitempty"`
	Path           string `json:"path,omitempty"`
	Source         string `json:"source,omitempty"`
	Transport      string `json:"transport,omitempty"`
	DeliveryMethod string `json:"deliveryMethod,omitempty"`
	PreviewText    string `json:"previewText,omitempty"`
	ExternalID     string `json:"externalId,omitempty"`
	DownloadURL    string `json:"downloadUrl,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
}

type OrchestratorExecutionOutcome struct {
	Summary         string                 `json:"summary,omitempty"`
	FailureReason   string                 `json:"failureReason,omitempty"`
	FailureCategory string                 `json:"failureCategory,omitempty"`
	RenderMode      string                 `json:"renderMode,omitempty"`
	Artifacts       []OrchestratorArtifact `json:"artifacts,omitempty"`
}

type OrchestratorExecutionMode string

const (
	OrchestratorExecutionModeTask OrchestratorExecutionMode = "task"
	OrchestratorExecutionModeWork OrchestratorExecutionMode = "work"
)

type OrchestratorExecutionWorkContext struct {
	ProjectID          string `json:"projectId,omitempty"`
	WorkItemID         string `json:"workItemId,omitempty"`
	RunID              string `json:"runId,omitempty"`
	WorkspaceID        string `json:"workspaceId,omitempty"`
	WorkspacePath      string `json:"workspacePath,omitempty"`
	Backend            string `json:"backend,omitempty"`
	WorkflowDigest     string `json:"workflowDigest,omitempty"`
	Phase              string `json:"phase,omitempty"`
	VerificationStatus string `json:"verificationStatus,omitempty"`
	PublishStatus      string `json:"publishStatus,omitempty"`
}

type OrchestratorGuardrailSummary struct {
	Total        int    `json:"total"`
	AllowCount   int    `json:"allowCount,omitempty"`
	WarnCount    int    `json:"warnCount,omitempty"`
	AskCount     int    `json:"askCount,omitempty"`
	DenyCount    int    `json:"denyCount,omitempty"`
	LastDecision string `json:"lastDecision,omitempty"`
}

type OrchestratorExecutionGuardrails struct {
	Summary OrchestratorGuardrailSummary `json:"summary,omitempty"`
	Events  []baseagent.GuardrailEvent   `json:"events,omitempty"`
}

type OrchestratorExecutionGraphNode struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Label  string `json:"label,omitempty"`
	Status string `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type OrchestratorExecutionGraphEdge struct {
	FromID string `json:"fromId"`
	ToID   string `json:"toId"`
	Kind   string `json:"kind"`
	Label  string `json:"label,omitempty"`
}

type OrchestratorExecutionMetadataSnapshot struct {
	ExecutionID            string                           `json:"executionId"`
	Mode                   OrchestratorExecutionMode        `json:"mode,omitempty"`
	RequestedProvider      string                           `json:"requestedProvider,omitempty"`
	Work                   OrchestratorExecutionWorkContext `json:"work,omitempty"`
	ToolPolicy             OrchestratorToolPolicy           `json:"toolPolicy,omitempty"`
	RequiredMemory         []string                         `json:"requiredMemory,omitempty"`
	MemoryContractDigest   string                           `json:"memoryContractDigest,omitempty"`
	MemoryProvenance       []string                         `json:"memoryProvenance,omitempty"`
	SharedInstructions     []baseagent.SharedInstruction    `json:"sharedInstructions,omitempty"`
	RuntimeContextManifest baseagent.RuntimeContextManifest `json:"runtimeContextManifest,omitempty"`
	Guardrails             OrchestratorExecutionGuardrails  `json:"guardrails,omitempty"`
	ProviderResolutions    []ProviderGovernanceResolution   `json:"providerResolutions,omitempty"`
	Nodes                  []OrchestratorExecutionGraphNode `json:"nodes,omitempty"`
	Edges                  []OrchestratorExecutionGraphEdge `json:"edges,omitempty"`
}

type OrchestratorExecution struct {
	ID                     string                              `json:"id"`
	Mode                   OrchestratorExecutionMode           `json:"mode,omitempty"`
	Work                   OrchestratorExecutionWorkContext    `json:"work,omitempty"`
	Goal                   string                              `json:"goal"`
	Team                   string                              `json:"team,omitempty"`
	Project                string                              `json:"project,omitempty"`
	Environment            string                              `json:"environment,omitempty"`
	TemplateID             string                              `json:"templateId,omitempty"`
	TemplateVersion        string                              `json:"templateVersion,omitempty"`
	TriggerSource          string                              `json:"triggerSource,omitempty"`
	TriggerID              string                              `json:"triggerId,omitempty"`
	TriggerEvent           string                              `json:"triggerEvent,omitempty"`
	TriggerPayloadDigest   string                              `json:"triggerPayloadDigest,omitempty"`
	Initiator              string                              `json:"initiator,omitempty"`
	RequestedProvider      string                              `json:"requestedProvider,omitempty"`
	RequiredMemory         []string                            `json:"requiredMemory,omitempty"`
	MemoryContractDigest   string                              `json:"memoryContractDigest,omitempty"`
	MemoryProvenance       []string                            `json:"memoryProvenance,omitempty"`
	AgentLifecycleMode     string                              `json:"agentLifecycleMode,omitempty"`
	MemoryBindingMode      string                              `json:"memoryBindingMode,omitempty"`
	SourceScopes           []string                            `json:"sourceScopes,omitempty"`
	SnapshotID             string                              `json:"snapshotId,omitempty"`
	SnapshotDigest         string                              `json:"snapshotDigest,omitempty"`
	ChildAgentID           string                              `json:"childAgentId,omitempty"`
	ChildPerAgentMemoryID  string                              `json:"childPerAgentMemoryId,omitempty"`
	DistillRunID           string                              `json:"distillRunId,omitempty"`
	CleanupStatus          string                              `json:"cleanupStatus,omitempty"`
	DistillOutputs         []string                            `json:"distillOutputs,omitempty"`
	IdempotencyKey         string                              `json:"idempotencyKey,omitempty"`
	ParentExecutionID      string                              `json:"parentExecutionId,omitempty"`
	SourceExecutionID      string                              `json:"sourceExecutionId,omitempty"`
	LaunchReason           string                              `json:"launchReason,omitempty"`
	SharedInstructions     []baseagent.SharedInstruction       `json:"sharedInstructions,omitempty"`
	RuntimeContextManifest baseagent.RuntimeContextManifest    `json:"runtimeContextManifest,omitempty"`
	ApprovalScope          string                              `json:"approvalScope"`
	ToolPolicy             OrchestratorToolPolicy              `json:"toolPolicy,omitempty"`
	RequiredWorkers        []OrchestratorRequiredWorker        `json:"requiredWorkers"`
	TaskUnits              []OrchestratorTaskUnit              `json:"taskUnits"`
	Status                 OrchestratorExecutionStatus         `json:"status"`
	MaxConcurrency         int                                 `json:"maxConcurrency,omitempty"`
	Authorization          OrchestratorAuthorization           `json:"authorization"`
	Policy                 OrchestratorExecutionPolicySnapshot `json:"policy,omitempty"`
	Governance             OrchestratorExecutionGovernance     `json:"governance,omitempty"`
	Guardrails             OrchestratorExecutionGuardrails     `json:"guardrails,omitempty"`
	Outcome                OrchestratorExecutionOutcome        `json:"outcome,omitempty"`
	Results                []OrchestratorTaskResult            `json:"results,omitempty"`
	Error                  string                              `json:"error,omitempty"`
	CreatedAt              string                              `json:"createdAt"`
	StartedAt              string                              `json:"startedAt,omitempty"`
	CompletedAt            string                              `json:"completedAt,omitempty"`
	UpdatedAt              string                              `json:"updatedAt"`
}

type OrchestratorExecutionGovernance struct {
	ProviderResolutions []ProviderGovernanceResolution `json:"providerResolutions,omitempty"`
}

type OrchestratorWorkerLease struct {
	ID              string                  `json:"id"`
	ExecutionID     string                  `json:"executionId"`
	HostID          string                  `json:"hostId"`
	AgentID         string                  `json:"agentId"`
	State           OrchestratorWorkerState `json:"state"`
	LeaseState      string                  `json:"leaseState,omitempty"`
	Ephemeral       bool                    `json:"ephemeral"`
	InstalledByRun  bool                    `json:"installedByRun"`
	TaskCount       int                     `json:"taskCount"`
	QueuePosition   int                     `json:"queuePosition,omitempty"`
	Stale           bool                    `json:"stale,omitempty"`
	StaleReason     string                  `json:"staleReason,omitempty"`
	LastError       string                  `json:"lastError,omitempty"`
	LeaseExpireAt   string                  `json:"leaseExpireAt,omitempty"`
	LastHeartbeatAt string                  `json:"lastHeartbeatAt,omitempty"`
	HeartbeatAt     string                  `json:"heartbeatAt,omitempty"`
	CreatedAt       string                  `json:"createdAt"`
	UpdatedAt       string                  `json:"updatedAt"`
}

type OrchestratorWorkerInventoryItem struct {
	ID              string `json:"id"`
	Source          string `json:"source"`
	HostID          string `json:"hostId"`
	HostName        string `json:"hostName,omitempty"`
	AgentID         string `json:"agentId"`
	State           string `json:"state"`
	ExecutionID     string `json:"executionId,omitempty"`
	TaskCount       int    `json:"taskCount,omitempty"`
	QueuePosition   int    `json:"queuePosition,omitempty"`
	Ephemeral       bool   `json:"ephemeral,omitempty"`
	InstalledByRun  bool   `json:"installedByRun,omitempty"`
	RuntimeState    string `json:"runtimeState,omitempty"`
	RuntimeMode     string `json:"runtimeMode,omitempty"`
	Health          string `json:"health,omitempty"`
	DriftState      string `json:"driftState,omitempty"`
	LastSyncStatus  string `json:"lastSyncStatus,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	LeaseState      string `json:"leaseState,omitempty"`
	Stale           bool   `json:"stale,omitempty"`
	StaleReason     string `json:"staleReason,omitempty"`
	LeaseAgeSec     int64  `json:"leaseAgeSec,omitempty"`
	HeartbeatAgeSec int64  `json:"heartbeatAgeSec,omitempty"`
	LeaseExpireAt   string `json:"leaseExpireAt,omitempty"`
	LastHeartbeatAt string `json:"lastHeartbeatAt,omitempty"`
	HeartbeatAt     string `json:"heartbeatAt,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

type OrchestratorWorkerInventorySummary struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Busy   int `json:"busy"`
	Error  int `json:"error"`
	Local  int `json:"local"`
	Remote int `json:"remote"`
	Stale  int `json:"stale,omitempty"`
}

type OrchestratorWorkerQueueSummary struct {
	ActiveExecutions   int    `json:"activeExecutions"`
	QueuedTasks        int    `json:"queuedTasks"`
	StaleLeases        int    `json:"staleLeases"`
	ReclaimableWorkers int    `json:"reclaimableWorkers"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
}

func normalizeOrchestratorExecutionForStore(in OrchestratorExecution) OrchestratorExecution {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	out.Mode = normalizeOrchestratorExecutionMode(out.Mode)
	out.Work = normalizeOrchestratorExecutionWorkContext(out.Work)
	out.Goal = strings.TrimSpace(out.Goal)
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
	if len(out.MemoryProvenance) == 0 {
		out.MemoryProvenance = append([]string(nil), out.RequiredMemory...)
	}
	if out.MemoryContractDigest == "" {
		out.MemoryContractDigest = buildMemoryContractDigest(out.MemoryProvenance)
	}
	out.IdempotencyKey = strings.TrimSpace(out.IdempotencyKey)
	out.ParentExecutionID = strings.TrimSpace(out.ParentExecutionID)
	out.SourceExecutionID = strings.TrimSpace(out.SourceExecutionID)
	out.LaunchReason = strings.TrimSpace(out.LaunchReason)
	out.SharedInstructions = baseagent.NormalizeSharedInstructions(out.SharedInstructions)
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
	out.Outcome = normalizeOrchestratorExecutionOutcome(out.Outcome)
	if out.Results == nil {
		out.Results = []OrchestratorTaskResult{}
	} else {
		results := make([]OrchestratorTaskResult, len(out.Results))
		for i := range out.Results {
			results[i] = normalizeOrchestratorTaskResult(out.Results[i])
		}
		out.Results = results
	}
	out.Error = strings.TrimSpace(out.Error)
	out.Governance = hydrateProviderGovernanceUsage(out)
	out.Policy = buildOrchestratorExecutionPolicySnapshot(out)
	out.RuntimeContextManifest = mergeOrchestratorRuntimeContextManifest(buildOrchestratorExecutionRuntimeContextManifest(out), out.RuntimeContextManifest)
	out.Guardrails = normalizeStoredOrchestratorExecutionGuardrails(out)
	return out
}

func normalizeOrchestratorWorkerLeaseForStore(in OrchestratorWorkerLease) OrchestratorWorkerLease {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	out.ExecutionID = strings.TrimSpace(out.ExecutionID)
	out.HostID = strings.TrimSpace(out.HostID)
	out.AgentID = strings.TrimSpace(out.AgentID)
	out.LeaseState = strings.TrimSpace(out.LeaseState)
	out.LastError = strings.TrimSpace(out.LastError)
	out.StaleReason = strings.TrimSpace(out.StaleReason)
	out.LastHeartbeatAt = strings.TrimSpace(out.LastHeartbeatAt)
	if out.QueuePosition < 0 {
		out.QueuePosition = 0
	}
	if out.State == "" {
		out.State = OrchestratorWorkerStateProvisioning
	}
	if out.LeaseState == "" {
		out.LeaseState = string(out.State)
	}
	if out.LastHeartbeatAt == "" {
		out.LastHeartbeatAt = strings.TrimSpace(out.HeartbeatAt)
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
