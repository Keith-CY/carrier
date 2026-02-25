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
	Hosts    []RemoteHost      `json:"hosts"`
	Profiles []ProviderProfile `json:"providerProfiles"`
	Bindings []ProviderBinding `json:"providerBindings"`
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
			return &remoteControlState{Hosts: []RemoteHost{}, Profiles: []ProviderProfile{}, Bindings: []ProviderBinding{}}, path, nil
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
		if strings.TrimSpace(patch.SSHConfigHost) != "" {
			merged.SSHConfigHost = patch.SSHConfigHost
		}
		if strings.TrimSpace(string(patch.RuntimeMode)) != "" {
			merged.RuntimeMode = patch.RuntimeMode
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
