package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
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
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write remote control store: %w", err)
	}
	return nil
}

func listRemoteHosts() ([]RemoteHost, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()
	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]RemoteHost, len(state.Hosts))
	copy(out, state.Hosts)
	return out, nil
}

func getRemoteHost(hostID string) (RemoteHost, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()
	state, _, err := loadRemoteControlState()
	if err != nil {
		return RemoteHost{}, false, err
	}
	id := strings.TrimSpace(hostID)
	for _, host := range state.Hosts {
		if strings.EqualFold(strings.TrimSpace(host.ID), id) {
			return host, true, nil
		}
	}
	return RemoteHost{}, false, nil
}

func upsertRemoteHost(host RemoteHost) (RemoteHost, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return RemoteHost{}, err
	}
	h := normalizeRemoteHost(host)
	if err := validateRemoteHost(h); err != nil {
		return RemoteHost{}, err
	}
	now := nowTimestamp()
	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	if h.CreatedAt == "" {
		h.CreatedAt = now
	}
	h.UpdatedAt = now

	updated := false
	for i := range state.Hosts {
		if strings.EqualFold(strings.TrimSpace(state.Hosts[i].ID), h.ID) {
			h.CreatedAt = state.Hosts[i].CreatedAt
			state.Hosts[i] = h
			updated = true
			break
		}
	}
	if !updated {
		state.Hosts = append(state.Hosts, h)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return RemoteHost{}, err
	}
	return h, nil
}

func patchRemoteHost(hostID string, patch RemoteHost) (RemoteHost, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return RemoteHost{}, err
	}
	id := strings.TrimSpace(hostID)
	for i := range state.Hosts {
		if !strings.EqualFold(strings.TrimSpace(state.Hosts[i].ID), id) {
			continue
		}
		existing := state.Hosts[i]
		merged := existing
		if strings.TrimSpace(patch.Name) != "" {
			merged.Name = patch.Name
		}
		if strings.TrimSpace(patch.Host) != "" {
			merged.Host = patch.Host
		}
		if patch.Port != 0 {
			merged.Port = patch.Port
		}
		if strings.TrimSpace(patch.User) != "" {
			merged.User = patch.User
		}
		if strings.TrimSpace(string(patch.AuthMode)) != "" {
			merged.AuthMode = patch.AuthMode
		}
		if strings.TrimSpace(patch.KeyPath) != "" {
			merged.KeyPath = patch.KeyPath
		}
		if strings.TrimSpace(patch.KeyRef) != "" {
			merged.KeyRef = patch.KeyRef
		}
		if strings.TrimSpace(patch.SSHConfigHost) != "" {
			merged.SSHConfigHost = patch.SSHConfigHost
		}
		if strings.TrimSpace(string(patch.RuntimeMode)) != "" {
			merged.RuntimeMode = patch.RuntimeMode
		}
		if patch.Labels != nil {
			merged.Labels = patch.Labels
		}
		merged = normalizeRemoteHost(merged)
		if err := validateRemoteHost(merged); err != nil {
			return RemoteHost{}, err
		}
		merged.UpdatedAt = nowTimestamp()
		state.Hosts[i] = merged
		if err := saveRemoteControlState(path, state); err != nil {
			return RemoteHost{}, err
		}
		return merged, nil
	}
	return RemoteHost{}, fmt.Errorf("host %s not found", hostID)
}

func deleteRemoteHost(hostID string) (bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return false, err
	}
	id := strings.TrimSpace(hostID)
	before := len(state.Hosts)
	filtered := state.Hosts[:0]
	for _, host := range state.Hosts {
		if strings.EqualFold(strings.TrimSpace(host.ID), id) {
			continue
		}
		filtered = append(filtered, host)
	}
	state.Hosts = filtered
	if len(state.Hosts) == before {
		return false, nil
	}
	bindings := state.Bindings[:0]
	for _, binding := range state.Bindings {
		if strings.EqualFold(binding.TargetType, "host") && strings.EqualFold(binding.TargetID, id) {
			continue
		}
		bindings = append(bindings, binding)
	}
	state.Bindings = bindings
	for key, syncStatus := range state.InstanceSyncs {
		if strings.EqualFold(strings.TrimSpace(syncStatus.HostID), id) {
			delete(state.InstanceSyncs, key)
		}
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return false, err
	}
	return true, nil
}

func updateRemoteHostHealth(hostID string, health RemoteHealth, detail string) error {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()
	state, path, err := loadRemoteControlState()
	if err != nil {
		return err
	}
	id := strings.TrimSpace(hostID)
	for i := range state.Hosts {
		if !strings.EqualFold(strings.TrimSpace(state.Hosts[i].ID), id) {
			continue
		}
		state.Hosts[i].LastHealth = health
		state.Hosts[i].LastCheckAt = nowTimestamp()
		state.Hosts[i].LastError = strings.TrimSpace(detail)
		state.Hosts[i].UpdatedAt = nowTimestamp()
		return saveRemoteControlState(path, state)
	}
	return fmt.Errorf("host %s not found", hostID)
}

func listProviderProfiles() ([]ProviderProfile, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()
	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]ProviderProfile, len(state.Profiles))
	copy(out, state.Profiles)
	return out, nil
}

func getProviderProfile(profileID string) (ProviderProfile, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()
	state, _, err := loadRemoteControlState()
	if err != nil {
		return ProviderProfile{}, false, err
	}
	id := strings.TrimSpace(profileID)
	for _, profile := range state.Profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ID), id) {
			return profile, true, nil
		}
	}
	return ProviderProfile{}, false, nil
}

func upsertProviderProfile(profile ProviderProfile) (ProviderProfile, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return ProviderProfile{}, err
	}
	p := normalizeProviderProfile(profile)
	if err := validateProviderProfile(p); err != nil {
		return ProviderProfile{}, err
	}
	now := nowTimestamp()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Name == "" {
		p.Name = p.Provider + "/" + p.Model
	}
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	updated := false
	for i := range state.Profiles {
		if strings.EqualFold(strings.TrimSpace(state.Profiles[i].ID), p.ID) {
			p.CreatedAt = state.Profiles[i].CreatedAt
			state.Profiles[i] = p
			updated = true
			break
		}
	}
	if !updated {
		state.Profiles = append(state.Profiles, p)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return ProviderProfile{}, err
	}
	return p, nil
}

func patchProviderProfile(profileID string, patch providerProfilePatch) (ProviderProfile, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return ProviderProfile{}, err
	}
	id := strings.TrimSpace(profileID)
	for i := range state.Profiles {
		if !strings.EqualFold(strings.TrimSpace(state.Profiles[i].ID), id) {
			continue
		}
		existing := state.Profiles[i]
		merged := existing
		if patch.Name != nil {
			merged.Name = strings.TrimSpace(*patch.Name)
		}
		if patch.Provider != nil {
			merged.Provider = strings.TrimSpace(*patch.Provider)
		}
		if patch.Model != nil {
			merged.Model = strings.TrimSpace(*patch.Model)
		}
		if patch.BaseURL != nil {
			merged.BaseURL = strings.TrimSpace(*patch.BaseURL)
		}
		if patch.AuthRef != nil {
			merged.AuthRef = strings.TrimSpace(*patch.AuthRef)
		}
		if patch.Enabled != nil {
			merged.Enabled = *patch.Enabled
		}
		merged = normalizeProviderProfile(merged)
		if err := validateProviderProfile(merged); err != nil {
			return ProviderProfile{}, err
		}
		merged.UpdatedAt = nowTimestamp()
		state.Profiles[i] = merged
		if err := saveRemoteControlState(path, state); err != nil {
			return ProviderProfile{}, err
		}
		return merged, nil
	}
	return ProviderProfile{}, fmt.Errorf("profile %s not found", profileID)
}

func deleteProviderProfile(profileID string) (bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return false, err
	}
	id := strings.TrimSpace(profileID)
	before := len(state.Profiles)
	profiles := state.Profiles[:0]
	for _, profile := range state.Profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ID), id) {
			continue
		}
		profiles = append(profiles, profile)
	}
	state.Profiles = profiles
	if len(state.Profiles) == before {
		return false, nil
	}
	bindings := state.Bindings[:0]
	for _, binding := range state.Bindings {
		if strings.EqualFold(strings.TrimSpace(binding.ProfileID), id) {
			continue
		}
		bindings = append(bindings, binding)
	}
	state.Bindings = bindings
	if err := saveRemoteControlState(path, state); err != nil {
		return false, err
	}
	return true, nil
}

func listProviderBindings() ([]ProviderBinding, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()
	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]ProviderBinding, len(state.Bindings))
	copy(out, state.Bindings)
	return out, nil
}

func upsertProviderBinding(binding ProviderBinding) (ProviderBinding, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return ProviderBinding{}, err
	}
	b := normalizeProviderBinding(binding)
	if err := validateProviderBinding(b); err != nil {
		return ProviderBinding{}, err
	}
	now := nowTimestamp()
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	if b.CreatedAt == "" {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	updated := false
	for i := range state.Bindings {
		if strings.EqualFold(strings.TrimSpace(state.Bindings[i].ID), b.ID) {
			b.CreatedAt = state.Bindings[i].CreatedAt
			state.Bindings[i] = b
			updated = true
			break
		}
	}
	if !updated {
		state.Bindings = append(state.Bindings, b)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return ProviderBinding{}, err
	}
	return b, nil
}

func deleteProviderBinding(bindingID string) (bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return false, err
	}
	id := strings.TrimSpace(bindingID)
	before := len(state.Bindings)
	filtered := state.Bindings[:0]
	for _, binding := range state.Bindings {
		if strings.EqualFold(strings.TrimSpace(binding.ID), id) {
			continue
		}
		filtered = append(filtered, binding)
	}
	state.Bindings = filtered
	if len(state.Bindings) == before {
		return false, nil
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return false, err
	}
	return true, nil
}

func listOrchestratorPolicies() ([]OrchestratorPolicyRule, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]OrchestratorPolicyRule, len(state.Policies))
	copy(out, state.Policies)
	return sortOrchestratorPolicyRules(out), nil
}

func listExecutionTriggers() ([]ExecutionTrigger, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]ExecutionTrigger, len(state.Triggers))
	copy(out, state.Triggers)
	return out, nil
}

func getExecutionTrigger(triggerID string) (ExecutionTrigger, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return ExecutionTrigger{}, false, err
	}
	id := strings.TrimSpace(triggerID)
	for _, trigger := range state.Triggers {
		if strings.EqualFold(strings.TrimSpace(trigger.ID), id) {
			return trigger, true, nil
		}
	}
	return ExecutionTrigger{}, false, nil
}

func upsertExecutionTrigger(trigger ExecutionTrigger) (ExecutionTrigger, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return ExecutionTrigger{}, err
	}
	trigger, err = normalizeExecutionTriggerForStore(trigger)
	if err != nil {
		return ExecutionTrigger{}, err
	}
	now := nowTimestamp()
	trigger.UpdatedAt = now
	if trigger.CreatedAt == "" {
		trigger.CreatedAt = now
	}
	updated := false
	for i := range state.Triggers {
		if strings.EqualFold(strings.TrimSpace(state.Triggers[i].ID), trigger.ID) {
			trigger.CreatedAt = state.Triggers[i].CreatedAt
			state.Triggers[i] = trigger
			updated = true
			break
		}
	}
	if !updated {
		state.Triggers = append(state.Triggers, trigger)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return ExecutionTrigger{}, err
	}
	return trigger, nil
}

func deleteExecutionTrigger(triggerID string) error {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return err
	}
	id := strings.TrimSpace(triggerID)
	filtered := state.Triggers[:0]
	found := false
	for _, trigger := range state.Triggers {
		if strings.EqualFold(strings.TrimSpace(trigger.ID), id) {
			found = true
			continue
		}
		filtered = append(filtered, trigger)
	}
	if !found {
		return os.ErrNotExist
	}
	state.Triggers = append([]ExecutionTrigger(nil), filtered...)
	return saveRemoteControlState(path, state)
}

func upsertOrchestratorPolicy(rule OrchestratorPolicyRule) (OrchestratorPolicyRule, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorPolicyRule{}, err
	}
	normalized := normalizeOrchestratorPolicyRule(rule)
	if err := validateOrchestratorPolicyRule(normalized); err != nil {
		return OrchestratorPolicyRule{}, err
	}
	now := nowTimestamp()
	if normalized.ID == "" {
		normalized.ID = uuid.NewString()
	}
	if normalized.CreatedAt == "" {
		normalized.CreatedAt = now
	}
	normalized.UpdatedAt = now

	updated := false
	for i := range state.Policies {
		if strings.EqualFold(strings.TrimSpace(state.Policies[i].ID), normalized.ID) {
			normalized.CreatedAt = state.Policies[i].CreatedAt
			state.Policies[i] = normalized
			updated = true
			break
		}
	}
	if !updated {
		state.Policies = append(state.Policies, normalized)
	}
	state.Policies = sortOrchestratorPolicyRules(state.Policies)
	if err := saveRemoteControlState(path, state); err != nil {
		return OrchestratorPolicyRule{}, err
	}
	return normalized, nil
}

func deleteOrchestratorPolicy(policyID string) (bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return false, err
	}
	id := strings.TrimSpace(policyID)
	before := len(state.Policies)
	filtered := state.Policies[:0]
	for _, policy := range state.Policies {
		if strings.EqualFold(strings.TrimSpace(policy.ID), id) {
			continue
		}
		filtered = append(filtered, policy)
	}
	state.Policies = filtered
	if len(state.Policies) == before {
		return false, nil
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return false, err
	}
	return true, nil
}

func remoteInstanceSyncKey(hostID, agentID string) string {
	return strings.ToLower(strings.TrimSpace(hostID) + ":" + strings.TrimSpace(agentID))
}

func getRemoteInstanceSyncStatus(hostID, agentID string) (RemoteInstanceSyncStatus, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return RemoteInstanceSyncStatus{}, false, err
	}
	key := remoteInstanceSyncKey(hostID, agentID)
	status, ok := state.InstanceSyncs[key]
	if !ok {
		return RemoteInstanceSyncStatus{}, false, nil
	}
	return status, true, nil
}

func listRemoteInstanceSyncStatuses() ([]RemoteInstanceSyncStatus, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]RemoteInstanceSyncStatus, 0, len(state.InstanceSyncs))
	for _, status := range state.InstanceSyncs {
		out = append(out, status)
	}
	return out, nil
}

func upsertRemoteInstanceSyncStatus(status RemoteInstanceSyncStatus) (RemoteInstanceSyncStatus, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return RemoteInstanceSyncStatus{}, err
	}
	hostID := strings.TrimSpace(status.HostID)
	agentID := strings.TrimSpace(status.AgentID)
	if hostID == "" || agentID == "" {
		return RemoteInstanceSyncStatus{}, fmt.Errorf("hostId and agentId are required")
	}
	status.HostID = hostID
	status.AgentID = agentID
	status.SyncMode = normalizeProviderBindingSyncMode(status.SyncMode)
	if err := validateProviderBindingSyncMode(status.SyncMode); err != nil {
		return RemoteInstanceSyncStatus{}, err
	}
	if strings.TrimSpace(status.DriftState) == "" {
		status.DriftState = "unknown"
	}
	if strings.TrimSpace(status.LastSyncStatus) == "" {
		status.LastSyncStatus = "unknown"
	}
	if strings.TrimSpace(status.MemoryLastSyncStatus) == "" {
		status.MemoryLastSyncStatus = "unknown"
	}
	status.MemoryGit = normalizeRemoteMemoryGitConfig(status.MemoryGit)
	status.UpdatedAt = nowTimestamp()
	key := remoteInstanceSyncKey(hostID, agentID)
	state.InstanceSyncs[key] = status
	if err := saveRemoteControlState(path, state); err != nil {
		return RemoteInstanceSyncStatus{}, err
	}
	return status, nil
}

func deleteRemoteInstanceSyncStatus(hostID, agentID string) error {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return err
	}
	delete(state.InstanceSyncs, remoteInstanceSyncKey(hostID, agentID))
	return saveRemoteControlState(path, state)
}

func listOrchestratorExecutions() ([]OrchestratorExecution, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]OrchestratorExecution, len(state.Executions))
	copy(out, state.Executions)
	return out, nil
}

func getOrchestratorExecution(executionID string) (OrchestratorExecution, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorExecution{}, false, err
	}
	id := strings.TrimSpace(executionID)
	for _, execution := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(execution.ID), id) {
			return execution, true, nil
		}
	}
	return OrchestratorExecution{}, false, nil
}

func findOrchestratorExecutionByIdempotencyKey(idempotencyKey string) (OrchestratorExecution, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorExecution{}, false, err
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return OrchestratorExecution{}, false, nil
	}
	for _, execution := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(execution.IdempotencyKey), key) {
			return execution, true, nil
		}
	}
	return OrchestratorExecution{}, false, nil
}

func upsertOrchestratorExecution(execution OrchestratorExecution) (OrchestratorExecution, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorExecution{}, err
	}
	execution = normalizeOrchestratorExecutionForStore(execution)
	id := execution.ID
	if id == "" {
		return OrchestratorExecution{}, fmt.Errorf("execution id is required")
	}
	execution.UpdatedAt = nowTimestamp()
	if execution.CreatedAt == "" {
		execution.CreatedAt = execution.UpdatedAt
	}

	updated := false
	for i := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(state.Executions[i].ID), id) {
			execution.CreatedAt = state.Executions[i].CreatedAt
			state.Executions[i] = execution
			updated = true
			break
		}
	}
	if !updated {
		state.Executions = append(state.Executions, execution)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return OrchestratorExecution{}, err
	}
	return execution, nil
}

func listOrchestratorWorkerLeases() ([]OrchestratorWorkerLease, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]OrchestratorWorkerLease, len(state.WorkerLeases))
	copy(out, state.WorkerLeases)
	return out, nil
}

func listOrchestratorWorkerLeasesByExecution(executionID string) ([]OrchestratorWorkerLease, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(executionID)
	out := make([]OrchestratorWorkerLease, 0)
	for _, lease := range state.WorkerLeases {
		if strings.EqualFold(strings.TrimSpace(lease.ExecutionID), id) {
			out = append(out, lease)
		}
	}
	return out, nil
}

func upsertOrchestratorWorkerLease(lease OrchestratorWorkerLease) (OrchestratorWorkerLease, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorWorkerLease{}, err
	}
	lease = normalizeOrchestratorWorkerLeaseForStore(lease)
	id := lease.ID
	if id == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("worker lease id is required")
	}
	if lease.ExecutionID == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("executionId is required")
	}
	if lease.HostID == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("hostId is required")
	}
	if lease.AgentID == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("agentId is required")
	}
	lease.UpdatedAt = nowTimestamp()
	if lease.CreatedAt == "" {
		lease.CreatedAt = lease.UpdatedAt
	}
	if lease.HeartbeatAt == "" {
		lease.HeartbeatAt = lease.UpdatedAt
	}
	if lease.LastHeartbeatAt == "" {
		lease.LastHeartbeatAt = lease.HeartbeatAt
	}
	if lease.LeaseExpireAt == "" {
		lease.LeaseExpireAt = lease.UpdatedAt
	}

	updated := false
	for i := range state.WorkerLeases {
		if strings.EqualFold(strings.TrimSpace(state.WorkerLeases[i].ID), id) {
			lease.CreatedAt = state.WorkerLeases[i].CreatedAt
			state.WorkerLeases[i] = lease
			updated = true
			break
		}
	}
	if !updated {
		state.WorkerLeases = append(state.WorkerLeases, lease)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return OrchestratorWorkerLease{}, err
	}
	return lease, nil
}

func deleteOrchestratorWorkerLease(leaseID string) error {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return err
	}
	id := strings.TrimSpace(leaseID)
	filtered := state.WorkerLeases[:0]
	for _, lease := range state.WorkerLeases {
		if strings.EqualFold(strings.TrimSpace(lease.ID), id) {
			continue
		}
		filtered = append(filtered, lease)
	}
	state.WorkerLeases = filtered
	return saveRemoteControlState(path, state)
}
