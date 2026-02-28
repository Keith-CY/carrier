package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"carrier/daemon/internal/manifest"
)

type processSignaler interface {
	Signal(agentID string, sig os.Signal) error
}

func (s *Service) HotReloadConfig(agentID string, changes map[string]string) error {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}
	if state.Install != InstallStateInstalled {
		return ErrNotInstalled
	}
	if err := updateAgentConfig(agentID, changes); err != nil {
		return err
	}
	if manifestSupportsHotReload(m) {
		signaler, ok := s.processManager.(processSignaler)
		if !ok {
			return errors.New("process manager does not support signaling")
		}
		return signaler.Signal(agentID, syscall.SIGHUP)
	}
	if err := s.Stop(context.Background(), agentID); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	if err := s.Start(context.Background(), agentID); err != nil {
		return err
	}
	return nil
}

func manifestSupportsHotReload(m manifest.Manifest) bool {
	for _, cap := range m.Capabilities {
		if strings.EqualFold(strings.TrimSpace(cap), "hot_reload") {
			return true
		}
	}
	return false
}

func updateAgentConfig(agentID string, changes map[string]string) error {
	if len(changes) == 0 {
		return nil
	}
	path, err := hotReloadConfigPath(agentID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload := map[string]string{}
	if raw, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(raw))) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	for key, value := range changes {
		payload[key] = value
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func hotReloadConfigPath(agentID string) (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_AGENT_CONFIG_DIR")); custom != "" {
		return filepath.Join(custom, strings.TrimSpace(agentID)+".json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "agents", strings.TrimSpace(agentID)+".json"), nil
}
