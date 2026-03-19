package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type remoteControlState struct {
	Hosts         []RemoteHost                        `json:"hosts"`
	Profiles      []ProviderProfile                   `json:"providerProfiles"`
	Bindings      []ProviderBinding                   `json:"providerBindings"`
	Policies      []OrchestratorPolicyRule            `json:"orchestratorPolicies,omitempty"`
	Triggers      []ExecutionTrigger                  `json:"executionTriggers,omitempty"`
	InstanceSyncs map[string]RemoteInstanceSyncStatus `json:"instanceSyncs,omitempty"`
	Executions    []OrchestratorExecution             `json:"orchestratorExecutions,omitempty"`
	WorkerLeases  []OrchestratorWorkerLease           `json:"orchestratorWorkerLeases,omitempty"`
}

type providerProfilePatch struct {
	Name     *string
	Provider *string
	Model    *string
	BaseURL  *string
	AuthRef  *string
	Enabled  *bool
}

var remoteControlStoreMu sync.Mutex
var writeRemoteControlStoreFile = os.WriteFile

func remoteControlStorePath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_REMOTE_CONTROL_STORE")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for remote control store: %w", err)
	}
	return filepath.Join(home, ".carrier", "remote-control.json"), nil
}

func loadRemoteControlState() (*remoteControlState, string, error) {
	path, err := remoteControlStorePath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &remoteControlState{
				Hosts:         []RemoteHost{},
				Profiles:      []ProviderProfile{},
				Bindings:      []ProviderBinding{},
				Policies:      []OrchestratorPolicyRule{},
				Triggers:      []ExecutionTrigger{},
				InstanceSyncs: map[string]RemoteInstanceSyncStatus{},
				Executions:    []OrchestratorExecution{},
				WorkerLeases:  []OrchestratorWorkerLease{},
			}, path, nil
		}
		return nil, "", fmt.Errorf("read remote control store: %w", err)
	}
	var state remoteControlState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, "", fmt.Errorf("parse remote control store: %w", err)
	}
	if state.Hosts == nil {
		state.Hosts = []RemoteHost{}
	}
	if state.Profiles == nil {
		state.Profiles = []ProviderProfile{}
	}
	if state.Bindings == nil {
		state.Bindings = []ProviderBinding{}
	}
	if state.Policies == nil {
		state.Policies = []OrchestratorPolicyRule{}
	}
	if state.Triggers == nil {
		state.Triggers = []ExecutionTrigger{}
	}
	if state.InstanceSyncs == nil {
		state.InstanceSyncs = map[string]RemoteInstanceSyncStatus{}
	}
	if state.Executions == nil {
		state.Executions = []OrchestratorExecution{}
	}
	if state.WorkerLeases == nil {
		state.WorkerLeases = []OrchestratorWorkerLease{}
	}
	return &state, path, nil
}

func saveRemoteControlState(path string, state *remoteControlState) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("remote control store path is empty")
	}
	if state == nil {
		return errors.New("remote control state is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create remote control store dir: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal remote control store: %w", err)
	}
	if err := writeRemoteControlStoreFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write remote control store: %w", err)
	}
	return nil
}
