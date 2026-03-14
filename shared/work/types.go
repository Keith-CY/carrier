package work

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

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
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	SourceType    SourceType   `json:"sourceType"`
	SourceRef     string       `json:"sourceRef"`
	DefaultBranch string       `json:"defaultBranch,omitempty"`
	WorkflowPath  string       `json:"workflowPath,omitempty"`
	WorkflowDigest string      `json:"workflowDigest,omitempty"`
	State         ProjectState `json:"state"`
	LastSyncAt    string       `json:"lastSyncAt,omitempty"`
	LastSyncError string       `json:"lastSyncError,omitempty"`
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

func NormalizeProject(in Project) (Project, error) {
	out := in
	var err error
	out.ID, err = ensurePrefixedID("proj", out.ID)
	if err != nil {
		return Project{}, err
	}
	out.Name = strings.TrimSpace(out.Name)
	out.SourceType = normalizeSourceType(out.SourceType)
	out.SourceRef = strings.TrimSpace(out.SourceRef)
	out.DefaultBranch = strings.TrimSpace(out.DefaultBranch)
	out.WorkflowPath = strings.TrimSpace(out.WorkflowPath)
	out.WorkflowDigest = strings.TrimSpace(out.WorkflowDigest)
	out.State = normalizeProjectState(out.State)
	out.LastSyncAt = strings.TrimSpace(out.LastSyncAt)
	out.LastSyncError = strings.TrimSpace(out.LastSyncError)

	if out.Name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}
	if out.SourceType == "" {
		return Project{}, fmt.Errorf("project sourceType is required")
	}
	if out.SourceRef == "" {
		return Project{}, fmt.Errorf("project sourceRef is required")
	}
	if out.DefaultBranch == "" {
		out.DefaultBranch = "main"
	}
	if out.WorkflowPath == "" {
		out.WorkflowPath = DefaultWorkflowPath
	}
	if out.State == "" {
		out.State = ProjectStateRegistered
	}
	return out, nil
}

func NormalizeWorkItem(in WorkItem) (WorkItem, error) {
	out := in
	var err error
	out.ID, err = ensurePrefixedID("work", out.ID)
	if err != nil {
		return WorkItem{}, err
	}
	out.ProjectID = strings.TrimSpace(out.ProjectID)
	out.Title = strings.TrimSpace(out.Title)
	out.Description = strings.TrimSpace(out.Description)
	out.Acceptance = normalizeStringList(out.Acceptance)
	out.Priority = normalizeWorkPriority(out.Priority)
	out.Source = normalizeWorkSource(out.Source)
	out.SourceRef = strings.TrimSpace(out.SourceRef)
	out.Labels = normalizeStringList(out.Labels)
	out.State = normalizeWorkItemState(out.State)
	out.ClaimedByRunID = strings.TrimSpace(out.ClaimedByRunID)
	out.LatestRunID = strings.TrimSpace(out.LatestRunID)
	out.CreatedAt = strings.TrimSpace(out.CreatedAt)
	out.UpdatedAt = strings.TrimSpace(out.UpdatedAt)

	if out.ProjectID == "" {
		return WorkItem{}, fmt.Errorf("work item projectId is required")
	}
	if out.Title == "" {
		return WorkItem{}, fmt.Errorf("work item title is required")
	}
	if out.Priority == "" {
		out.Priority = WorkPriorityNormal
	}
	if out.Source == "" {
		out.Source = WorkSourceLocal
	}
	if out.SourceRef == "" && out.Source == WorkSourceLocal {
		out.SourceRef = "local:manual"
	}
	if out.State == "" {
		out.State = WorkItemStateNew
	}
	return out, nil
}

func NormalizeRun(in Run) (Run, error) {
	out := in
	rawBackend := strings.TrimSpace(string(in.Backend))
	var err error
	out.ID, err = ensurePrefixedID("run", out.ID)
	if err != nil {
		return Run{}, err
	}
	out.ProjectID = strings.TrimSpace(out.ProjectID)
	out.WorkItemID = strings.TrimSpace(out.WorkItemID)
	out.ExecutionID = strings.TrimSpace(out.ExecutionID)
	out.WorkspaceID = strings.TrimSpace(out.WorkspaceID)
	out.WorkspacePath = strings.TrimSpace(out.WorkspacePath)
	out.Backend = normalizeRunBackend(out.Backend)
	out.Phase = normalizeRunPhase(out.Phase)
	out.LeaseOwner = strings.TrimSpace(out.LeaseOwner)
	out.LeaseExpiresAt = strings.TrimSpace(out.LeaseExpiresAt)
	out.VerificationStatus = normalizeVerificationStatus(out.VerificationStatus)
	out.PublishStatus = normalizePublishStatus(out.PublishStatus)
	out.WorkflowDigest = strings.TrimSpace(out.WorkflowDigest)
	out.CreatedAt = strings.TrimSpace(out.CreatedAt)
	out.UpdatedAt = strings.TrimSpace(out.UpdatedAt)

	if out.ProjectID == "" {
		return Run{}, fmt.Errorf("run projectId is required")
	}
	if out.WorkItemID == "" {
		return Run{}, fmt.Errorf("run workItemId is required")
	}
	if rawBackend != "" && out.Backend == "" {
		return Run{}, fmt.Errorf("run backend %q is invalid", rawBackend)
	}
	if out.Backend == "" {
		out.Backend = RunBackendLocalSandboxed
	}
	if out.Phase == "" {
		out.Phase = RunPhaseCreated
	}
	if out.VerificationStatus == "" {
		out.VerificationStatus = VerificationStatusPending
	}
	if out.PublishStatus == "" {
		out.PublishStatus = PublishStatusPending
	}
	return out, nil
}

func ensurePrefixedID(prefix string, current string) (string, error) {
	trimmed := strings.TrimSpace(current)
	if trimmed != "" {
		return trimmed, nil
	}
	suffix, err := randomID()
	if err != nil {
		return "", err
	}
	return prefix + "_" + suffix, nil
}

var randomIDRead = rand.Read

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := randomIDRead(buf); err != nil {
		return "", fmt.Errorf("failed to read random bytes for id generation: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func normalizeSourceType(in SourceType) SourceType {
	switch SourceType(strings.ToLower(strings.TrimSpace(string(in)))) {
	case SourceTypeGitHub:
		return SourceTypeGitHub
	case SourceTypeGit:
		return SourceTypeGit
	case SourceTypeLocal:
		return SourceTypeLocal
	default:
		return ""
	}
}

func normalizeProjectState(in ProjectState) ProjectState {
	switch ProjectState(strings.ToLower(strings.TrimSpace(string(in)))) {
	case ProjectStateRegistered:
		return ProjectStateRegistered
	case ProjectStateSyncing:
		return ProjectStateSyncing
	case ProjectStateReady:
		return ProjectStateReady
	case ProjectStateDrifted:
		return ProjectStateDrifted
	case ProjectStateError:
		return ProjectStateError
	case ProjectStateArchived:
		return ProjectStateArchived
	default:
		return ""
	}
}

func normalizeWorkPriority(in WorkPriority) WorkPriority {
	switch WorkPriority(strings.ToLower(strings.TrimSpace(string(in)))) {
	case WorkPriorityLow:
		return WorkPriorityLow
	case WorkPriorityNormal:
		return WorkPriorityNormal
	case WorkPriorityHigh:
		return WorkPriorityHigh
	case WorkPriorityUrgent:
		return WorkPriorityUrgent
	default:
		return ""
	}
}

func normalizeWorkSource(in WorkSource) WorkSource {
	switch WorkSource(strings.ToLower(strings.TrimSpace(string(in)))) {
	case WorkSourceLocal:
		return WorkSourceLocal
	case WorkSourceGitHub:
		return WorkSourceGitHub
	default:
		return ""
	}
}

func normalizeWorkItemState(in WorkItemState) WorkItemState {
	switch WorkItemState(strings.ToLower(strings.TrimSpace(string(in)))) {
	case WorkItemStateNew:
		return WorkItemStateNew
	case WorkItemStateTriaged:
		return WorkItemStateTriaged
	case WorkItemStateQueued:
		return WorkItemStateQueued
	case WorkItemStateClaimed:
		return WorkItemStateClaimed
	case WorkItemStateRunning:
		return WorkItemStateRunning
	case WorkItemStateBlocked:
		return WorkItemStateBlocked
	case WorkItemStateAwaitingReview:
		return WorkItemStateAwaitingReview
	case WorkItemStateDone:
		return WorkItemStateDone
	case WorkItemStateCancelled:
		return WorkItemStateCancelled
	default:
		return ""
	}
}

func normalizeRunBackend(in RunBackend) RunBackend {
	switch RunBackend(strings.ToLower(strings.TrimSpace(string(in)))) {
	case RunBackendLocalSandboxed:
		return RunBackendLocalSandboxed
	case RunBackendManaged:
		return RunBackendManaged
	case RunBackendRemoteVM:
		return RunBackendRemoteVM
	default:
		return ""
	}
}

func normalizeRunPhase(in RunPhase) RunPhase {
	switch RunPhase(strings.ToLower(strings.TrimSpace(string(in)))) {
	case RunPhaseCreated:
		return RunPhaseCreated
	case RunPhasePreparing:
		return RunPhasePreparing
	case RunPhaseReady:
		return RunPhaseReady
	case RunPhaseExecuting:
		return RunPhaseExecuting
	case RunPhaseVerifying:
		return RunPhaseVerifying
	case RunPhasePublishing:
		return RunPhasePublishing
	case RunPhaseCompleted:
		return RunPhaseCompleted
	case RunPhaseFailed:
		return RunPhaseFailed
	case RunPhaseCancelled:
		return RunPhaseCancelled
	case RunPhaseStale:
		return RunPhaseStale
	default:
		return ""
	}
}

func normalizeVerificationStatus(in VerificationStatus) VerificationStatus {
	switch VerificationStatus(strings.ToLower(strings.TrimSpace(string(in)))) {
	case VerificationStatusPending:
		return VerificationStatusPending
	case VerificationStatusPassed:
		return VerificationStatusPassed
	case VerificationStatusFailed:
		return VerificationStatusFailed
	case VerificationStatusSkipped:
		return VerificationStatusSkipped
	default:
		return ""
	}
}

func normalizePublishStatus(in PublishStatus) PublishStatus {
	switch PublishStatus(strings.ToLower(strings.TrimSpace(string(in)))) {
	case PublishStatusPending:
		return PublishStatusPending
	case PublishStatusPublished:
		return PublishStatusPublished
	case PublishStatusFailed:
		return PublishStatusFailed
	case PublishStatusSkipped:
		return PublishStatusSkipped
	default:
		return ""
	}
}

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, value := range in {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
