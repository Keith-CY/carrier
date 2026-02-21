package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultDaemonBaseURL  = "http://127.0.0.1:9090"
	defaultDaemonTimeout  = 30 * time.Second
	maxDaemonResponseSize = 32 * 1024 * 1024 // 32 MB
)

// AgentState mirrors the daemon's agent status.
type AgentState struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Version              string  `json:"version"`
	InstallState         string  `json:"installState"`
	Runtime              string  `json:"runtimeState"`
	Health               string  `json:"health"`
	Ports                []int   `json:"ports"`
	StartedAt            *string `json:"startedAt,omitempty"`
	RestartCount         int     `json:"restartCount"`
	NeedsRemoteDiagnosis bool    `json:"needsRemoteDiagnosis"`
	LastError            *string `json:"lastError,omitempty"`
	UpdatedAt            string  `json:"updatedAt"`
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

type BaseAgentChatResult struct {
	Message    string `json:"message"`
	Action     string `json:"action,omitempty"`
	SelfHealed bool   `json:"selfHealed,omitempty"`
	BackupRef  string `json:"backupRef,omitempty"`
}

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
	_, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/install", nil, actor, requestID)
	return err
}

// StartAgent starts an agent.
func (c *DaemonClient) StartAgent(ctx context.Context, agentID, actor, requestID string) error {
	_, err := c.request(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/start", nil, actor, requestID)
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
	var arr []AgentState
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	return []AgentState{}, nil
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
	actor string,
) (*BaseAgentChatResult, error) {
	body := map[string]string{
		"provider":  provider,
		"chatId":    chatID,
		"requestId": requestID,
		"message":   message,
	}
	raw, err := c.request(ctx, http.MethodPost, "/api/v1/base-agent/chat", body, actor, requestID)
	if err != nil {
		return nil, err
	}
	var result BaseAgentChatResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("base-agent chat response: %w", err)
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
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error != nil {
		if envelope.Error.Code != "" {
			code = envelope.Error.Code
		}
		if envelope.Error.Message != "" {
			msg = envelope.Error.Message
		}
	}
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
