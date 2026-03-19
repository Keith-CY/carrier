package gateway

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

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
