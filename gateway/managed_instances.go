package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type managedAgentInstance struct {
	ID                  string                    `json:"id"`
	Type                string                    `json:"type"`
	AgentID             string                    `json:"agent_id"`
	Isolation           bool                      `json:"isolation,omitempty"`
	GatewayURL          string                    `json:"gateway_url"`
	Workspace           string                    `json:"workspace_path,omitempty"`
	ConfigPath          string                    `json:"config_path,omitempty"`
	RecordPath          string                    `json:"record_path,omitempty"`
	Channel             string                    `json:"channel,omitempty"`
	Provider            string                    `json:"provider,omitempty"`
	ModelSurface        *managedAgentModelSurface `json:"model_surface,omitempty"`
	ModelRuntime        *managedAgentModelRuntime `json:"model_runtime,omitempty"`
	ModelSelectionCursors map[string]int          `json:"model_selection_cursors,omitempty"`
	PairRequired        bool                      `json:"pair_required,omitempty"`
	PairCode            string                    `json:"pair_code,omitempty"`
	PairedChatID        string                    `json:"paired_chat_id,omitempty"`
	RuntimeState        string                    `json:"runtime_state,omitempty"`
	CreatedAt           string                    `json:"created_at"`
	UpdatedAt           string                    `json:"updated_at"`
}

type managedAgentModelSurface struct {
	DefaultProfile string                     `json:"default_profile,omitempty"`
	Profiles       []managedAgentModelProfile `json:"profiles,omitempty"`
}

type managedAgentModelRuntime struct {
	RequestedAlias   string `json:"requested_alias,omitempty"`
	RequestedModel   string `json:"requested_model,omitempty"`
	ResolvedModel    string `json:"resolved_model,omitempty"`
	ResolvedProfile  string `json:"resolved_profile,omitempty"`
	FallbackGroup    string `json:"fallback_group,omitempty"`
	SelectionStrategy string `json:"selection_strategy,omitempty"`
	SelectionOrdinal int    `json:"selection_ordinal,omitempty"`
	OverrideHit      bool   `json:"override_hit,omitempty"`
	FallbackHit      bool   `json:"fallback_hit,omitempty"`
	LastRunAt        string `json:"last_run_at,omitempty"`
}

type managedAgentModelProfile struct {
	ProfileName    string `json:"profile_name,omitempty"`
	ModelAlias     string `json:"model_alias,omitempty"`
	ModelID        string `json:"model_id,omitempty"`
	ProviderID     string `json:"provider_id,omitempty"`
	ProviderKey    string `json:"provider_key,omitempty"`
	ProtocolFamily string `json:"protocol_family,omitempty"`
	BaseURL        string `json:"base_url,omitempty"`
	AuthMethod     string `json:"auth_method,omitempty"`
	TimeoutMs      int    `json:"timeout_ms,omitempty"`
	RetryBudget    int    `json:"retry_budget,omitempty"`
	FallbackStrategy string `json:"fallback_strategy,omitempty"`
	FallbackGroup  string `json:"fallback_group,omitempty"`
	AliasGroupSize int    `json:"alias_group_size,omitempty"`
	Primary        bool   `json:"primary,omitempty"`
}

type managedAgentInstanceFile struct {
	Instances []managedAgentInstance `json:"instances"`
}

var managedInstanceRandReader io.Reader = rand.Reader

func managedInstancesPath() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_INSTANCE_STORE")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for instance store: %w", err)
	}
	return filepath.Join(home, ".carrier", "instances.json"), nil
}

func generateManagedInstanceID(prefix string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		p = "agent"
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(managedInstanceRandReader, buf); err != nil {
		return "", fmt.Errorf("generate random id: %w", err)
	}
	return fmt.Sprintf("%s-%s", p, hex.EncodeToString(buf)), nil
}

func loadManagedInstances() ([]managedAgentInstance, string, error) {
	path, err := managedInstancesPath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []managedAgentInstance{}, path, nil
		}
		return nil, "", fmt.Errorf("read instance store: %w", err)
	}
	var file managedAgentInstanceFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, "", fmt.Errorf("parse instance store: %w", err)
	}
	if file.Instances == nil {
		file.Instances = []managedAgentInstance{}
	}
	for i := range file.Instances {
		file.Instances[i].Channel = normalizeManagedInstanceChannel(file.Instances[i].Channel)
	}
	return file.Instances, path, nil
}

func saveManagedInstances(path string, instances []managedAgentInstance) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("instance store path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create instance store dir: %w", err)
	}
	payload := managedAgentInstanceFile{Instances: instances}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal instance store: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write instance store: %w", err)
	}
	return nil
}

func findManagedInstanceIndex(instances []managedAgentInstance, instanceID string) int {
	id := strings.TrimSpace(instanceID)
	for i, inst := range instances {
		if strings.EqualFold(strings.TrimSpace(inst.ID), id) {
			return i
		}
	}
	return -1
}

func findManagedInstanceIndexByAgentID(instances []managedAgentInstance, agentID string) int {
	id := strings.TrimSpace(agentID)
	for i, inst := range instances {
		if strings.EqualFold(strings.TrimSpace(inst.AgentID), id) {
			return i
		}
	}
	return -1
}

func upsertManagedInstance(inst managedAgentInstance) error {
	inst.Channel = normalizeManagedInstanceChannel(inst.Channel)
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	idx := findManagedInstanceIndex(instances, inst.ID)
	if idx >= 0 {
		instances[idx] = inst
	} else {
		instances = append(instances, inst)
	}
	return saveManagedInstances(path, instances)
}

func normalizeManagedInstanceChannel(raw string) string {
	channel := strings.TrimSpace(raw)
	if channel == "" {
		return ""
	}
	channelID, err := NormalizeChannelID(channel)
	if err != nil {
		// Keep unknown values to preserve backward compatibility for older records.
		return channel
	}
	return string(channelID)
}

func cleanupManagedInstanceFiles(inst managedAgentInstance) error {
	var firstErr error
	removeFile := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	removeDir := func(path string) {
		p := strings.TrimSpace(path)
		if p == "" {
			return
		}
		if err := os.RemoveAll(p); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}

	removeFile(strings.TrimSpace(inst.RecordPath))
	removeFile(strings.TrimSpace(inst.ConfigPath))
	removeDir(strings.TrimSpace(inst.Workspace))
	return firstErr
}
