package gateway

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type RemoteAuthMode string

const (
	RemoteAuthModePrivateKey RemoteAuthMode = "private_key"
	RemoteAuthModeSSHConfig  RemoteAuthMode = "ssh_config"
)

type RemoteRuntimeMode string

const (
	RemoteRuntimeModeOnDemand       RemoteRuntimeMode = "on_demand"
	RemoteRuntimeModeManagedGateway RemoteRuntimeMode = "managed_gateway"
)

type RemoteHealth string

const (
	RemoteHealthUnknown   RemoteHealth = "unknown"
	RemoteHealthHealthy   RemoteHealth = "healthy"
	RemoteHealthUnhealthy RemoteHealth = "unhealthy"
)

type RemoteHost struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Host          string            `json:"host,omitempty"`
	Port          int               `json:"port,omitempty"`
	User          string            `json:"user,omitempty"`
	AuthMode      RemoteAuthMode    `json:"authMode"`
	KeyPath       string            `json:"keyPath,omitempty"`
	KeyRef        string            `json:"keyRef,omitempty"`
	SSHConfigHost string            `json:"sshConfigHost,omitempty"`
	RuntimeMode   RemoteRuntimeMode `json:"runtimeMode"`
	Labels        []string          `json:"labels,omitempty"`
	LastHealth    RemoteHealth      `json:"lastHealth,omitempty"`
	LastCheckAt   string            `json:"lastCheckAt,omitempty"`
	LastError     string            `json:"lastError,omitempty"`
	CreatedAt     string            `json:"createdAt"`
	UpdatedAt     string            `json:"updatedAt"`
}

type RemoteInstance struct {
	ID           string `json:"id"`
	HostID       string `json:"hostId"`
	AgentID      string `json:"agentId"`
	RuntimeState string `json:"runtimeState"`
	Health       string `json:"health"`
	LastError    string `json:"lastError,omitempty"`
	ConfigPath   string `json:"configPath,omitempty"`
	MemoryPath   string `json:"memoryPath,omitempty"`
	SessionPath  string `json:"sessionPath,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

type ProviderProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	BaseURL   string `json:"baseUrl,omitempty"`
	AuthRef   string `json:"authRef,omitempty"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type ProviderBinding struct {
	ID         string `json:"id"`
	ProfileID  string `json:"profileId"`
	TargetType string `json:"targetType"` // host | instance
	TargetID   string `json:"targetId"`
	SyncMode   string `json:"syncMode"` // always_push | pull_validate_push | manual
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type ProviderGovernanceResolution struct {
	Source                string  `json:"source"`
	Status                string  `json:"status"`
	HostID                string  `json:"hostId"`
	AgentID               string  `json:"agentId,omitempty"`
	BindingID             string  `json:"bindingId,omitempty"`
	BindingTargetType     string  `json:"bindingTargetType,omitempty"`
	BindingTargetID       string  `json:"bindingTargetId,omitempty"`
	ProfileID             string  `json:"profileId,omitempty"`
	ProfileName           string  `json:"profileName,omitempty"`
	Provider              string  `json:"provider,omitempty"`
	Model                 string  `json:"model,omitempty"`
	BaseURL               string  `json:"baseUrl,omitempty"`
	AuthRef               string  `json:"authRef,omitempty"`
	SyncMode              string  `json:"syncMode,omitempty"`
	Enabled               bool    `json:"enabled"`
	EstimatedInputTokens  int     `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int     `json:"estimatedOutputTokens,omitempty"`
	EstimatedTotalTokens  int     `json:"estimatedTotalTokens,omitempty"`
	EstimatedCostUSD      float64 `json:"estimatedCostUsd,omitempty"`
	SuccessfulTasks       int     `json:"successfulTasks,omitempty"`
	FailedTasks           int     `json:"failedTasks,omitempty"`
	AvgLatencyMs          int64   `json:"avgLatencyMs,omitempty"`
	Message               string  `json:"message,omitempty"`
}

type RemoteSessionEntry struct {
	SessionID  string `json:"sessionId"`
	Kind       string `json:"kind"`
	SizeBytes  int64  `json:"sizeBytes"`
	ModifiedAt int64  `json:"modifiedAt"`
	Path       string `json:"path"`
}

type RemoteMemoryEntry struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"sizeBytes"`
	ModifiedAt int64  `json:"modifiedAt"`
}

type RemoteMemoryContractStatus struct {
	ContractID     string `json:"contractId,omitempty"`
	ContractDigest string `json:"contractDigest,omitempty"`
	SyncState      string `json:"syncState,omitempty"`
	SyncError      string `json:"syncError,omitempty"`
	SyncedAt       string `json:"syncedAt,omitempty"`
}

type RemoteMemoryGitConfig struct {
	Enabled  bool   `json:"enabled"`
	RepoURL  string `json:"repoUrl,omitempty"`
	Branch   string `json:"branch,omitempty"`
	BaseDir  string `json:"baseDir,omitempty"`
	AuthMode string `json:"authMode,omitempty"`
}

type RemoteInstanceSyncStatus struct {
	HostID               string                 `json:"hostId"`
	AgentID              string                 `json:"agentId"`
	SyncMode             string                 `json:"syncMode"`
	DriftState           string                 `json:"driftState"`
	LastSyncStatus       string                 `json:"lastSyncStatus"`
	LastSyncAt           string                 `json:"lastSyncAt,omitempty"`
	LastDiagnoseAt       string                 `json:"lastDiagnoseAt,omitempty"`
	LastDiagnoseResult   string                 `json:"lastDiagnoseResult,omitempty"`
	LastReconcileAt      string                 `json:"lastReconcileAt,omitempty"`
	LastRollbackAt       string                 `json:"lastRollbackAt,omitempty"`
	LastRemoteHash       string                 `json:"lastRemoteHash,omitempty"`
	LastCommonCommit     string                 `json:"lastCommonCommit,omitempty"`
	LastLocalCommit      string                 `json:"lastLocalCommit,omitempty"`
	LastRemoteCommit     string                 `json:"lastRemoteCommit,omitempty"`
	LastCanonicalConfig  map[string]interface{} `json:"lastCanonicalConfig,omitempty"`
	MemoryLastSyncStatus string                 `json:"memoryLastSyncStatus,omitempty"`
	MemoryLastSyncAt     string                 `json:"memoryLastSyncAt,omitempty"`
	MemoryLastSyncError  string                 `json:"memoryLastSyncError,omitempty"`
	MemoryContractDigest string                 `json:"memoryContractDigest,omitempty"`
	MemoryGit            RemoteMemoryGitConfig  `json:"memoryGit,omitempty"`
	UpdatedAt            string                 `json:"updatedAt"`
}

const (
	providerBindingSyncModeAlwaysPush       = "always_push"
	providerBindingSyncModePullValidatePush = "pull_validate_push"
	providerBindingSyncModeManual           = "manual"
)

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func normalizeRemoteHost(input RemoteHost) RemoteHost {
	h := input
	h.ID = strings.TrimSpace(h.ID)
	h.Name = strings.TrimSpace(h.Name)
	h.Host = strings.TrimSpace(h.Host)
	h.User = strings.TrimSpace(h.User)
	h.KeyPath = strings.TrimSpace(h.KeyPath)
	h.KeyRef = strings.TrimSpace(h.KeyRef)
	h.SSHConfigHost = strings.TrimSpace(h.SSHConfigHost)
	h.Labels = normalizeStringSelectorList(h.Labels, true)
	h.LastError = strings.TrimSpace(h.LastError)
	if h.AuthMode == "" {
		h.AuthMode = RemoteAuthModePrivateKey
	}
	if h.RuntimeMode == "" {
		h.RuntimeMode = RemoteRuntimeModeOnDemand
	}
	if h.Port == 0 {
		h.Port = 22
	}
	if h.Name == "" {
		if h.Host != "" {
			h.Name = h.Host
		} else {
			h.Name = h.SSHConfigHost
		}
	}
	if h.LastHealth == "" {
		h.LastHealth = RemoteHealthUnknown
	}
	if h.AuthMode == RemoteAuthModePrivateKey && h.KeyPath != "" {
		h.KeyPath = filepath.Clean(h.KeyPath)
	}
	return h
}

func validateRemoteHost(h RemoteHost) error {
	switch h.AuthMode {
	case RemoteAuthModePrivateKey:
		if h.Host == "" {
			return fmt.Errorf("host is required for private_key auth mode")
		}
		if h.KeyPath == "" && h.KeyRef == "" {
			return fmt.Errorf("keyPath or keyRef is required for private_key auth mode")
		}
	case RemoteAuthModeSSHConfig:
		if h.SSHConfigHost == "" && h.Host == "" {
			return fmt.Errorf("sshConfigHost or host is required for ssh_config auth mode")
		}
	default:
		return fmt.Errorf("unsupported authMode %q", h.AuthMode)
	}
	switch h.RuntimeMode {
	case RemoteRuntimeModeOnDemand, RemoteRuntimeModeManagedGateway:
		// valid
	default:
		return fmt.Errorf("unsupported runtimeMode %q", h.RuntimeMode)
	}
	if h.Port < 1 || h.Port > 65535 {
		return fmt.Errorf("port must be in range 1-65535")
	}
	return nil
}

func normalizeProviderProfile(in ProviderProfile) ProviderProfile {
	p := in
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Provider = strings.ToLower(strings.TrimSpace(p.Provider))
	p.Model = strings.TrimSpace(p.Model)
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	p.AuthRef = strings.TrimSpace(p.AuthRef)
	if p.Name == "" && p.Provider != "" && p.Model != "" {
		p.Name = p.Provider + "/" + p.Model
	}
	return p
}

func validateProviderProfile(p ProviderProfile) error {
	if strings.TrimSpace(p.Provider) == "" {
		return fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

func normalizeProviderBinding(in ProviderBinding) ProviderBinding {
	b := in
	b.ID = strings.TrimSpace(b.ID)
	b.ProfileID = strings.TrimSpace(b.ProfileID)
	b.TargetType = strings.ToLower(strings.TrimSpace(b.TargetType))
	b.TargetID = strings.TrimSpace(b.TargetID)
	b.SyncMode = normalizeProviderBindingSyncMode(b.SyncMode)
	if b.SyncMode == "" {
		b.SyncMode = providerBindingSyncModeAlwaysPush
	}
	return b
}

func validateProviderBinding(b ProviderBinding) error {
	if b.ProfileID == "" {
		return fmt.Errorf("profileId is required")
	}
	switch b.TargetType {
	case "host", "instance":
		// valid
	default:
		return fmt.Errorf("targetType must be one of host or instance")
	}
	if b.TargetID == "" {
		return fmt.Errorf("targetId is required")
	}
	if err := validateProviderBindingSyncMode(b.SyncMode); err != nil {
		return err
	}
	return nil
}

func normalizeProviderBindingSyncMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return providerBindingSyncModeAlwaysPush
	}
	return mode
}

func validateProviderBindingSyncMode(mode string) error {
	switch normalizeProviderBindingSyncMode(mode) {
	case providerBindingSyncModeAlwaysPush, providerBindingSyncModePullValidatePush, providerBindingSyncModeManual:
		return nil
	default:
		return fmt.Errorf("syncMode must be one of always_push, pull_validate_push, manual")
	}
}

func normalizeRemoteMemoryGitConfig(in RemoteMemoryGitConfig) RemoteMemoryGitConfig {
	out := in
	out.RepoURL = strings.TrimSpace(out.RepoURL)
	out.Branch = strings.TrimSpace(out.Branch)
	out.BaseDir = strings.TrimSpace(out.BaseDir)
	out.AuthMode = strings.ToLower(strings.TrimSpace(out.AuthMode))
	if out.AuthMode == "" {
		out.AuthMode = "system"
	}
	if out.Branch == "" {
		out.Branch = "main"
	}
	return out
}

func validateAgentIdentifier(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return fmt.Errorf("agent id is required")
	}
	if len(trimmed) > 128 {
		return fmt.Errorf("agent id exceeds maximum length of 128 characters")
	}
	for _, ch := range trimmed {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return fmt.Errorf("agent id contains unsupported character %q", ch)
	}
	return nil
}

func validateRemoteSessionIdentifier(id string) (string, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "", fmt.Errorf("sessionId is required")
	}
	if len(trimmed) > 128 {
		return "", fmt.Errorf("sessionId exceeds maximum length of 128 characters")
	}
	for _, ch := range trimmed {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return "", fmt.Errorf("sessionId contains unsupported character %q", ch)
	}
	return trimmed, nil
}
