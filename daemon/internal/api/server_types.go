package api

import (
	"net/http"
	"regexp"
	"time"

	"carrier/daemon/internal/lifecycle"
	"carrier/shared/redact"
)

var validAgentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

type Server struct {
	lifecycle *lifecycle.Service
	pairing   *PairingCodeStore
}

type ServerOption func(*Server)

func WithPairingCodeStore(store *PairingCodeStore) ServerOption {
	return func(s *Server) {
		if store != nil {
			s.pairing = store
		}
	}
}

func NewServer(svc *lifecycle.Service, opts ...ServerOption) *Server {
	server := &Server{
		lifecycle: svc,
		pairing:   NewPairingCodeStore(nil),
	}
	for _, opt := range opts {
		opt(server)
	}
	return server
}

func (s *Server) Handler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents", s.handleListAgents)
	mux.HandleFunc("/api/v1/agents/", s.handleAgentAction)
	mux.HandleFunc("/api/v1/logs", s.handleMergedLogs)
	mux.HandleFunc("/api/v1/audit/logs", s.handleAuditLogs)
	mux.HandleFunc("/api/v1/pairing/codes", s.handleIssuePairCode)
	mux.HandleFunc("/api/v1/pairing/verify-consume", s.handleVerifyConsumePairCode)
	mux.HandleFunc("/api/v1/diagnosis/handoffs", s.handleCreateDiagnosisHandoff)
	return mux
}

type daemonAgent struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	Version              string             `json:"version"`
	InstallState         string             `json:"installState"`
	Installed            bool               `json:"installed"`
	RuntimeState         string             `json:"runtimeState"`
	Health               string             `json:"health"`
	Ports                []int              `json:"ports,omitempty"`
	StartedAt            string             `json:"startedAt,omitempty"`
	RestartCount         int                `json:"restartCount"`
	NeedsRemoteDiagnosis bool               `json:"needsRemoteDiagnosis"`
	Isolated             bool               `json:"isolated"`
	LimaInstanceName     string             `json:"limaInstanceName,omitempty"`
	LastError            string             `json:"lastError,omitempty"`
	LastTriageSummary    string             `json:"lastTriageSummary,omitempty"`
	LastDiagnoseFile     string             `json:"lastDiagnoseFile,omitempty"`
	Memory               *daemonAgentMemory `json:"memory,omitempty"`
	UpdatedAt            string             `json:"updatedAt"`
}

type daemonAgentMemory struct {
	ContractID     string `json:"contractId,omitempty"`
	ContractDigest string `json:"contractDigest,omitempty"`
	SyncState      string `json:"syncState,omitempty"`
	SyncError      string `json:"syncError,omitempty"`
	SyncedAt       string `json:"syncedAt,omitempty"`
}

type listAgentsResponse struct {
	Agents []daemonAgent `json:"agents"`
}

func (s *Server) agentFromState(state lifecycle.AgentState) daemonAgent {
	startedAt := ""
	if state.StartedAt != nil {
		startedAt = state.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	var memoryState *daemonAgentMemory
	if state.Memory != nil {
		syncedAt := ""
		if state.Memory.SyncedAt != nil {
			syncedAt = state.Memory.SyncedAt.UTC().Format(time.RFC3339Nano)
		}
		memoryState = &daemonAgentMemory{
			ContractID:     state.Memory.ContractID,
			ContractDigest: state.Memory.ContractDigest,
			SyncState:      state.Memory.SyncState,
			SyncError:      redact.RedactText(state.Memory.SyncError),
			SyncedAt:       syncedAt,
		}
	}
	return daemonAgent{
		ID:                   state.ID,
		Name:                 s.lifecycle.AgentName(state.ID),
		Version:              state.Version,
		InstallState:         string(state.Install),
		Installed:            state.Install == lifecycle.InstallStateInstalled,
		RuntimeState:         string(state.Runtime),
		Health:               string(state.Health),
		Ports:                append([]int(nil), state.Ports...),
		StartedAt:            startedAt,
		RestartCount:         state.RestartCount,
		NeedsRemoteDiagnosis: state.NeedsRemoteDiagnosis,
		Isolated:             state.Isolated,
		LimaInstanceName:     state.LimaInstanceName,
		LastError:            redact.RedactText(state.LastError),
		LastTriageSummary:    redact.RedactText(state.LastTriageSummary),
		LastDiagnoseFile:     state.LastDiagnoseFile,
		Memory:               memoryState,
		UpdatedAt:            state.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}
