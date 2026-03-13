package gateway

import (
	"bytes"
	"carrier/baseagent"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDaemonBaseURL = "http://127.0.0.1:9090"
	// Install/upgrade actions may run for many minutes on cold hosts.
	defaultDaemonTimeout  = 30 * time.Minute
	maxDaemonResponseSize = 32 * 1024 * 1024 // 32 MB
)

// AgentState mirrors the daemon's agent status.
type AgentState struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	InstallState         string            `json:"installState"`
	Runtime              string            `json:"runtimeState"`
	Health               string            `json:"health"`
	Memory               *AgentMemoryState `json:"memory,omitempty"`
	Heartbeat            *AgentHeartbeat   `json:"heartbeat,omitempty"`
	Ports                []int             `json:"ports"`
	StartedAt            *string           `json:"startedAt,omitempty"`
	RestartCount         int               `json:"restartCount"`
	NeedsRemoteDiagnosis bool              `json:"needsRemoteDiagnosis"`
	Isolated             bool              `json:"isolated"`
	LimaInstanceName     *string           `json:"limaInstanceName,omitempty"`
	LastError            *string           `json:"lastError,omitempty"`
	UpdatedAt            string            `json:"updatedAt"`
}

type AgentMemoryState struct {
	ContractID     string  `json:"contractId,omitempty"`
	ContractDigest string  `json:"contractDigest,omitempty"`
	SyncState      string  `json:"syncState,omitempty"`
	SyncError      string  `json:"syncError,omitempty"`
	SyncedAt       *string `json:"syncedAt,omitempty"`
}

type AgentHeartbeat struct {
	State          string  `json:"state"`
	AgeSeconds     int64   `json:"ageSeconds"`
	LastActivityAt *string `json:"lastActivityAt,omitempty"`
}

// LogsResult holds log lines from the daemon.
type LogsResult struct {
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"`
}

// UpgradeResult is returned by the daemon's upgrade endpoint.
type UpgradeResult struct {
	AgentID      string `json:"agentId"`
	FromVersion  string `json:"fromVersion"`
	ToVersion    string `json:"toVersion"`
	BackupPath   string `json:"backupPath,omitempty"`
	RollbackHint string `json:"rollbackHint,omitempty"`
}

// DiagnoseResult is returned by the daemon's diagnose endpoint.
type DiagnoseResult struct {
	ArtifactRef string `json:"artifactRef"`
}

// HandoffStatus is the status of a remote diagnosis handoff.
type HandoffStatus string

const (
	HandoffPending  HandoffStatus = "pending"
	HandoffDeclined HandoffStatus = "declined"
)

// RemoteDiagnosisHandoff is returned by the diagnosis handoff endpoint.
type RemoteDiagnosisHandoff struct {
	ID          string        `json:"id"`
	AgentID     string        `json:"agentId"`
	Consent     bool          `json:"consent"`
	ArtifactRef string        `json:"artifactRef"`
	Status      HandoffStatus `json:"status"`
	CreatedAt   string        `json:"createdAt"`
}

type BaseAgentChatResult = baseagent.ChatResponse
type BaseAgentDecomposeTask = baseagent.DecomposeTask

type AgentChatResult struct {
	AgentID     string                         `json:"agentId"`
	SessionID   string                         `json:"sessionId,omitempty"`
	Message     string                         `json:"message"`
	RichContent *baseagent.RichOutboundMessage `json:"richContent,omitempty"`
	Action      string                         `json:"action,omitempty"`
	SelfHealed  bool                           `json:"selfHealed,omitempty"`
	BackupRef   string                         `json:"backupRef,omitempty"`
}

type AgentCapabilitySummary = baseagent.RuntimeCapabilitySummary
type AgentSessionStats = baseagent.SessionStats
type CronJob = baseagent.CronJob

// DaemonClientError is returned when the daemon returns a non-2xx response.
type DaemonClientError struct {
	Code    string
	Message string
}

func (e *DaemonClientError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// IsRemoteDiagNotNeeded returns true for the specific error code.
func IsRemoteDiagNotNeeded(err error) bool {
	if de, ok := err.(*DaemonClientError); ok {
		return de.Code == "E_REMOTE_DIAG_NOT_NEEDED"
	}
	return false
}

// DaemonClient is the HTTP client to the daemon API.
type DaemonClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type InstallAgentOptions struct {
	Isolation bool `json:"isolation,omitempty"`
}

type StartAgentOptions struct {
	Isolation bool `json:"isolation,omitempty"`
}

// NewDaemonClient creates a new client.
func NewDaemonClient(baseURL, token string, timeout time.Duration) *DaemonClient {
	if baseURL == "" {
		baseURL = defaultDaemonBaseURL
	}
	if timeout <= 0 {
		timeout = defaultDaemonTimeout
	}
	return &DaemonClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ListAgents returns all agents.
func (c *DaemonClient) ListAgents(ctx context.Context, actor, requestID string) ([]AgentState, error) {
	var result struct {
		Agents []AgentState `json:"agents"`
	}
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/agents", nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	// May be array directly or wrapped
	if err := json.Unmarshal(raw, &result); err != nil {
		// Try direct array
		var arr []AgentState
		if err2 := json.Unmarshal(raw, &arr); err2 != nil {
			return nil, fmt.Errorf("list agents response: %w", err)
		}
		return arr, nil
	}
	if result.Agents != nil {
		return result.Agents, nil
	}
	// Try direct array
	var arr []AgentState
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	return []AgentState{}, nil
}

// InstallAgent installs an agent.
func (c *DaemonClient) InstallAgent(ctx context.Context, agentID, actor, requestID string) error {
	return c.InstallAgentWithOptions(ctx, agentID, InstallAgentOptions{}, actor, requestID)
}

// InstallAgentWithOptions installs an agent with optional runtime options.
func (c *DaemonClient) InstallAgentWithOptions(ctx context.Context, agentID string, opts InstallAgentOptions, actor, requestID string) error {
	var body interface{}
	if opts.Isolation {
		body = opts
	}
	_, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/install", body, actor, requestID)
	return err
}

// StartAgent starts an agent.
func (c *DaemonClient) StartAgent(ctx context.Context, agentID, actor, requestID string) error {
	return c.StartAgentWithOptions(ctx, agentID, StartAgentOptions{}, actor, requestID)
}

// StartAgentWithOptions starts an agent with optional runtime options.
func (c *DaemonClient) StartAgentWithOptions(ctx context.Context, agentID string, opts StartAgentOptions, actor, requestID string) error {
	var body interface{}
	if opts.Isolation {
		body = opts
	}
	_, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/start", body, actor, requestID)
	return err
}

// StopAgent stops an agent.
func (c *DaemonClient) StopAgent(ctx context.Context, agentID, actor, requestID string) error {
	_, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/stop", nil, actor, requestID)
	return err
}

// UninstallAgent uninstalls an agent.
func (c *DaemonClient) UninstallAgent(ctx context.Context, agentID, actor, requestID string) error {
	_, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/uninstall", nil, actor, requestID)
	return err
}

// GetStatus returns status for one agent (or all if agentID is empty).
func (c *DaemonClient) GetStatus(ctx context.Context, agentID, actor, requestID string) ([]AgentState, error) {
	var path string
	if agentID != "" {
		path = "/api/v1/agents/" + url.PathEscape(agentID) + "/status"
	} else {
		path = "/api/v1/agents/status"
	}
	raw, err := c.request(ctx, http.MethodGet, path, nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Statuses []AgentState `json:"statuses"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		var arr []AgentState
		if err2 := json.Unmarshal(raw, &arr); err2 != nil {
			return nil, fmt.Errorf("status response: %w", err)
		}
		return arr, nil
	}
	if wrapped.Statuses != nil {
		return wrapped.Statuses, nil
	}
	var single AgentState
	if err := json.Unmarshal(raw, &single); err == nil {
		if strings.TrimSpace(single.ID) != "" {
			return []AgentState{single}, nil
		}
	}
	var arr []AgentState
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	return []AgentState{}, nil
}

func (c *DaemonClient) GetAgentCapabilities(ctx context.Context, agentID, actor, requestID string) (AgentCapabilitySummary, error) {
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/agents/"+url.PathEscape(agentID)+"/capabilities", nil, actor, requestID)
	if err != nil {
		return AgentCapabilitySummary{}, err
	}
	var summary AgentCapabilitySummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return AgentCapabilitySummary{}, fmt.Errorf("capabilities response: %w", err)
	}
	return summary, nil
}

func (c *DaemonClient) SetAgentSkillEnabled(ctx context.Context, agentID, skillName string, enabled bool, actor, requestID string) (AgentCapabilitySummary, error) {
	body := map[string]bool{"enabled": enabled}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/skills/"+url.PathEscape(skillName), body, actor, requestID)
	if err != nil {
		return AgentCapabilitySummary{}, err
	}
	var summary AgentCapabilitySummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return AgentCapabilitySummary{}, fmt.Errorf("skill toggle response: %w", err)
	}
	return summary, nil
}

func (c *DaemonClient) SearchAgentSkills(ctx context.Context, agentID, query, actor, requestID string) ([]baseagent.SkillDefinition, error) {
	path := "/api/v1/agents/" + url.PathEscape(agentID) + "/skills/search"
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		path += "?q=" + url.QueryEscape(trimmed)
	}
	raw, err := c.request(ctx, http.MethodGet, path, nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Skills []baseagent.SkillDefinition `json:"skills"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("skill search response: %w", err)
	}
	if wrapped.Skills == nil {
		return []baseagent.SkillDefinition{}, nil
	}
	return wrapped.Skills, nil
}

func (c *DaemonClient) InstallAgentSkill(ctx context.Context, agentID, skillName, actor, requestID string) (baseagent.SkillDefinition, error) {
	body := map[string]string{"name": strings.TrimSpace(skillName)}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/skills/install", body, actor, requestID)
	if err != nil {
		return baseagent.SkillDefinition{}, err
	}
	var installed baseagent.SkillDefinition
	if err := json.Unmarshal(raw, &installed); err != nil {
		return baseagent.SkillDefinition{}, fmt.Errorf("skill install response: %w", err)
	}
	return installed, nil
}

func (c *DaemonClient) UpdateAgentSkill(ctx context.Context, agentID, skillName, version, actor, requestID string) (baseagent.SkillDefinition, error) {
	body := map[string]string{
		"name":    strings.TrimSpace(skillName),
		"version": strings.TrimSpace(version),
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/skills/update", body, actor, requestID)
	if err != nil {
		return baseagent.SkillDefinition{}, err
	}
	var updated baseagent.SkillDefinition
	if err := json.Unmarshal(raw, &updated); err != nil {
		return baseagent.SkillDefinition{}, fmt.Errorf("skill update response: %w", err)
	}
	return updated, nil
}

func (c *DaemonClient) UninstallAgentSkill(ctx context.Context, agentID, skillName, actor, requestID string) (baseagent.SkillDefinition, error) {
	body := map[string]string{"name": strings.TrimSpace(skillName)}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/skills/uninstall", body, actor, requestID)
	if err != nil {
		return baseagent.SkillDefinition{}, err
	}
	var removed baseagent.SkillDefinition
	if err := json.Unmarshal(raw, &removed); err != nil {
		return baseagent.SkillDefinition{}, fmt.Errorf("skill uninstall response: %w", err)
	}
	return removed, nil
}

func (c *DaemonClient) SetAgentMCPServerEnabled(ctx context.Context, agentID, serverName string, enabled bool, actor, requestID string) (AgentCapabilitySummary, error) {
	body := map[string]bool{"enabled": enabled}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/mcp/"+url.PathEscape(serverName), body, actor, requestID)
	if err != nil {
		return AgentCapabilitySummary{}, err
	}
	var summary AgentCapabilitySummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return AgentCapabilitySummary{}, fmt.Errorf("mcp toggle response: %w", err)
	}
	return summary, nil
}

func (c *DaemonClient) GetAgentMCPServerDetail(ctx context.Context, agentID, serverName, actor, requestID string) (baseagent.MCPServerCapability, error) {
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/agents/"+url.PathEscape(agentID)+"/mcp/"+url.PathEscape(serverName), nil, actor, requestID)
	if err != nil {
		return baseagent.MCPServerCapability{}, err
	}
	var detail baseagent.MCPServerCapability
	if err := json.Unmarshal(raw, &detail); err != nil {
		return baseagent.MCPServerCapability{}, fmt.Errorf("mcp detail response: %w", err)
	}
	return detail, nil
}

func (c *DaemonClient) SetAgentMCPServerAttached(ctx context.Context, agentID, serverName string, attached bool, actor, requestID string) (baseagent.MCPServerCapability, error) {
	action := "detach"
	if attached {
		action = "attach"
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/mcp/"+url.PathEscape(serverName)+"/"+action, map[string]any{}, actor, requestID)
	if err != nil {
		return baseagent.MCPServerCapability{}, err
	}
	var detail baseagent.MCPServerCapability
	if err := json.Unmarshal(raw, &detail); err != nil {
		return baseagent.MCPServerCapability{}, fmt.Errorf("mcp attach response: %w", err)
	}
	return detail, nil
}

func (c *DaemonClient) UpdateAgentMCPServerConfig(ctx context.Context, agentID, serverName, config, actor, requestID string) (baseagent.MCPServerCapability, error) {
	body := map[string]string{"config": strings.TrimSpace(config)}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/mcp/"+url.PathEscape(serverName)+"/config", body, actor, requestID)
	if err != nil {
		return baseagent.MCPServerCapability{}, err
	}
	var detail baseagent.MCPServerCapability
	if err := json.Unmarshal(raw, &detail); err != nil {
		return baseagent.MCPServerCapability{}, fmt.Errorf("mcp config response: %w", err)
	}
	return detail, nil
}

func (c *DaemonClient) GetAgentSessions(ctx context.Context, agentID string, limit int, actor, requestID string) ([]baseagent.SessionStats, error) {
	path := "/api/v1/agents/" + url.PathEscape(agentID) + "/sessions"
	if limit > 0 {
		path += "?limit=" + url.QueryEscape(strconv.Itoa(limit))
	}
	raw, err := c.request(ctx, http.MethodGet, path, nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Sessions []baseagent.SessionStats `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("agent sessions response: %w", err)
	}
	if wrapped.Sessions == nil {
		return []baseagent.SessionStats{}, nil
	}
	return wrapped.Sessions, nil
}

func (c *DaemonClient) GetAgentSubagentJobs(ctx context.Context, agentID string, limit int, actor, requestID string) ([]baseagent.SubagentJob, error) {
	path := "/api/v1/agents/" + url.PathEscape(agentID) + "/subagents"
	if limit > 0 {
		path += "?limit=" + url.QueryEscape(strconv.Itoa(limit))
	}
	raw, err := c.request(ctx, http.MethodGet, path, nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Jobs []baseagent.SubagentJob `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("agent subagent jobs response: %w", err)
	}
	if wrapped.Jobs == nil {
		return []baseagent.SubagentJob{}, nil
	}
	return wrapped.Jobs, nil
}

func (c *DaemonClient) GetAgentSubagentJob(ctx context.Context, agentID, jobID, actor, requestID string) (baseagent.SubagentJob, error) {
	raw, err := c.request(ctx, http.MethodGet, "/api/v1/agents/"+url.PathEscape(agentID)+"/subagents/"+url.PathEscape(jobID), nil, actor, requestID)
	if err != nil {
		return baseagent.SubagentJob{}, err
	}
	var job baseagent.SubagentJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return baseagent.SubagentJob{}, fmt.Errorf("agent subagent job response: %w", err)
	}
	return job, nil
}

// GetLogs returns agent logs.
func (c *DaemonClient) GetLogs(ctx context.Context, agentID string, tail int, actor, requestID string) (*LogsResult, error) {
	safeTail := tail
	if safeTail <= 0 {
		safeTail = 200
	}
	if safeTail > 1000 {
		safeTail = 1000
	}
	path := fmt.Sprintf("/api/v1/agents/%s/logs?tail=%d", url.PathEscape(agentID), safeTail)
	raw, err := c.request(ctx, http.MethodGet, path, nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result LogsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("logs response: %w", err)
	}
	return &result, nil
}

// GetMergedLogs returns merged logs from all agents.
func (c *DaemonClient) GetMergedLogs(ctx context.Context, tail int, actor, requestID string) (*LogsResult, error) {
	safeTail := tail
	if safeTail <= 0 {
		safeTail = 200
	}
	if safeTail > 1000 {
		safeTail = 1000
	}
	path := fmt.Sprintf("/api/v1/logs?tail=%d", safeTail)
	raw, err := c.request(ctx, http.MethodGet, path, nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result LogsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("merged logs response: %w", err)
	}
	return &result, nil
}

// UpgradeAgent upgrades an agent.
func (c *DaemonClient) UpgradeAgent(ctx context.Context, agentID, actor, requestID string) (*UpgradeResult, error) {
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/upgrade", nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result UpgradeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("upgrade response: %w", err)
	}
	return &result, nil
}

// DiagnoseAgent runs diagnostics on an agent and returns the artifact ref.
func (c *DaemonClient) DiagnoseAgent(ctx context.Context, agentID, actor, requestID string) (*DiagnoseResult, error) {
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/diagnose", nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result DiagnoseResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("diagnose response: %w", err)
	}
	return &result, nil
}

// CreateHandoff creates a remote diagnosis handoff.
func (c *DaemonClient) CreateHandoff(ctx context.Context, agentID string, consent bool, actor, requestID string) (*RemoteDiagnosisHandoff, error) {
	body := map[string]interface{}{
		"agentId":   agentID,
		"consent":   consent,
		"actor":     actor,
		"requestId": requestID,
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/diagnosis/handoffs", body, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result RemoteDiagnosisHandoff
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("handoff response: %w", err)
	}
	return &result, nil
}

// VerifyPairCode verifies and consumes a pairing code.
func (c *DaemonClient) VerifyPairCode(ctx context.Context, code, actor, requestID string) error {
	body := map[string]string{"code": code}
	_, err := c.request(ctx, http.MethodPost, "/api/v1/pairing/verify-consume", body, actor, requestID)
	return err
}

func (c *DaemonClient) ChatBaseAgent(
	ctx context.Context,
	provider string,
	chatID string,
	requestID string,
	message string,
	attachments []baseagent.AttachmentRef,
	actor string,
) (*BaseAgentChatResult, error) {
	body := baseagent.ChatRequest{
		Provider:    provider,
		ChatID:      chatID,
		RequestID:   requestID,
		Message:     message,
		Attachments: append([]baseagent.AttachmentRef(nil), attachments...),
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/base-agent/chat", body, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result baseagent.ChatResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("base-agent chat response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) DecomposeBaseAgent(
	ctx context.Context,
	goal string,
	actor string,
	requestID string,
) ([]baseagent.DecomposeTask, error) {
	return c.DecomposeBaseAgentWithProvider(ctx, goal, "", actor, requestID)
}

func (c *DaemonClient) DecomposeBaseAgentWithProvider(
	ctx context.Context,
	goal string,
	provider string,
	actor string,
	requestID string,
) ([]baseagent.DecomposeTask, error) {
	body := map[string]interface{}{
		"goal": strings.TrimSpace(goal),
	}
	if trimmedProvider := strings.TrimSpace(provider); trimmedProvider != "" {
		body["provider"] = trimmedProvider
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/base-agent/decompose", body, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tasks []baseagent.DecomposeTask `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("base-agent decompose response: %w", err)
	}
	if result.Tasks == nil {
		result.Tasks = []baseagent.DecomposeTask{}
	}
	return result.Tasks, nil
}

func (c *DaemonClient) ChatAgent(
	ctx context.Context,
	agentID string,
	provider string,
	message string,
	sessionID string,
	modelAlias string,
	model string,
	actor string,
	requestID string,
) (*AgentChatResult, error) {
	payload := map[string]interface{}{
		"message": message,
	}
	if strings.TrimSpace(provider) != "" {
		payload["provider"] = strings.TrimSpace(provider)
	}
	if strings.TrimSpace(sessionID) != "" {
		payload["sessionId"] = strings.TrimSpace(sessionID)
	}
	if strings.TrimSpace(modelAlias) != "" {
		payload["modelAlias"] = strings.TrimSpace(modelAlias)
	}
	if strings.TrimSpace(model) != "" {
		payload["model"] = strings.TrimSpace(model)
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/chat", payload, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result AgentChatResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("agent chat response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) SpeakAgentMedia(
	ctx context.Context,
	agentID string,
	text string,
	voice string,
	format string,
	actor string,
	requestID string,
) (*AgentChatResult, error) {
	payload := map[string]any{
		"text": strings.TrimSpace(text),
	}
	if trimmed := strings.TrimSpace(voice); trimmed != "" {
		payload["voice"] = trimmed
	}
	if trimmed := strings.TrimSpace(format); trimmed != "" {
		payload["format"] = trimmed
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/media/speak", payload, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result AgentChatResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("agent media response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) ScheduleCronJob(
	ctx context.Context,
	job baseagent.CronJob,
	actor string,
	requestID string,
) (*CronJob, error) {
	payload := struct {
		SessionKey string    `json:"sessionKey"`
		AgentID    string    `json:"agentId,omitempty"`
		Prompt     string    `json:"prompt"`
		NextRunAt  time.Time `json:"nextRunAt,omitempty"`
	}{
		SessionKey: strings.TrimSpace(job.SessionKey),
		AgentID:    strings.TrimSpace(job.AgentID),
		Prompt:     strings.TrimSpace(job.Prompt),
		NextRunAt:  job.NextRunAt,
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/base-agent/cron/schedule", payload, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result CronJob
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cron schedule response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) ListCronJobs(
	ctx context.Context,
	agentID string,
	sessionKey string,
	actor string,
	requestID string,
) ([]CronJob, error) {
	values := url.Values{}
	if trimmed := strings.TrimSpace(agentID); trimmed != "" {
		values.Set("agentId", trimmed)
	}
	if trimmed := strings.TrimSpace(sessionKey); trimmed != "" {
		values.Set("sessionKey", trimmed)
	}
	path := "/api/base-agent/cron/jobs"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	raw, err := c.request(ctx, http.MethodGet, path, nil, actor, requestID)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Jobs []CronJob `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("cron jobs response: %w", err)
	}
	if wrapped.Jobs == nil {
		wrapped.Jobs = []CronJob{}
	}
	return wrapped.Jobs, nil
}

func (c *DaemonClient) CancelCronJob(
	ctx context.Context,
	jobID string,
	actor string,
	requestID string,
) (*CronJob, error) {
	path := "/api/base-agent/cron/" + url.PathEscape(strings.TrimSpace(jobID)) + "/cancel"
	raw, err := c.request(ctx, http.MethodPost, path, map[string]any{}, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result CronJob
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cron cancel response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) PauseCronJob(
	ctx context.Context,
	jobID string,
	actor string,
	requestID string,
) (*CronJob, error) {
	path := "/api/base-agent/cron/" + url.PathEscape(strings.TrimSpace(jobID)) + "/pause"
	raw, err := c.request(ctx, http.MethodPost, path, map[string]any{}, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result CronJob
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cron pause response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) ResumeCronJob(
	ctx context.Context,
	jobID string,
	actor string,
	requestID string,
) (*CronJob, error) {
	path := "/api/base-agent/cron/" + url.PathEscape(strings.TrimSpace(jobID)) + "/resume"
	raw, err := c.request(ctx, http.MethodPost, path, map[string]any{}, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result CronJob
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cron resume response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) RunCronJob(
	ctx context.Context,
	jobID string,
	actor string,
	requestID string,
) (*CronJob, error) {
	path := "/api/base-agent/cron/" + url.PathEscape(strings.TrimSpace(jobID)) + "/run"
	raw, err := c.request(ctx, http.MethodPost, path, map[string]any{}, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result CronJob
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cron run response: %w", err)
	}
	return &result, nil
}

func (c *DaemonClient) request(ctx context.Context, method, path string, body interface{}, actor, requestID string) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Carrier-Actor", actor)
	req.Header.Set("X-Carrier-Request-Id", requestID)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &DaemonClientError{Code: "E_COMMAND_FAILED", Message: err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxDaemonResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.parseErrorResponse(resp.StatusCode, raw)
	}

	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}"), nil
	}
	return raw, nil
}

func (c *DaemonClient) parseErrorResponse(status int, body []byte) *DaemonClientError {
	fallbackCode := c.statusToCode(status)
	msg := fmt.Sprintf("daemon request failed with status %d", status)
	code := fallbackCode

	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(bytes.TrimSpace(envelope.Error)) > 0 {
		var detailed struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(envelope.Error, &detailed); err == nil {
			if strings.TrimSpace(detailed.Code) != "" {
				code = strings.TrimSpace(detailed.Code)
			}
			if strings.TrimSpace(detailed.Message) != "" {
				msg = strings.TrimSpace(detailed.Message)
			}
		} else {
			// Legacy daemon responses can return {"error":"..."}.
			var legacy string
			if err := json.Unmarshal(envelope.Error, &legacy); err == nil && strings.TrimSpace(legacy) != "" {
				msg = strings.TrimSpace(legacy)
			}
		}
	}

	code = normalizeDaemonErrorCode(code, msg)
	return &DaemonClientError{Code: code, Message: msg}
}

func (c *DaemonClient) statusToCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "E_USAGE"
	case http.StatusUnauthorized, http.StatusForbidden:
		return "E_SESSION_REQUIRED"
	case http.StatusNotFound:
		return "E_AGENT_NOT_FOUND"
	default:
		return "E_COMMAND_FAILED"
	}
}

func normalizeDaemonErrorCode(code, message string) string {
	normalized := strings.TrimSpace(code)
	if normalized == "" {
		normalized = "E_COMMAND_FAILED"
	}

	lowerMsg := strings.ToLower(strings.TrimSpace(message))
	if (normalized == "E_USAGE" || normalized == "E_COMMAND_FAILED") &&
		strings.Contains(lowerMsg, "pairing code is invalid or expired") {
		return "E_PAIR_CODE_INVALID"
	}

	return normalized
}
