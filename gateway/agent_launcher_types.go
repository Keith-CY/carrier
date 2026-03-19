package gateway

import "carrier/baseagent"

type agentLauncherSummary struct {
	AgentID           string                          `json:"agentId"`
	Status            AgentState                      `json:"status"`
	Heartbeat         *AgentHeartbeat                 `json:"heartbeat,omitempty"`
	Memory            *AgentMemoryState               `json:"memory,omitempty"`
	Capabilities      AgentCapabilitySummary          `json:"capabilities"`
	ProviderReadiness agentProviderReadiness          `json:"providerReadiness"`
	MediaRuntime      *agentLauncherMediaRuntime      `json:"mediaRuntime,omitempty"`
	Remediations      []agentLauncherRemediation      `json:"remediations,omitempty"`
	ModelSurface      *agentLauncherModelSurface      `json:"modelSurface,omitempty"`
	LastModelRun      *agentLauncherModelRuntime      `json:"lastModelRun,omitempty"`
	Cron              *agentLauncherCronSummary       `json:"cron,omitempty"`
	Delegation        *agentLauncherDelegationSummary `json:"delegation,omitempty"`
	Sessions          *agentLauncherSessionsSummary   `json:"sessions,omitempty"`
	Session           *agentLauncherSession           `json:"session,omitempty"`
}

type agentLauncherMediaRuntime struct {
	Provider        string `json:"provider,omitempty"`
	Status          string `json:"status,omitempty"`
	Detail          string `json:"detail,omitempty"`
	RemediationHint string `json:"remediationHint,omitempty"`
}

type agentLauncherRemediation struct {
	Category        string                          `json:"category,omitempty"`
	Summary         string                          `json:"summary,omitempty"`
	Detail          string                          `json:"detail,omitempty"`
	RemediationHint string                          `json:"remediationHint,omitempty"`
	Action          *agentLauncherRemediationAction `json:"action,omitempty"`
}

type agentLauncherRemediationAction struct {
	Kind   string `json:"kind,omitempty"`
	Label  string `json:"label,omitempty"`
	Target string `json:"target,omitempty"`
}

type agentLauncherModelSurface struct {
	DefaultProfile string                             `json:"defaultProfile,omitempty"`
	Profiles       []agentLauncherModelSurfaceProfile `json:"profiles,omitempty"`
}

type agentLauncherModelSurfaceProfile struct {
	ProfileName      string `json:"profileName,omitempty"`
	ModelAlias       string `json:"modelAlias,omitempty"`
	ModelID          string `json:"modelId,omitempty"`
	ProviderID       string `json:"providerId,omitempty"`
	ProviderKey      string `json:"providerKey,omitempty"`
	ProtocolFamily   string `json:"protocolFamily,omitempty"`
	BaseURL          string `json:"baseUrl,omitempty"`
	AuthMethod       string `json:"authMethod,omitempty"`
	TimeoutMs        int    `json:"timeoutMs,omitempty"`
	RetryBudget      int    `json:"retryBudget,omitempty"`
	FallbackStrategy string `json:"fallbackStrategy,omitempty"`
	FallbackGroup    string `json:"fallbackGroup,omitempty"`
	AliasGroupSize   int    `json:"aliasGroupSize,omitempty"`
	Primary          bool   `json:"primary,omitempty"`
}

type agentLauncherModelRuntime struct {
	RequestedAlias    string `json:"requestedAlias,omitempty"`
	RequestedModel    string `json:"requestedModel,omitempty"`
	ResolvedModel     string `json:"resolvedModel,omitempty"`
	ResolvedProfile   string `json:"resolvedProfile,omitempty"`
	FallbackGroup     string `json:"fallbackGroup,omitempty"`
	SelectionStrategy string `json:"selectionStrategy,omitempty"`
	SelectionOrdinal  int    `json:"selectionOrdinal,omitempty"`
	OverrideHit       bool   `json:"overrideHit,omitempty"`
	FallbackHit       bool   `json:"fallbackHit,omitempty"`
	LastRunAt         string `json:"lastRunAt,omitempty"`
}

type agentLauncherCronSummary struct {
	Count      int       `json:"count"`
	NextRunAt  string    `json:"nextRunAt,omitempty"`
	LastRunAt  string    `json:"lastRunAt,omitempty"`
	LastResult string    `json:"lastResult,omitempty"`
	Jobs       []CronJob `json:"jobs,omitempty"`
}

type agentLauncherDelegationSummary struct {
	Count int                     `json:"count"`
	Jobs  []baseagent.SubagentJob `json:"jobs,omitempty"`
}

type agentLauncherSessionsSummary struct {
	Count    int                      `json:"count"`
	Sessions []baseagent.SessionStats `json:"sessions,omitempty"`
}

type agentProviderReadiness struct {
	Provider             string `json:"provider,omitempty"`
	AuthMode             string `json:"authMode,omitempty"`
	CredentialBackend    string `json:"credentialBackend,omitempty"`
	CredentialConfigured bool   `json:"credentialConfigured"`
	Ready                bool   `json:"ready"`
}

type agentLauncherSession struct {
	InstanceID          string   `json:"instanceId"`
	Type                string   `json:"type,omitempty"`
	Channel             string   `json:"channel,omitempty"`
	Provider            string   `json:"provider,omitempty"`
	Isolation           bool     `json:"isolation,omitempty"`
	RuntimeState        string   `json:"runtimeState,omitempty"`
	GatewayURL          string   `json:"gatewayUrl,omitempty"`
	PairRequired        bool     `json:"pairRequired,omitempty"`
	PairedChatID        string   `json:"pairedChatId,omitempty"`
	Workspace           string   `json:"workspace,omitempty"`
	ConfigPath          string   `json:"configPath,omitempty"`
	RecordPath          string   `json:"recordPath,omitempty"`
	AgentLifecycleMode  string   `json:"agentLifecycleMode,omitempty"`
	MemoryBindingMode   string   `json:"memoryBindingMode,omitempty"`
	PublicScopes        []string `json:"publicScopes,omitempty"`
	SharedScopes        []string `json:"sharedScopes,omitempty"`
	PerAgentMemoryID    string   `json:"perAgentMemoryId,omitempty"`
	MemoryRefreshPolicy string   `json:"memoryRefreshPolicy,omitempty"`
	ParentAgentID       string   `json:"parentAgentId,omitempty"`
	ParentExecutionID   string   `json:"parentExecutionId,omitempty"`
	TaskID              string   `json:"taskId,omitempty"`
	SnapshotID          string   `json:"snapshotId,omitempty"`
	SnapshotDigest      string   `json:"snapshotDigest,omitempty"`
	DistillTarget       string   `json:"distillTarget,omitempty"`
	CleanupPolicy       string   `json:"cleanupPolicy,omitempty"`
	CreatedAt           string   `json:"createdAt,omitempty"`
	UpdatedAt           string   `json:"updatedAt,omitempty"`
}
