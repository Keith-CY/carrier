package lifecycle

import "time"

type InstallState string

type RuntimeState string

type HealthState string

const (
	InstallStateNotInstalled InstallState = "not_installed"
	InstallStateInstalled    InstallState = "installed"
	InstallStateBroken       InstallState = "broken"
)

const (
	RuntimeStateStopped   RuntimeState = "stopped"
	RuntimeStateStarting  RuntimeState = "starting"
	RuntimeStateRunning   RuntimeState = "running"
	RuntimeStateStopping  RuntimeState = "stopping"
	RuntimeStateCrashing  RuntimeState = "crashing"
	RuntimeStateCrashLoop RuntimeState = "crash_loop"
)

const (
	HealthStateUnknown   HealthState = "unknown"
	HealthStateHealthy   HealthState = "healthy"
	HealthStateDegraded  HealthState = "degraded"
	HealthStateUnhealthy HealthState = "unhealthy"
)

type AgentState struct {
	ID                   string       `json:"id"`
	Name                 string       `json:"name"`
	Version              string       `json:"version"`
	Install              InstallState `json:"installState"`
	Runtime              RuntimeState `json:"runtimeState"`
	Health               HealthState  `json:"health"`
	Memory               *MemoryState `json:"memory,omitempty"`
	Ports                []int        `json:"ports"`
	StartedAt            *time.Time   `json:"startedAt,omitempty"`
	RestartCount         int          `json:"restartCount"`
	LastError            string       `json:"lastError,omitempty"`
	LastTriageSummary    string       `json:"lastTriageSummary,omitempty"`
	NeedsRemoteDiagnosis bool         `json:"needsRemoteDiagnosis"`
	LastDiagnoseFile     string       `json:"lastDiagnoseFile,omitempty"`
	Isolated             bool         `json:"isolated"`
	LimaInstanceName     string       `json:"limaInstanceName,omitempty"`
	UpdatedAt            time.Time    `json:"updatedAt"`
}

type MemoryState struct {
	ContractID     string     `json:"contractId,omitempty"`
	ContractDigest string     `json:"contractDigest,omitempty"`
	SyncState      string     `json:"syncState,omitempty"`
	SyncError      string     `json:"syncError,omitempty"`
	SyncedAt       *time.Time `json:"syncedAt,omitempty"`
}

type UpgradeResult struct {
	AgentID     string
	FromVersion string
	ToVersion   string
	BackupPath  string
}

type HandoffStatus string

const (
	HandoffStatusPending  HandoffStatus = "pending"
	HandoffStatusDeclined HandoffStatus = "declined"
)

type DiagnosisHandoff struct {
	ID          string
	AgentID     string
	Consent     bool
	ArtifactRef string
	Status      HandoffStatus
	CreatedAt   time.Time
}

type AuditResult string

const (
	AuditResultSuccess AuditResult = "success"
	AuditResultFailure AuditResult = "failure"
	AuditResultNeutral AuditResult = "neutral"
)

type AuditLog struct {
	RequestID string
	Actor     string
	Action    string
	Target    string
	Result    AuditResult
	ErrorCode string
	Message   string
	Timestamp time.Time
}
