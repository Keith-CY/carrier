package integration

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

type BindingStatus string

const (
	BindingStatusDraft   BindingStatus = "draft"
	BindingStatusActive  BindingStatus = "active"
	BindingStatusRevoked BindingStatus = "revoked"
)

type TokenStatus string

const (
	TokenStatusActive  TokenStatus = "active"
	TokenStatusRevoked TokenStatus = "revoked"
)

type BindingTarget struct {
	HostID        string `json:"hostId"`
	AgentID       string `json:"agentId"`
	Backend       string `json:"backend"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

type Binding struct {
	ID             string        `json:"id"`
	Adapter        string        `json:"adapter"`
	Account        string        `json:"account"`
	CallbackURL    string        `json:"callbackUrl,omitempty"`
	CallbackKeyID  string        `json:"callbackKeyId,omitempty"`
	CallbackSecret string        `json:"callbackSecret,omitempty"`
	Target         BindingTarget `json:"target"`
	Status         BindingStatus `json:"status"`
	Capabilities   []string      `json:"capabilities,omitempty"`
	CreatedAt      string        `json:"createdAt,omitempty"`
	UpdatedAt      string        `json:"updatedAt,omitempty"`
}

type BindingToken struct {
	ID          string      `json:"id"`
	BindingID   string      `json:"bindingId"`
	Adapter     string      `json:"adapter"`
	TokenPrefix string      `json:"tokenPrefix"`
	TokenHash   string      `json:"tokenHash"`
	IssuedBy    string      `json:"issuedBy,omitempty"`
	Status      TokenStatus `json:"status"`
	CreatedAt   string      `json:"createdAt,omitempty"`
	UpdatedAt   string      `json:"updatedAt,omitempty"`
	LastUsedAt  string      `json:"lastUsedAt,omitempty"`
}

type BindingHealth struct {
	Healthy       bool   `json:"healthy"`
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

type VerifyBindingRequest struct {
	HostID                string   `json:"hostId"`
	AgentID               string   `json:"agentId"`
	Backend               string   `json:"backend"`
	WorkspaceRoot         string   `json:"workspaceRoot"`
	SupportedCapabilities []string `json:"supportedCapabilities,omitempty"`
}

type VerifyBindingResult struct {
	Verified         bool          `json:"verified"`
	Health           BindingHealth `json:"health"`
	VersionValue     string        `json:"versionValue,omitempty"`
	ResolvedHostID   string        `json:"resolvedHostId,omitempty"`
	ResolvedAgentID  string        `json:"resolvedAgentId,omitempty"`
	ResolvedBackend  string        `json:"resolvedBackend,omitempty"`
	Capabilities     []string      `json:"capabilities,omitempty"`
	BindingID        string        `json:"bindingId,omitempty"`
	BindingAccountID string        `json:"bindingAccountId,omitempty"`
}

type ExecutionState string

const (
	ExecutionStateAccepted       ExecutionState = "accepted"
	ExecutionStateRunning        ExecutionState = "running"
	ExecutionStatePauseRequested ExecutionState = "pause_requested"
	ExecutionStatePaused         ExecutionState = "paused"
	ExecutionStateCompleted      ExecutionState = "completed"
	ExecutionStateFailed         ExecutionState = "failed"
	ExecutionStateCancelled      ExecutionState = "cancelled"
)

type ActionType string

const (
	ActionTypePause  ActionType = "pause"
	ActionTypeResume ActionType = "resume"
	ActionTypeCancel ActionType = "cancel"
)

type ActionState string

const (
	ActionStateAccepted ActionState = "accepted"
	ActionStateApplied  ActionState = "applied"
	ActionStateRejected ActionState = "rejected"
)

type CreateExecutionRequest struct {
	ExternalExecutionID string   `json:"externalExecutionId,omitempty"`
	IdempotencyKey      string   `json:"idempotencyKey"`
	Goal                string   `json:"goal"`
	Input               string   `json:"input,omitempty"`
	RequestedProvider   string   `json:"requestedProvider,omitempty"`
	RequiredMemory      []string `json:"requiredMemory,omitempty"`
	DistillOutputs      []string `json:"distillOutputs,omitempty"`
	MaxConcurrency      int      `json:"maxConcurrency,omitempty"`
}

type Execution struct {
	ID                      string         `json:"carrierExecutionId"`
	BindingID               string         `json:"bindingId"`
	Adapter                 string         `json:"adapter"`
	Account                 string         `json:"account"`
	ExternalExecutionID     string         `json:"externalExecutionId,omitempty"`
	OrchestratorExecutionID string         `json:"orchestratorExecutionId,omitempty"`
	IdempotencyKey          string         `json:"idempotencyKey,omitempty"`
	State                   ExecutionState `json:"state"`
	Goal                    string         `json:"goal,omitempty"`
	Input                   string         `json:"input,omitempty"`
	RequestedProvider       string         `json:"requestedProvider,omitempty"`
	FailureCategory         string         `json:"failureCategory,omitempty"`
	FailureCode             string         `json:"failureCode,omitempty"`
	CurrentAttemptID        string         `json:"currentAttemptId,omitempty"`
	CreatedAt               string         `json:"createdAt,omitempty"`
	StartedAt               string         `json:"startedAt,omitempty"`
	CompletedAt             string         `json:"completedAt,omitempty"`
	UpdatedAt               string         `json:"updatedAt,omitempty"`
}

type Attempt struct {
	ID          string `json:"attemptId"`
	ExecutionID string `json:"carrierExecutionId"`
	Number      int    `json:"number"`
	CreatedAt   string `json:"createdAt,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

type Action struct {
	ID             string      `json:"id"`
	ExecutionID    string      `json:"carrierExecutionId"`
	Type           ActionType  `json:"type"`
	IdempotencyKey string      `json:"idempotencyKey,omitempty"`
	Reason         string      `json:"reason,omitempty"`
	State          ActionState `json:"state"`
	CreatedAt      string      `json:"createdAt,omitempty"`
	UpdatedAt      string      `json:"updatedAt,omitempty"`
}

type ActionRequest struct {
	Type           ActionType `json:"type"`
	IdempotencyKey string     `json:"idempotencyKey,omitempty"`
	Reason         string     `json:"reason,omitempty"`
}

type Event struct {
	ID          string `json:"eventId"`
	ExecutionID string `json:"carrierExecutionId"`
	AttemptID   string `json:"attemptId,omitempty"`
	Sequence    int64  `json:"sequence"`
	EventType   string `json:"eventType"`
	PayloadJSON string `json:"payloadJson,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

type UsageProof struct {
	ID          string `json:"id"`
	ExecutionID string `json:"carrierExecutionId"`
	ProofRef    string `json:"proofRef"`
	MeterRef    string `json:"meterRef,omitempty"`
	UsageKind   string `json:"usageKind"`
	AmountCents int64  `json:"amountCents"`
	Digest      string `json:"digest,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

type ArtifactRef struct {
	ID          string `json:"id"`
	ExecutionID string `json:"carrierExecutionId"`
	ArtifactRef string `json:"artifactRef"`
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	URL         string `json:"url,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

type CallbackDelivery struct {
	ID            string `json:"id"`
	EventID       string `json:"eventId"`
	ExecutionID   string `json:"carrierExecutionId"`
	BindingID     string `json:"bindingId"`
	CallbackURL   string `json:"callbackUrl"`
	Status        string `json:"status"`
	AttemptCount  int    `json:"attemptCount"`
	LastError     string `json:"lastError,omitempty"`
	NextAttemptAt string `json:"nextAttemptAt,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

func NormalizeBinding(in Binding) (Binding, error) {
	out := in
	var err error
	out.ID, err = ensurePrefixedID("bind", "binding id", out.ID)
	if err != nil {
		return Binding{}, err
	}
	out.Adapter = normalizeIdentifier(out.Adapter)
	out.Account = strings.TrimSpace(out.Account)
	out.CallbackURL = strings.TrimSpace(out.CallbackURL)
	out.CallbackKeyID = strings.TrimSpace(out.CallbackKeyID)
	out.CallbackSecret = strings.TrimSpace(out.CallbackSecret)
	out.Target = normalizeBindingTarget(out.Target)
	out.Status = normalizeBindingStatus(out.Status)
	out.Capabilities = normalizeCapabilities(out.Capabilities)
	out.CreatedAt = strings.TrimSpace(out.CreatedAt)
	out.UpdatedAt = strings.TrimSpace(out.UpdatedAt)

	if out.Adapter == "" {
		return Binding{}, fmt.Errorf("binding adapter is required")
	}
	if out.Account == "" {
		return Binding{}, fmt.Errorf("binding account is required")
	}
	if err := validateBindingTarget(out.Target); err != nil {
		return Binding{}, err
	}
	if out.Status == "" {
		out.Status = BindingStatusDraft
	}
	return out, nil
}

func NormalizeVerifyBindingRequest(in VerifyBindingRequest) (VerifyBindingRequest, error) {
	out := in
	out.HostID = normalizeIdentifier(out.HostID)
	out.AgentID = normalizeIdentifier(out.AgentID)
	out.Backend = normalizeIdentifier(out.Backend)
	out.WorkspaceRoot = normalizeWorkspaceRoot(out.WorkspaceRoot)
	out.SupportedCapabilities = normalizeCapabilities(out.SupportedCapabilities)
	if err := validateBindingTarget(BindingTarget{
		HostID:        out.HostID,
		AgentID:       out.AgentID,
		Backend:       out.Backend,
		WorkspaceRoot: out.WorkspaceRoot,
	}); err != nil {
		return VerifyBindingRequest{}, err
	}
	return out, nil
}

func NormalizeBindingToken(in BindingToken) (BindingToken, error) {
	out := in
	var err error
	out.ID, err = ensurePrefixedID("btk", "binding token id", out.ID)
	if err != nil {
		return BindingToken{}, err
	}
	out.BindingID = normalizeIdentifier(out.BindingID)
	out.Adapter = normalizeIdentifier(out.Adapter)
	out.TokenPrefix = strings.TrimSpace(out.TokenPrefix)
	out.TokenHash = strings.TrimSpace(out.TokenHash)
	out.IssuedBy = strings.TrimSpace(out.IssuedBy)
	out.Status = normalizeTokenStatus(out.Status)
	out.CreatedAt = strings.TrimSpace(out.CreatedAt)
	out.UpdatedAt = strings.TrimSpace(out.UpdatedAt)
	out.LastUsedAt = strings.TrimSpace(out.LastUsedAt)
	if out.BindingID == "" {
		return BindingToken{}, fmt.Errorf("binding token bindingId is required")
	}
	if out.Adapter == "" {
		return BindingToken{}, fmt.Errorf("binding token adapter is required")
	}
	if out.TokenPrefix == "" {
		return BindingToken{}, fmt.Errorf("binding token prefix is required")
	}
	if out.TokenHash == "" {
		return BindingToken{}, fmt.Errorf("binding token hash is required")
	}
	if out.Status == "" {
		out.Status = TokenStatusActive
	}
	return out, nil
}

func NormalizeCreateExecutionRequest(in CreateExecutionRequest) (CreateExecutionRequest, error) {
	out := in
	out.ExternalExecutionID = strings.TrimSpace(out.ExternalExecutionID)
	out.IdempotencyKey = strings.TrimSpace(out.IdempotencyKey)
	out.Goal = strings.TrimSpace(out.Goal)
	out.Input = strings.TrimSpace(out.Input)
	out.RequestedProvider = strings.TrimSpace(out.RequestedProvider)
	out.RequiredMemory = normalizeCapabilities(out.RequiredMemory)
	out.DistillOutputs = normalizeCapabilities(out.DistillOutputs)
	if out.IdempotencyKey == "" {
		return CreateExecutionRequest{}, fmt.Errorf("idempotencyKey is required")
	}
	if out.Goal == "" {
		return CreateExecutionRequest{}, fmt.Errorf("goal is required")
	}
	if out.Input == "" {
		out.Input = out.Goal
	}
	if out.MaxConcurrency < 0 {
		out.MaxConcurrency = 0
	}
	return out, nil
}

func NormalizeActionRequest(in ActionRequest) (ActionRequest, error) {
	out := in
	out.Type = normalizeActionType(out.Type)
	out.IdempotencyKey = strings.TrimSpace(out.IdempotencyKey)
	out.Reason = strings.TrimSpace(out.Reason)
	if out.Type == "" {
		return ActionRequest{}, fmt.Errorf("action type is required")
	}
	return out, nil
}

func GenerateTokenRaw(prefix string) (string, error) {
	suffix, err := randomHex(16)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(prefix) + suffix, nil
}

func normalizeBindingTarget(in BindingTarget) BindingTarget {
	return BindingTarget{
		HostID:        normalizeIdentifier(in.HostID),
		AgentID:       normalizeIdentifier(in.AgentID),
		Backend:       normalizeIdentifier(in.Backend),
		WorkspaceRoot: normalizeWorkspaceRoot(in.WorkspaceRoot),
	}
}

func validateBindingTarget(target BindingTarget) error {
	if target.HostID == "" {
		return fmt.Errorf("binding target hostId is required")
	}
	if target.AgentID == "" {
		return fmt.Errorf("binding target agentId is required")
	}
	if target.Backend == "" {
		return fmt.Errorf("binding target backend is required")
	}
	if target.WorkspaceRoot == "" {
		return fmt.Errorf("binding target workspaceRoot is required")
	}
	return nil
}

func normalizeBindingStatus(in BindingStatus) BindingStatus {
	switch strings.ToLower(strings.TrimSpace(string(in))) {
	case string(BindingStatusDraft):
		return BindingStatusDraft
	case string(BindingStatusActive):
		return BindingStatusActive
	case string(BindingStatusRevoked):
		return BindingStatusRevoked
	default:
		return ""
	}
}

func normalizeTokenStatus(in TokenStatus) TokenStatus {
	switch strings.ToLower(strings.TrimSpace(string(in))) {
	case string(TokenStatusActive):
		return TokenStatusActive
	case string(TokenStatusRevoked):
		return TokenStatusRevoked
	default:
		return ""
	}
}

func normalizeExecutionState(in ExecutionState) ExecutionState {
	switch strings.ToLower(strings.TrimSpace(string(in))) {
	case string(ExecutionStateAccepted):
		return ExecutionStateAccepted
	case string(ExecutionStateRunning):
		return ExecutionStateRunning
	case string(ExecutionStatePauseRequested):
		return ExecutionStatePauseRequested
	case string(ExecutionStatePaused):
		return ExecutionStatePaused
	case string(ExecutionStateCompleted):
		return ExecutionStateCompleted
	case string(ExecutionStateFailed):
		return ExecutionStateFailed
	case string(ExecutionStateCancelled):
		return ExecutionStateCancelled
	default:
		return ""
	}
}

func normalizeActionType(in ActionType) ActionType {
	switch strings.ToLower(strings.TrimSpace(string(in))) {
	case string(ActionTypePause):
		return ActionTypePause
	case string(ActionTypeResume):
		return ActionTypeResume
	case string(ActionTypeCancel):
		return ActionTypeCancel
	default:
		return ""
	}
}

func normalizeCapabilities(in []string) []string {
	set := make(map[string]struct{}, len(in))
	for _, item := range in {
		normalized := normalizeIdentifier(item)
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	slices.Sort(out)
	return out
}

func normalizeIdentifier(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	return strings.Join(strings.Fields(trimmed), "-")
}

func normalizeWorkspaceRoot(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

func ensurePrefixedID(prefix, label, raw string) (string, error) {
	if trimmed := normalizeIdentifier(raw); trimmed != "" {
		return trimmed, nil
	}
	suffix, err := randomHex(8)
	if err != nil {
		return "", fmt.Errorf("generate %s: %w", label, err)
	}
	return prefix + "_" + suffix, nil
}

func randomHex(bytesN int) (string, error) {
	buf := make([]byte, bytesN)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
