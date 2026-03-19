package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	managedLookPath        = exec.LookPath
	managedInstanceStoreMu sync.Mutex
)

func resolveManagedLimaCtlPath() (string, error) {
	if managedLookPath != nil {
		if path, err := managedLookPath("limactl"); err == nil && strings.TrimSpace(path) != "" {
			return strings.TrimSpace(path), nil
		}
	}
	for _, candidate := range []string{"/opt/homebrew/bin/limactl", "/usr/local/bin/limactl"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("limactl executable not found for managed zeroclaw proxy")
}

func loadLocalZeroClawConfig() (zeroclawLocalConfig, error) {
	home, err := userHomeDirFunc()
	if err != nil {
		return zeroclawLocalConfig{}, err
	}
	raw, err := os.ReadFile(filepath.Join(home, ".zeroclaw", "config.toml"))
	if err != nil {
		return zeroclawLocalConfig{}, err
	}
	return parseZeroClawLocalConfig(raw), nil
}

func managedInstanceStorePath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_INSTANCE_STORE")); custom != "" {
		return custom, nil
	}
	home, err := userHomeDirFunc()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "instances.json"), nil
}

func persistManagedAgentModelRuntime(agentID string, selection managedZeroClawModelSelection) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	storePath, err := managedInstanceStorePath()
	if err != nil {
		return err
	}
	managedInstanceStoreMu.Lock()
	defer managedInstanceStoreMu.Unlock()
	raw, err := os.ReadFile(storePath)
	if err != nil {
		return err
	}
	var store map[string]any
	if err := json.Unmarshal(raw, &store); err != nil {
		return err
	}
	instances, _ := store["instances"].([]any)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updated := false
	for i := range instances {
		entry, _ := instances[i].(map[string]any)
		if !strings.EqualFold(strings.TrimSpace(anyString(entry["agent_id"])), agentID) {
			continue
		}
		entry["model_runtime"] = map[string]any{
			"requested_alias":    strings.TrimSpace(selection.RequestedAlias),
			"requested_model":    strings.TrimSpace(selection.RequestedModel),
			"resolved_model":     strings.TrimSpace(selection.ResolvedModel),
			"resolved_profile":   strings.TrimSpace(selection.ResolvedProfile),
			"fallback_group":     strings.TrimSpace(selection.FallbackGroup),
			"selection_strategy": strings.TrimSpace(selection.SelectionStrategy),
			"selection_ordinal":  selection.SelectionOrdinal,
			"override_hit":       selection.OverrideHit,
			"fallback_hit":       selection.FallbackHit,
			"last_run_at":        now,
		}
		if strings.TrimSpace(selection.cursorGroup) != "" {
			cursors, _ := entry["model_selection_cursors"].(map[string]any)
			if cursors == nil {
				cursors = map[string]any{}
			}
			cursors[strings.TrimSpace(selection.cursorGroup)] = selection.nextCursor
			entry["model_selection_cursors"] = cursors
		}
		entry["updated_at"] = now
		instances[i] = entry
		updated = true
		break
	}
	if !updated {
		return nil
	}
	store["instances"] = instances
	encoded, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storePath, append(encoded, '\n'), 0o600)
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func parseZeroClawLocalConfig(raw []byte) zeroclawLocalConfig {
	cfg := zeroclawLocalConfig{
		Raw: raw,
		Gateway: zeroclawGatewayConfig{
			Host:           "127.0.0.1",
			Port:           9091,
			RequirePairing: true,
		},
	}
	section := ""
	currentProfile := zeroclawProviderProfile{}
	flushProfile := func() {
		if strings.TrimSpace(currentProfile.SectionName) == "" {
			return
		}
		cfg.Profiles = append(cfg.Profiles, currentProfile)
		currentProfile = zeroclawProviderProfile{}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.Index(trimmed, "#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flushProfile()
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			if strings.HasPrefix(section, "provider_profiles.") {
				currentProfile.SectionName = strings.TrimSpace(strings.TrimPrefix(section, "provider_profiles."))
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch {
		case section == "":
			switch key {
			case "default_provider":
				if unquoted, err := strconv.Unquote(value); err == nil {
					cfg.DefaultProvider = strings.TrimSpace(unquoted)
				}
			case "default_model":
				if unquoted, err := strconv.Unquote(value); err == nil {
					cfg.DefaultModel = strings.TrimSpace(unquoted)
				}
			}
		case section == "gateway":
			switch key {
			case "host":
				if unquoted, err := strconv.Unquote(value); err == nil && strings.TrimSpace(unquoted) != "" {
					cfg.Gateway.Host = strings.TrimSpace(unquoted)
				}
			case "port":
				if port, err := strconv.Atoi(value); err == nil && port > 0 {
					cfg.Gateway.Port = port
				}
			case "require_pairing":
				cfg.Gateway.RequirePairing = strings.EqualFold(value, "true")
			}
		case strings.HasPrefix(section, "provider_profiles."):
			switch key {
			case "model_alias":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.ModelAlias = strings.TrimSpace(unquoted)
				}
			case "model":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.Model = strings.TrimSpace(unquoted)
				}
			case "provider":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.Provider = strings.TrimSpace(unquoted)
				}
			case "provider_id":
				if unquoted, err := strconv.Unquote(value); err == nil {
					currentProfile.ProviderID = strings.TrimSpace(unquoted)
				}
			}
		}
	}
	flushProfile()
	return cfg
}

func buildManagedZeroClawModelOverride(cfg zeroclawLocalConfig, selectedModel string) (string, error) {
	if strings.TrimSpace(selectedModel) == "" {
		return "", nil
	}
	rewritten := rewriteZeroClawDefaultModel(cfg.Raw, strings.TrimSpace(selectedModel))
	return base64.StdEncoding.EncodeToString(rewritten), nil
}

func resolveManagedZeroClawModelSelection(agentID string, cfg zeroclawLocalConfig, provider, modelAlias, model string) (managedZeroClawModelSelection, error) {
	selectedModel, err := resolveManagedZeroClawSelectedModel(agentID, cfg, provider, modelAlias, model)
	if err != nil {
		return managedZeroClawModelSelection{}, err
	}
	selection := managedZeroClawModelSelection{
		RequestedAlias: strings.TrimSpace(modelAlias),
		RequestedModel: strings.TrimSpace(model),
		ResolvedModel:  strings.TrimSpace(selectedModel),
	}
	if selection.RequestedAlias != "" || selection.RequestedModel != "" {
		selection.OverrideHit = true
	}
	matchModel := ""
	if selection.RequestedModel != "" {
		matchModel = selection.ResolvedModel
	}
	if profile, primary, ordinal, strategy, ok := findManagedZeroClawProfile(agentID, cfg, provider, selection.RequestedAlias, matchModel); ok {
		selection.ResolvedProfile = strings.TrimSpace(profile.SectionName)
		selection.FallbackGroup = managedZeroClawFallbackGroup(profile)
		if selection.RequestedModel != "" {
			selection.SelectionStrategy = "explicit_model"
		} else {
			selection.SelectionStrategy = strings.TrimSpace(strategy)
		}
		selection.SelectionOrdinal = managedZeroClawProfileOrdinal(cfg, profile)
		selection.FallbackHit = !primary
		if selection.SelectionStrategy == "round_robin" && selection.FallbackGroup != "" {
			selection.cursorGroup = selection.FallbackGroup
			selection.nextCursor = (ordinal + 1) % max(1, countManagedZeroClawProfilesInGroup(cfg, selection.FallbackGroup))
		}
	} else if selection.RequestedModel != "" {
		selection.SelectionStrategy = "explicit_model"
	}
	return selection, nil
}

func findManagedZeroClawProfile(agentID string, cfg zeroclawLocalConfig, provider, alias, model string) (zeroclawProviderProfile, bool, int, string, bool) {
	matchProvider := strings.ToLower(strings.TrimSpace(provider))
	if matchProvider == "" {
		matchProvider = strings.ToLower(strings.TrimSpace(cfg.DefaultProvider))
	}
	groupPrimaryModel := map[string]string{}
	for _, profile := range cfg.Profiles {
		group := managedZeroClawFallbackGroup(profile)
		if group == "" {
			continue
		}
		if _, seen := groupPrimaryModel[group]; !seen {
			groupPrimaryModel[group] = strings.TrimSpace(profile.Model)
		}
	}
	withProvider := make([]zeroclawProviderProfile, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		if model != "" && !strings.EqualFold(strings.TrimSpace(profile.Model), strings.TrimSpace(model)) {
			continue
		}
		if alias != "" && !strings.EqualFold(strings.TrimSpace(profile.ModelAlias), strings.TrimSpace(alias)) {
			continue
		}
		profileProvider := strings.ToLower(strings.TrimSpace(firstNonEmpty(profile.ProviderID, profile.Provider)))
		if matchProvider != "" && profileProvider != "" && profileProvider != matchProvider {
			continue
		}
		withProvider = append(withProvider, profile)
	}
	if len(withProvider) > 0 {
		profile, ordinal, strategy := selectManagedZeroClawProfile(agentID, cfg, withProvider)
		group := managedZeroClawFallbackGroup(profile)
		return profile, strings.EqualFold(strings.TrimSpace(profile.Model), strings.TrimSpace(groupPrimaryModel[group])), ordinal, strategy, true
	}
	withoutProvider := make([]zeroclawProviderProfile, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		if model != "" && !strings.EqualFold(strings.TrimSpace(profile.Model), strings.TrimSpace(model)) {
			continue
		}
		if alias != "" && !strings.EqualFold(strings.TrimSpace(profile.ModelAlias), strings.TrimSpace(alias)) {
			continue
		}
		withoutProvider = append(withoutProvider, profile)
	}
	if len(withoutProvider) > 0 {
		profile, ordinal, strategy := selectManagedZeroClawProfile(agentID, cfg, withoutProvider)
		group := managedZeroClawFallbackGroup(profile)
		return profile, strings.EqualFold(strings.TrimSpace(profile.Model), strings.TrimSpace(groupPrimaryModel[group])), ordinal, strategy, true
	}
	return zeroclawProviderProfile{}, false, 0, "", false
}

func managedZeroClawFallbackGroup(profile zeroclawProviderProfile) string {
	alias := strings.TrimSpace(profile.ModelAlias)
	if alias == "" {
		return ""
	}
	provider := strings.TrimSpace(firstNonEmpty(profile.ProviderID, profile.Provider))
	if provider == "" {
		return strings.ToLower(alias)
	}
	return strings.ToLower(provider) + ":" + strings.ToLower(alias)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveManagedZeroClawSelectedModel(agentID string, cfg zeroclawLocalConfig, provider, modelAlias, model string) (string, error) {
	if trimmedModel := strings.TrimSpace(model); trimmedModel != "" {
		return trimmedModel, nil
	}
	alias := strings.TrimSpace(modelAlias)
	if alias == "" {
		return "", nil
	}
	requestedProvider := strings.ToLower(strings.TrimSpace(provider))
	defaultProvider := strings.ToLower(strings.TrimSpace(cfg.DefaultProvider))
	profile, _, _, _, ok := findManagedZeroClawProfile(agentID, cfg, provider, alias, "")
	if ok && strings.TrimSpace(profile.Model) != "" {
		return strings.TrimSpace(profile.Model), nil
	}
	if requestedProvider != "" || defaultProvider != "" {
		return "", fmt.Errorf("zeroclaw config does not define model alias %q", alias)
	}
	return "", fmt.Errorf("zeroclaw config does not define model alias %q", alias)
}

func selectManagedZeroClawProfile(agentID string, cfg zeroclawLocalConfig, candidates []zeroclawProviderProfile) (zeroclawProviderProfile, int, string) {
	if len(candidates) == 0 {
		return zeroclawProviderProfile{}, 0, ""
	}
	group := managedZeroClawFallbackGroup(candidates[0])
	if len(candidates) == 1 || strings.TrimSpace(group) == "" {
		return candidates[0], 0, "alias_exact"
	}
	cursor := readManagedAgentModelSelectionCursor(agentID, group) % len(candidates)
	return candidates[cursor], cursor, "round_robin"
}

func readManagedAgentModelSelectionCursor(agentID, group string) int {
	agentID = strings.TrimSpace(agentID)
	group = strings.TrimSpace(group)
	if agentID == "" || group == "" {
		return 0
	}
	storePath, err := managedInstanceStorePath()
	if err != nil {
		return 0
	}
	raw, err := os.ReadFile(storePath)
	if err != nil {
		return 0
	}
	var store map[string]any
	if err := json.Unmarshal(raw, &store); err != nil {
		return 0
	}
	instances, _ := store["instances"].([]any)
	for _, item := range instances {
		entry, _ := item.(map[string]any)
		if !strings.EqualFold(strings.TrimSpace(anyString(entry["agent_id"])), agentID) {
			continue
		}
		cursors, _ := entry["model_selection_cursors"].(map[string]any)
		if cursors == nil {
			return 0
		}
		value, ok := cursors[group]
		if !ok {
			return 0
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		}
		return 0
	}
	return 0
}

func countManagedZeroClawProfilesInGroup(cfg zeroclawLocalConfig, group string) int {
	group = strings.TrimSpace(group)
	if group == "" {
		return 0
	}
	count := 0
	for _, profile := range cfg.Profiles {
		if strings.EqualFold(managedZeroClawFallbackGroup(profile), group) {
			count++
		}
	}
	return count
}

func managedZeroClawProfileOrdinal(cfg zeroclawLocalConfig, target zeroclawProviderProfile) int {
	group := managedZeroClawFallbackGroup(target)
	if group == "" {
		return 0
	}
	ordinal := 0
	for _, profile := range cfg.Profiles {
		if !strings.EqualFold(managedZeroClawFallbackGroup(profile), group) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(profile.SectionName), strings.TrimSpace(target.SectionName)) &&
			strings.EqualFold(strings.TrimSpace(profile.Model), strings.TrimSpace(target.Model)) {
			return ordinal
		}
		ordinal++
	}
	return 0
}

func rewriteZeroClawDefaultModel(raw []byte, model string) []byte {
	lines := strings.Split(string(raw), "\n")
	replaced := false
	insertAt := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if insertAt < 0 {
				insertAt = i
			}
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "default_model") {
			lines[i] = fmt.Sprintf("default_model = %s", strconv.Quote(strings.TrimSpace(model)))
			replaced = true
			break
		}
	}
	if !replaced {
		if insertAt < 0 {
			insertAt = len(lines)
		}
		newLine := fmt.Sprintf("default_model = %s", strconv.Quote(strings.TrimSpace(model)))
		lines = append(lines[:insertAt], append([]string{newLine}, lines[insertAt:]...)...)
	}
	text := strings.Join(lines, "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text)
}
