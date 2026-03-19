package work

const DefaultWorkflowPath = "WORKFLOW.md"

type SourceType string

const (
	SourceTypeGitHub SourceType = "github"
	SourceTypeGit    SourceType = "git"
	SourceTypeLocal  SourceType = "local"
)

type ProjectState string

const (
	ProjectStateRegistered ProjectState = "registered"
	ProjectStateSyncing    ProjectState = "syncing"
	ProjectStateReady      ProjectState = "ready"
	ProjectStateDrifted    ProjectState = "drifted"
	ProjectStateError      ProjectState = "error"
	ProjectStateArchived   ProjectState = "archived"
)

type WorkPriority string

const (
	WorkPriorityLow    WorkPriority = "low"
	WorkPriorityNormal WorkPriority = "normal"
	WorkPriorityHigh   WorkPriority = "high"
	WorkPriorityUrgent WorkPriority = "urgent"
)

type WorkSource string

const (
	WorkSourceLocal  WorkSource = "local"
	WorkSourceGitHub WorkSource = "github"
)

type WorkItemState string

const (
	WorkItemStateNew            WorkItemState = "new"
	WorkItemStateTriaged        WorkItemState = "triaged"
	WorkItemStateQueued         WorkItemState = "queued"
	WorkItemStateClaimed        WorkItemState = "claimed"
	WorkItemStateRunning        WorkItemState = "running"
	WorkItemStateBlocked        WorkItemState = "blocked"
	WorkItemStateAwaitingReview WorkItemState = "awaiting_review"
	WorkItemStateDone           WorkItemState = "done"
	WorkItemStateCancelled      WorkItemState = "cancelled"
)

type RunBackend string

const (
	RunBackendLocalSandboxed RunBackend = "local_sandboxed"
	RunBackendManaged        RunBackend = "managed_isolated"
	RunBackendRemoteVM       RunBackend = "remote_vm"
)

type RunPhase string

const (
	RunPhaseCreated    RunPhase = "created"
	RunPhasePreparing  RunPhase = "preparing"
	RunPhaseReady      RunPhase = "ready"
	RunPhaseExecuting  RunPhase = "executing"
	RunPhaseVerifying  RunPhase = "verifying"
	RunPhasePublishing RunPhase = "publishing"
	RunPhaseCompleted  RunPhase = "completed"
	RunPhaseFailed     RunPhase = "failed"
	RunPhaseCancelled  RunPhase = "cancelled"
	RunPhaseStale      RunPhase = "stale"
)

type VerificationStatus string

const (
	VerificationStatusPending VerificationStatus = "pending"
	VerificationStatusPassed  VerificationStatus = "passed"
	VerificationStatusFailed  VerificationStatus = "failed"
	VerificationStatusSkipped VerificationStatus = "skipped"
)

type PublishStatus string

const (
	PublishStatusPending   PublishStatus = "pending"
	PublishStatusPublished PublishStatus = "published"
	PublishStatusFailed    PublishStatus = "failed"
	PublishStatusSkipped   PublishStatus = "skipped"
)

type Project struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	SourceType     SourceType   `json:"sourceType"`
	SourceRef      string       `json:"sourceRef"`
	DefaultBranch  string       `json:"defaultBranch,omitempty"`
	WorkflowPath   string       `json:"workflowPath,omitempty"`
	WorkflowDigest string       `json:"workflowDigest,omitempty"`
	State          ProjectState `json:"state"`
	LastSyncAt     string       `json:"lastSyncAt,omitempty"`
	LastSyncError  string       `json:"lastSyncError,omitempty"`
}

type WorkItem struct {
	ID             string        `json:"id"`
	ProjectID      string        `json:"projectId"`
	Title          string        `json:"title"`
	Description    string        `json:"description,omitempty"`
	Acceptance     []string      `json:"acceptance,omitempty"`
	Priority       WorkPriority  `json:"priority"`
	Source         WorkSource    `json:"source"`
	SourceRef      string        `json:"sourceRef,omitempty"`
	Labels         []string      `json:"labels,omitempty"`
	State          WorkItemState `json:"state"`
	ClaimedByRunID string        `json:"claimedByRunId,omitempty"`
	LatestRunID    string        `json:"latestRunId,omitempty"`
	CreatedAt      string        `json:"createdAt,omitempty"`
	UpdatedAt      string        `json:"updatedAt,omitempty"`
}

type Run struct {
	ID                 string             `json:"id"`
	ProjectID          string             `json:"projectId"`
	WorkItemID         string             `json:"workItemId"`
	ExecutionID        string             `json:"executionId,omitempty"`
	WorkspaceID        string             `json:"workspaceId,omitempty"`
	WorkspacePath      string             `json:"workspacePath,omitempty"`
	Backend            RunBackend         `json:"backend"`
	Phase              RunPhase           `json:"phase"`
	LeaseOwner         string             `json:"leaseOwner,omitempty"`
	LeaseExpiresAt     string             `json:"leaseExpiresAt,omitempty"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	PublishStatus      PublishStatus      `json:"publishStatus"`
	WorkflowDigest     string             `json:"workflowDigest,omitempty"`
	CreatedAt          string             `json:"createdAt,omitempty"`
	UpdatedAt          string             `json:"updatedAt,omitempty"`
}
