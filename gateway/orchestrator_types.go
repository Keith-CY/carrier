package gateway

import (
	"fmt"
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
	HostID  string `json:"hostId"`
	AgentID string `json:"agentId"`
	Count   int    `json:"count"`
}

type OrchestratorTaskUnit struct {
	ID             string                 `json:"id"`
	Input          string                 `json:"input"`
	ExpectedSchema map[string]interface{} `json:"expectedSchema,omitempty"`
	TimeoutMs      int                    `json:"timeoutMs,omitempty"`
	RetryBudget    int                    `json:"retryBudget,omitempty"`
	ToolPolicy     string                 `json:"toolPolicy,omitempty"`
	HostID         string                 `json:"hostId,omitempty"`
	AgentID        string                 `json:"agentId,omitempty"`
	SessionID      string                 `json:"sessionId,omitempty"`
}

type OrchestratorAuthorization struct {
	InfrastructureApproved bool   `json:"infrastructureApproved"`
	ApprovedBy             string `json:"approvedBy,omitempty"`
	ApprovedAt             string `json:"approvedAt,omitempty"`
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
	ID              string                       `json:"id"`
	Goal            string                       `json:"goal"`
	IdempotencyKey  string                       `json:"idempotencyKey,omitempty"`
	ApprovalScope   string                       `json:"approvalScope"`
	ToolPolicy      OrchestratorToolPolicy       `json:"toolPolicy,omitempty"`
	RequiredWorkers []OrchestratorRequiredWorker `json:"requiredWorkers"`
	TaskUnits       []OrchestratorTaskUnit       `json:"taskUnits"`
	Status          OrchestratorExecutionStatus  `json:"status"`
	MaxConcurrency  int                          `json:"maxConcurrency,omitempty"`
	Authorization   OrchestratorAuthorization    `json:"authorization"`
	Results         []OrchestratorTaskResult     `json:"results,omitempty"`
	Error           string                       `json:"error,omitempty"`
	CreatedAt       string                       `json:"createdAt"`
	StartedAt       string                       `json:"startedAt,omitempty"`
	CompletedAt     string                       `json:"completedAt,omitempty"`
	UpdatedAt       string                       `json:"updatedAt"`
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

func normalizeOrchestratorExecutionForStore(in OrchestratorExecution) OrchestratorExecution {
	out := in
	out.ID = strings.TrimSpace(out.ID)
	out.Goal = strings.TrimSpace(out.Goal)
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
	if out.Results == nil {
		out.Results = []OrchestratorTaskResult{}
	}
	out.Error = strings.TrimSpace(out.Error)
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
		if worker.HostID == "" {
			return OrchestratorExecution{}, errOrchestratorValidation("requiredWorkers.hostId is required", i)
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
	out.ToolPolicy.Mode = strings.TrimSpace(out.ToolPolicy.Mode)
	if out.ToolPolicy.Mode == "" {
		out.ToolPolicy.Mode = "restricted"
	}
	return out, nil
}

func errOrchestratorValidation(message string, index int) error {
	if index >= 0 {
		return fmt.Errorf("item %d: %s", index, strings.TrimSpace(message))
	}
	return fmt.Errorf(strings.TrimSpace(message))
}
