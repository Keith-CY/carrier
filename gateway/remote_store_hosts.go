package gateway

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

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
