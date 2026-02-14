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
	ID        string
	Version   string
	Install   InstallState
	Runtime   RuntimeState
	Health    HealthState
	LastError string
	UpdatedAt time.Time
}
