package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"carrier/daemon/internal/manifest"
)

type upgradeBackup struct {
	AgentID           string            `json:"agent_id"`
	CreatedAt         time.Time         `json:"created_at"`
	CurrentVersion    string            `json:"current_version"`
	EnvVarKeys        []string          `json:"env_var_keys"`
	MemoryAttachments []string          `json:"memory_attachments"`
	RuntimeState      AgentState        `json:"runtime_state"`
	Config            manifest.Manifest `json:"config"`
}

func (s *Service) Upgrade(ctx context.Context, agentID string) (UpgradeResult, error) {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return UpgradeResult{}, err
	}
	fromVersion := state.Version
	if state.Install != InstallStateInstalled {
		return UpgradeResult{}, ErrNotInstalled
	}
	if state.Runtime == RuntimeStateRunning {
		return UpgradeResult{}, ErrAgentRunning
	}
	if strings.TrimSpace(m.Runtime.Upgrade.Command) == "" {
		return UpgradeResult{}, ErrUpgradeNotSupported
	}

	strategy := strings.TrimSpace(m.Upgrade.Strategy)
	if strategy == "" {
		strategy = manifest.UpgradeStrategyInPlaceOrReinstall
	}
	if strategy != manifest.UpgradeStrategyInPlaceOrReinstall {
		return UpgradeResult{}, fmt.Errorf("%w: unsupported strategy %q; supported: %q", ErrUpgradeStrategyUnsupported, strategy, manifest.UpgradeStrategyInPlaceOrReinstall)
	}

	toVersion, err := nextPatchVersion(fromVersion)
	if err != nil {
		s.updateStateOnUpgradeError(agentID, fmt.Errorf("upgrade target version calculation failed: %w", err))
		s.recordAudit("", "system", "upgrade", agentID, AuditResultFailure, "E_UPGRADE_VERSION", err.Error())
		return UpgradeResult{AgentID: agentID, FromVersion: fromVersion}, err
	}

	if err := s.checkRuntimePrerequisites(m); err != nil {
		updateErr := fmt.Errorf("%w: %v", ErrUpgradeFailed, err)
		s.updateStateOnUpgradeError(agentID, updateErr)
		s.recordAudit("", "system", "upgrade", agentID, AuditResultFailure, "E_UPGRADE_PREREQUISITES", err.Error())
		return UpgradeResult{AgentID: agentID, FromVersion: fromVersion}, updateErr
	}

	attachments := s.getMemoryAttachments(agentID)
	backupPath, backupErr := s.createUpgradeBackup(agentID, m, state, attachments)
	if backupErr != nil {
		updateErr := fmt.Errorf("%w: backup failed: %v", ErrUpgradeFailed, backupErr)
		s.updateStateOnUpgradeError(agentID, updateErr)
		s.recordAudit("", "system", "upgrade", agentID, AuditResultFailure, "E_UPGRADE_BACKUP", backupErr.Error())
		return UpgradeResult{AgentID: agentID, FromVersion: fromVersion}, updateErr
	}
	s.recordAudit("", "system", "upgrade", agentID, AuditResultSuccess, "", fmt.Sprintf("upgrade_start backup=%q command=%q", backupPath, m.Runtime.Upgrade.Command))

	result, runErr := s.runner.Run(ctx, m.Runtime.Upgrade.Command)
	s.appendCommandLog(agentID, "upgrade", m.Runtime.Upgrade.Command, result, runErr)
	if runErr != nil {
		updateErr := s.formatUpgradeFailure(runErr, backupPath)
		s.updateStateOnUpgradeError(agentID, updateErr)
		s.recordAudit("", "system", "upgrade", agentID, AuditResultFailure, "E_UPGRADE_FAILED", fmt.Sprintf("upgrade_failure backup=%q error=%q", backupPath, runErr.Error()))
		return UpgradeResult{
			AgentID:     agentID,
			FromVersion: fromVersion,
			ToVersion:   fromVersion,
			BackupPath:  backupPath,
		}, updateErr
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.Version = toVersion
	state.LastError = ""
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.restarts[agentID] = nil
	delete(s.cooldowns, agentID)
	s.memoryLinks[agentID] = append([]string(nil), attachments...)
	s.mu.Unlock()

	s.recordAudit("", "system", "upgrade", agentID, AuditResultSuccess, "", fmt.Sprintf("upgrade_success from=%s to=%s backup=%q", fromVersion, toVersion, backupPath))

	s.saveState()

	return UpgradeResult{
		AgentID:     agentID,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		BackupPath:  backupPath,
	}, nil
}

func (s *Service) formatUpgradeFailure(runErr error, backupPath string) error {
	detail := fmt.Sprintf("upgrade failed: %v", runErr)
	if backupPath != "" {
		return fmt.Errorf("%s. backup captured at %s; manual rollback guidance: restore from this backup path before retrying", detail, backupPath)
	}
	return fmt.Errorf("%s. no backup was captured; manual rollback guidance unavailable", detail)
}

func nextPatchVersion(version string) (string, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid version format %q, expected MAJOR.MINOR.PATCH", version)
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid patch version %q: %w", parts[2], err)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid minor version %q: %w", parts[1], err)
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch+1), nil
}

func (s *Service) envVarKeys(m manifest.Manifest) []string {
	seen := make(map[string]struct{}, len(m.Env.Required)+len(m.Env.Optional))
	keys := make([]string, 0, len(m.Env.Required)+len(m.Env.Optional))
	for _, envVar := range m.Env.Required {
		if _, ok := seen[envVar.Name]; !ok {
			seen[envVar.Name] = struct{}{}
			keys = append(keys, envVar.Name)
		}
	}
	for _, envVar := range m.Env.Optional {
		if _, ok := seen[envVar.Name]; !ok {
			seen[envVar.Name] = struct{}{}
			keys = append(keys, envVar.Name)
		}
	}
	sort.Strings(keys)
	return keys
}

func (s *Service) createUpgradeBackup(agentID string, m manifest.Manifest, state AgentState, attachments []string) (string, error) {
	if err := os.MkdirAll(s.diagnoseDir, 0o755); err != nil {
		return "", fmt.Errorf("create diagnose dir: %w", err)
	}

	fileName := fmt.Sprintf("%s-pre-upgrade-%s.json", agentID, s.now().UTC().Format("2006-01-02T15-04-05Z"))
	filePath := filepath.Join(s.diagnoseDir, fileName)
	backup := upgradeBackup{
		AgentID:           agentID,
		CreatedAt:         s.now().UTC(),
		CurrentVersion:    state.Version,
		EnvVarKeys:        s.envVarKeys(m),
		MemoryAttachments: append([]string(nil), attachments...),
		RuntimeState:      state,
		Config:            m,
	}

	content, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal upgrade backup: %w", err)
	}
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		return "", fmt.Errorf("write upgrade backup: %w", err)
	}

	return filePath, nil
}
