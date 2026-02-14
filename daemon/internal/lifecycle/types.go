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
	RuntimeStateStopped  RuntimeState = "stopped"
	RuntimeStateStarting RuntimeState = "starting"
	RuntimeStateRunning  RuntimeState = "running"
	RuntimeStateCrashing RuntimeState = "crashing"
)

const (
	HealthStateUnknown   HealthState = "unknown"
	HealthStateHealthy   HealthState = "healthy"
	HealthStateDegraded  HealthState = "degraded"
	HealthStateUnhealthy HealthState = "unhealthy"
)

type AgentState struct {
	ID                   string
	Version              string
	Install              InstallState
	Runtime              RuntimeState
	Health               HealthState
	LastError            string
	LastTriageSummary    string
	NeedsRemoteDiagnosis bool
	LastDiagnoseFile     string
	UpdatedAt            time.Time
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
