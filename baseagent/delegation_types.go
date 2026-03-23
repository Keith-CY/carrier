package baseagent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type DelegationContextKind string

const (
	DelegationContextKindRepo     DelegationContextKind = "repo_context"
	DelegationContextKindRuntime  DelegationContextKind = "runtime_context"
	DelegationContextKindExternal DelegationContextKind = "external_context"
	DelegationContextKindArtifact DelegationContextKind = "agent_artifact"
)

type DelegationContextStatus string

const (
	DelegationContextStatusPending     DelegationContextStatus = "pending"
	DelegationContextStatusFulfilled   DelegationContextStatus = "fulfilled"
	DelegationContextStatusPartial     DelegationContextStatus = "partial"
	DelegationContextStatusDenied      DelegationContextStatus = "denied"
	DelegationContextStatusUnavailable DelegationContextStatus = "unavailable"
	DelegationContextStatusTimedOut    DelegationContextStatus = "timed_out"
)

type DelegationBudget struct {
	MaxContextRequests int `json:"maxContextRequests,omitempty"`
	MaxDurationSec     int `json:"maxDurationSec,omitempty"`
	MaxTokens          int `json:"maxTokens,omitempty"`
}

type DelegationMemoryView struct {
	SnapshotID string   `json:"snapshotId,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
}

type DelegationContract struct {
	ContractID         string               `json:"contractId"`
	ParentSessionID    string               `json:"parentSessionId,omitempty"`
	MasterAgentID      string               `json:"masterAgentId,omitempty"`
	SubagentID         string               `json:"subagentId,omitempty"`
	TaskGoal           string               `json:"taskGoal"`
	TaskInstructions   string               `json:"taskInstructions,omitempty"`
	ScopeVersion       string               `json:"scopeVersion,omitempty"`
	AllowedTools       []string             `json:"allowedTools,omitempty"`
	Budget             DelegationBudget     `json:"budget,omitempty"`
	ApprovalScope      string               `json:"approvalScope,omitempty"`
	MemoryView         DelegationMemoryView `json:"memoryView,omitempty"`
	ExpectedOutputs    []string             `json:"expectedOutputs,omitempty"`
	DegradedModePolicy string               `json:"degradedModePolicy,omitempty"`
}

type DelegationCitation struct {
	Kind  string `json:"kind,omitempty"`
	Label string `json:"label,omitempty"`
	URI   string `json:"uri,omitempty"`
}

type DelegationContextRequest struct {
	RequestID        string                  `json:"requestId"`
	ContractID       string                  `json:"contractId,omitempty"`
	Kind             DelegationContextKind   `json:"kind,omitempty"`
	Question         string                  `json:"question"`
	Reason           string                  `json:"reason,omitempty"`
	Required         bool                    `json:"required,omitempty"`
	RequestedSources []string                `json:"requestedSources,omitempty"`
	RequestedAt      time.Time               `json:"requestedAt"`
	Status           DelegationContextStatus `json:"status"`
}

type DelegationContextResponse struct {
	RequestID          string                  `json:"requestId"`
	Status             DelegationContextStatus `json:"status"`
	Summary            string                  `json:"summary,omitempty"`
	Citations          []DelegationCitation    `json:"citations,omitempty"`
	Attachments        []AttachmentRef         `json:"attachments,omitempty"`
	SourceDescriptors  []string                `json:"sourceDescriptors,omitempty"`
	MissingInformation []string                `json:"missingInformation,omitempty"`
	RespondedAt        time.Time               `json:"respondedAt"`
}

type DelegationOutcome struct {
	Summary        string   `json:"summary,omitempty"`
	Result         string   `json:"result,omitempty"`
	Artifacts      []string `json:"artifacts,omitempty"`
	OpenQuestions  []string `json:"openQuestions,omitempty"`
	MissingContext []string `json:"missingContext,omitempty"`
	Degraded       bool     `json:"degraded,omitempty"`
	Confidence     string   `json:"confidence,omitempty"`
}

type delegationContextBroker interface {
	Request(context.Context, DelegationContextRequest) (DelegationContextResponse, error)
}

type delegationContextBrokerKey struct{}

func withDelegationContextBroker(ctx context.Context, broker delegationContextBroker) context.Context {
	if broker == nil {
		return ctx
	}
	return context.WithValue(ctx, delegationContextBrokerKey{}, broker)
}

func RequestDelegationContext(ctx context.Context, req DelegationContextRequest) (DelegationContextResponse, error) {
	if ctx == nil {
		return DelegationContextResponse{}, fmt.Errorf("context is required")
	}
	broker, ok := ctx.Value(delegationContextBrokerKey{}).(delegationContextBroker)
	if !ok || broker == nil {
		return DelegationContextResponse{}, fmt.Errorf("delegation context broker is unavailable")
	}
	return broker.Request(ctx, req)
}

func normalizeDelegationContract(contract *DelegationContract, fallbackTask string) *DelegationContract {
	normalized := cloneDelegationContract(contract)
	if normalized == nil {
		normalized = &DelegationContract{}
	}
	normalized.ContractID = strings.TrimSpace(normalized.ContractID)
	normalized.ParentSessionID = strings.TrimSpace(normalized.ParentSessionID)
	normalized.MasterAgentID = strings.TrimSpace(normalized.MasterAgentID)
	normalized.SubagentID = strings.TrimSpace(normalized.SubagentID)
	normalized.TaskGoal = firstNonEmptyString(normalized.TaskGoal, fallbackTask)
	normalized.TaskInstructions = strings.TrimSpace(normalized.TaskInstructions)
	normalized.ScopeVersion = firstNonEmptyString(normalized.ScopeVersion, "v1")
	normalized.AllowedTools = trimStringList(normalized.AllowedTools)
	normalized.ApprovalScope = strings.TrimSpace(normalized.ApprovalScope)
	normalized.MemoryView.SnapshotID = strings.TrimSpace(normalized.MemoryView.SnapshotID)
	normalized.MemoryView.Scopes = trimStringList(normalized.MemoryView.Scopes)
	normalized.ExpectedOutputs = trimStringList(normalized.ExpectedOutputs)
	normalized.DegradedModePolicy = firstNonEmptyString(normalized.DegradedModePolicy, "continue_with_missing_context")
	return normalized
}

func cloneDelegationContract(contract *DelegationContract) *DelegationContract {
	if contract == nil {
		return nil
	}
	cloned := *contract
	cloned.AllowedTools = trimStringList(contract.AllowedTools)
	cloned.MemoryView = DelegationMemoryView{
		SnapshotID: strings.TrimSpace(contract.MemoryView.SnapshotID),
		Scopes:     trimStringList(contract.MemoryView.Scopes),
	}
	cloned.ExpectedOutputs = trimStringList(contract.ExpectedOutputs)
	return &cloned
}

func normalizeDelegationContextRequest(req DelegationContextRequest, contractID string, now time.Time) DelegationContextRequest {
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.ContractID = firstNonEmptyString(strings.TrimSpace(req.ContractID), strings.TrimSpace(contractID))
	req.Kind = DelegationContextKind(strings.TrimSpace(string(req.Kind)))
	req.Question = strings.TrimSpace(req.Question)
	req.Reason = strings.TrimSpace(req.Reason)
	req.RequestedSources = trimStringList(req.RequestedSources)
	if req.RequestedAt.IsZero() {
		req.RequestedAt = now
	}
	if req.Status == "" {
		req.Status = DelegationContextStatusPending
	}
	return req
}

func cloneDelegationContextRequests(items []DelegationContextRequest) []DelegationContextRequest {
	if len(items) == 0 {
		return nil
	}
	out := make([]DelegationContextRequest, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeDelegationContextRequest(item, item.ContractID, item.RequestedAt))
	}
	return out
}

func normalizeDelegationContextResponse(resp DelegationContextResponse, requestID string, now time.Time) DelegationContextResponse {
	resp.RequestID = firstNonEmptyString(strings.TrimSpace(resp.RequestID), strings.TrimSpace(requestID))
	resp.Status = DelegationContextStatus(strings.TrimSpace(string(resp.Status)))
	if resp.Status == "" {
		resp.Status = DelegationContextStatusFulfilled
	}
	resp.Summary = strings.TrimSpace(resp.Summary)
	resp.Citations = cloneDelegationCitations(resp.Citations)
	resp.Attachments = cloneAttachmentRefs(resp.Attachments)
	resp.SourceDescriptors = trimStringList(resp.SourceDescriptors)
	resp.MissingInformation = trimStringList(resp.MissingInformation)
	if resp.RespondedAt.IsZero() {
		resp.RespondedAt = now
	}
	return resp
}

func cloneDelegationContextResponses(items []DelegationContextResponse) []DelegationContextResponse {
	if len(items) == 0 {
		return nil
	}
	out := make([]DelegationContextResponse, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeDelegationContextResponse(item, item.RequestID, item.RespondedAt))
	}
	return out
}

func cloneDelegationCitations(items []DelegationCitation) []DelegationCitation {
	if len(items) == 0 {
		return nil
	}
	out := make([]DelegationCitation, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.Label)
		uri := strings.TrimSpace(item.URI)
		kind := strings.TrimSpace(item.Kind)
		if label == "" && uri == "" && kind == "" {
			continue
		}
		out = append(out, DelegationCitation{
			Kind:  kind,
			Label: label,
			URI:   uri,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneDelegationOutcome(outcome *DelegationOutcome) *DelegationOutcome {
	if outcome == nil {
		return nil
	}
	cloned := *outcome
	cloned.Artifacts = trimStringList(outcome.Artifacts)
	cloned.OpenQuestions = trimStringList(outcome.OpenQuestions)
	cloned.MissingContext = trimStringList(outcome.MissingContext)
	return &cloned
}

func trimStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneSubagentJob(job SubagentJob) SubagentJob {
	job.Contract = cloneDelegationContract(job.Contract)
	job.ContextRequests = cloneDelegationContextRequests(job.ContextRequests)
	job.ContextResponses = cloneDelegationContextResponses(job.ContextResponses)
	job.MissingContext = trimStringList(job.MissingContext)
	job.Outcome = cloneDelegationOutcome(job.Outcome)
	job.Task = strings.TrimSpace(job.Task)
	job.Summary = strings.TrimSpace(job.Summary)
	job.Result = strings.TrimSpace(job.Result)
	job.Error = strings.TrimSpace(job.Error)
	job.Confidence = strings.TrimSpace(job.Confidence)
	return job
}
