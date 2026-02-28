package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"carrier/profilesync"
	"carrier/shared/openclawcfg"
)

const (
	remoteOpenClawConfigPath         = "$HOME/.openclaw/openclaw.json"
	remoteOpenClawCarrierSecretsPath = "$HOME/.openclaw/carrier-secrets.json"
	remotePicoClawConfigPath         = "$HOME/.picoclaw/config.json"
	remoteZeroClawConfigPath         = "$HOME/.zeroclaw/config.toml"
)

type remoteDiscoveryPullOptions struct {
	PullNew         bool
	PullNewAgentIDs map[string]bool
}

func (o remoteDiscoveryPullOptions) allowPullNewFor(agentID string) bool {
	if !o.PullNew {
		return false
	}
	if len(o.PullNewAgentIDs) == 0 {
		return true
	}
	return o.PullNewAgentIDs[strings.ToLower(strings.TrimSpace(agentID))]
}

type remoteHostCheckResult struct {
	SSHOK          bool               `json:"sshOk"`
	OpenClawFound  bool               `json:"openclawFound"`
	GatewayHealthy bool               `json:"gatewayHealthy"`
	Repaired       bool               `json:"repaired"`
	Details        []string           `json:"details,omitempty"`
	Steps          []remoteExecResult `json:"steps,omitempty"`
}

type remoteRepairResult struct {
	HostID         string             `json:"hostId"`
	AgentID        string             `json:"agentId"`
	GatewayHealthy bool               `json:"gatewayHealthy"`
	Repaired       bool               `json:"repaired"`
	Steps          []remoteExecResult `json:"steps"`
	LogTail        string             `json:"logTail,omitempty"`
}

type remoteInstallResult struct {
	HostID      string             `json:"hostId"`
	AgentID     string             `json:"agentId"`
	Installed   bool               `json:"installed"`
	GatewayMode RemoteRuntimeMode  `json:"gatewayMode"`
	Steps       []remoteExecResult `json:"steps"`
}

type remoteSyncResult struct {
	HostID         string `json:"hostId"`
	AgentID        string `json:"agentId"`
	Mode           string `json:"mode"`
	Status         string `json:"status"`
	DriftState     string `json:"driftState"`
	LastRemoteHash string `json:"lastRemoteHash,omitempty"`
	LastSyncAt     string `json:"lastSyncAt,omitempty"`
}

type remoteDiagnoseResult struct {
	HostID         string `json:"hostId"`
	AgentID        string `json:"agentId"`
	Result         string `json:"result"`
	DriftState     string `json:"driftState"`
	LastRemoteHash string `json:"lastRemoteHash,omitempty"`
	LastDiagnoseAt string `json:"lastDiagnoseAt,omitempty"`
}

type remoteReconcileResult struct {
	HostID          string `json:"hostId"`
	AgentID         string `json:"agentId"`
	Reconciled      bool   `json:"reconciled"`
	DriftState      string `json:"driftState"`
	LastRemoteHash  string `json:"lastRemoteHash,omitempty"`
	LastReconcileAt string `json:"lastReconcileAt,omitempty"`
}

type remoteRollbackResult struct {
	HostID           string `json:"hostId"`
	AgentID          string `json:"agentId"`
	RolledBack       bool   `json:"rolledBack"`
	FromCommit       string `json:"fromCommit,omitempty"`
	NewCommit        string `json:"newCommit,omitempty"`
	DriftState       string `json:"driftState"`
	LastRemoteHash   string `json:"lastRemoteHash,omitempty"`
	LastRollbackAt   string `json:"lastRollbackAt,omitempty"`
	RestoredSnapshot bool   `json:"restoredSnapshot"`
}

func checkRemoteHostAndMaybeRepair(ctx context.Context, host RemoteHost) (remoteHostCheckResult, error) {
	result := remoteHostCheckResult{Details: []string{}, Steps: []remoteExecResult{}}
	checkRes, err := runRemoteCommand(ctx, host, "echo carrier-ssh-ok")
	if err != nil {
		return result, err
	}
	result.Steps = append(result.Steps, checkRes)
	if checkRes.ExitCode != 0 {
		result.Details = append(result.Details, "ssh connectivity check failed")
		return result, remoteCommandError(checkRes, "ssh connectivity check")
	}
	result.SSHOK = true

	openClawRes, err := runRemoteCommand(ctx, host, "command -v openclaw >/dev/null 2>&1")
	if err != nil {
		return result, err
	}
	result.Steps = append(result.Steps, openClawRes)
	if openClawRes.ExitCode != 0 {
		result.Details = append(result.Details, "openclaw executable is missing")
		return result, nil
	}
	result.OpenClawFound = true

	healthy, repaired, repairSteps, repairErr := ensureRemoteHealthyForOperation(ctx, host)
	result.GatewayHealthy = healthy
	result.Repaired = repaired
	result.Steps = append(result.Steps, repairSteps...)
	if repairErr != nil {
		result.Details = append(result.Details, repairErr.Error())
		return result, repairErr
	}
	return result, nil
}

func ensureRemoteHealthyForOperation(ctx context.Context, host RemoteHost) (healthy bool, repaired bool, steps []remoteExecResult, err error) {
	steps = []remoteExecResult{}
	if host.RuntimeMode == RemoteRuntimeModeOnDemand {
		checkRes, runErr := runRemoteCommand(ctx, host, "command -v openclaw >/dev/null 2>&1")
		if runErr != nil {
			return false, false, steps, runErr
		}
		steps = append(steps, checkRes)
		if checkRes.ExitCode != 0 {
			return false, false, steps, remoteCommandError(checkRes, "openclaw preflight check")
		}
		return true, false, steps, nil
	}

	statusRes, runErr := runRemoteCommand(ctx, host, "openclaw gateway status 2>&1")
	if runErr != nil {
		return false, false, steps, runErr
	}
	steps = append(steps, statusRes)
	if statusRes.ExitCode == 0 {
		return true, false, steps, nil
	}

	restartRes, runErr := runRemoteCommand(ctx, host, "openclaw gateway restart 2>&1 || openclaw gateway start 2>&1")
	if runErr != nil {
		remoteMetrics.recordRepairAttempt(false)
		return false, false, steps, runErr
	}
	steps = append(steps, restartRes)

	statusRetryRes, runErr := runRemoteCommand(ctx, host, "openclaw gateway status 2>&1")
	if runErr != nil {
		remoteMetrics.recordRepairAttempt(false)
		return false, true, steps, runErr
	}
	steps = append(steps, statusRetryRes)
	if statusRetryRes.ExitCode != 0 {
		remoteMetrics.recordRepairAttempt(false)
		return false, true, steps, remoteCommandError(statusRetryRes, "gateway post-repair status check")
	}
	remoteMetrics.recordRepairAttempt(true)
	return true, true, steps, nil
}

func remoteInstallAgent(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteInstallResult, error) {
	switch normalizeRemoteInstallAgentID(agentID) {
	case "openclaw":
		return remoteInstallOpenClaw(ctx, host, hostID, agentID)
	case "picoclaw":
		return remoteInstallPicoClaw(ctx, host, hostID, agentID)
	case "zeroclaw":
		return remoteInstallZeroClaw(ctx, host, hostID, agentID)
	default:
		return nil, fmt.Errorf("unsupported remote install agent %q", strings.TrimSpace(agentID))
	}
}

func remoteInstallAgentStreaming(
	ctx context.Context,
	host RemoteHost,
	hostID, agentID string,
	onChunk func(remoteStreamChunk),
) (*remoteInstallResult, error) {
	switch normalizeRemoteInstallAgentID(agentID) {
	case "openclaw":
		return remoteInstallOpenClawStreaming(ctx, host, hostID, agentID, onChunk)
	case "picoclaw":
		return remoteInstallPicoClawStreaming(ctx, host, hostID, agentID, onChunk)
	case "zeroclaw":
		return remoteInstallZeroClawStreaming(ctx, host, hostID, agentID, onChunk)
	default:
		return nil, fmt.Errorf("unsupported remote install agent %q", strings.TrimSpace(agentID))
	}
}

func normalizeRemoteInstallAgentID(agentID string) string {
	trimmed := strings.ToLower(strings.TrimSpace(agentID))
	switch trimmed {
	case "", "main", "openclaw":
		return "openclaw"
	default:
		return trimmed
	}
}

func remoteInstallPicoClaw(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteInstallResult, error) {
	return remoteInstallBinaryRelease(ctx, host, hostID, agentID, "picoclaw", remotePicoClawInstallCommand(), nil)
}

func remoteInstallPicoClawStreaming(ctx context.Context, host RemoteHost, hostID, agentID string, onChunk func(remoteStreamChunk)) (*remoteInstallResult, error) {
	return remoteInstallBinaryRelease(ctx, host, hostID, agentID, "picoclaw", remotePicoClawInstallCommand(), onChunk)
}

func remoteInstallZeroClaw(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteInstallResult, error) {
	return remoteInstallBinaryRelease(ctx, host, hostID, agentID, "zeroclaw", remoteZeroClawInstallCommand(), nil)
}

func remoteInstallZeroClawStreaming(ctx context.Context, host RemoteHost, hostID, agentID string, onChunk func(remoteStreamChunk)) (*remoteInstallResult, error) {
	return remoteInstallBinaryRelease(ctx, host, hostID, agentID, "zeroclaw", remoteZeroClawInstallCommand(), onChunk)
}

func remoteInstallBinaryRelease(
	ctx context.Context,
	host RemoteHost,
	hostID, agentID string,
	action string,
	command string,
	onChunk func(remoteStreamChunk),
) (*remoteInstallResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, err
	}
	result := &remoteInstallResult{
		HostID:      hostID,
		AgentID:     agentID,
		Installed:   false,
		GatewayMode: host.RuntimeMode,
		Steps:       []remoteExecResult{},
	}
	var (
		res remoteExecResult
		err error
	)
	if onChunk != nil {
		res, err = runRemoteCommandStream(ctx, host, command, onChunk)
	} else {
		res, err = runRemoteCommand(ctx, host, command)
	}
	if err != nil {
		return result, err
	}
	result.Steps = append(result.Steps, res)
	if res.ExitCode != 0 {
		return result, remoteCommandError(res, "install "+action)
	}
	result.Installed = true
	return result, nil
}

func remotePicoClawInstallCommand() string {
	tag := remoteInstallReleaseTag("picoclaw", "v0.1.2")
	return fmt.Sprintf(
		"set -euo pipefail; arch=\"$(uname -m)\"; case \"$arch\" in x86_64|amd64) arch=\"x86_64\" ;; aarch64|arm64) arch=\"arm64\" ;; armv7l|armv6l) arch=\"armv6\" ;; riscv64) arch=\"riscv64\" ;; *) echo \"unsupported arch: $arch\" >&2; exit 2 ;; esac; tmp=\"$(mktemp -d)\"; trap 'rm -rf \"$tmp\"' EXIT; asset=\"picoclaw_Linux_${arch}.tar.gz\"; url=\"https://github.com/sipeed/picoclaw/releases/download/%s/${asset}\"; curl -fsSL \"$url\" -o \"$tmp/picoclaw.tar.gz\"; tar -xzf \"$tmp/picoclaw.tar.gz\" -C \"$tmp\"; bin=\"$(find \"$tmp\" -type f -name 'picoclaw*' -perm -u+x | head -n 1)\"; [ -n \"$bin\" ] || { echo \"picoclaw binary not found in release archive\" >&2; exit 3; }; mkdir -p \"$HOME/.local/bin\" \"$HOME/.picoclaw\"; install -m 0755 \"$bin\" \"$HOME/.local/bin/picoclaw\"; \"$HOME/.local/bin/picoclaw\" --version 2>&1 || true",
		tag,
	)
}

func remoteZeroClawInstallCommand() string {
	tag := remoteInstallReleaseTag("zeroclaw", "v0.1.7")
	return fmt.Sprintf(
		"set -euo pipefail; arch=\"$(uname -m)\"; case \"$arch\" in x86_64|amd64) target=\"x86_64-unknown-linux-gnu\" ;; aarch64|arm64) target=\"aarch64-unknown-linux-gnu\" ;; armv7l|armv6l) target=\"armv7-unknown-linux-gnueabihf\" ;; *) echo \"unsupported arch: $arch\" >&2; exit 2 ;; esac; tmp=\"$(mktemp -d)\"; trap 'rm -rf \"$tmp\"' EXIT; asset=\"zeroclaw-${target}.tar.gz\"; url=\"https://github.com/zeroclaw-labs/zeroclaw/releases/download/%s/${asset}\"; curl -fsSL \"$url\" -o \"$tmp/zeroclaw.tar.gz\"; tar -xzf \"$tmp/zeroclaw.tar.gz\" -C \"$tmp\"; bin=\"$(find \"$tmp\" -type f -name 'zeroclaw*' -perm -u+x | head -n 1)\"; [ -n \"$bin\" ] || { echo \"zeroclaw binary not found in release archive\" >&2; exit 3; }; mkdir -p \"$HOME/.local/bin\" \"$HOME/.zeroclaw\"; install -m 0755 \"$bin\" \"$HOME/.local/bin/zeroclaw\"; \"$HOME/.local/bin/zeroclaw\" --version 2>&1 || true",
		tag,
	)
}

func remoteInstallReleaseTag(agentID, fallback string) string {
	lock := loadManagedCompatLock()
	compat, ok := lock.Agents[strings.ToLower(strings.TrimSpace(agentID))]
	if !ok {
		return fallback
	}
	version := strings.TrimSpace(compat.RecommendedVersion)
	if version == "" {
		return fallback
	}
	if strings.HasPrefix(strings.ToLower(version), "v") {
		return version
	}
	return "v" + version
}

func remoteInstallOpenClaw(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteInstallResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, err
	}
	result := &remoteInstallResult{
		HostID:      hostID,
		AgentID:     agentID,
		Installed:   false,
		GatewayMode: host.RuntimeMode,
		Steps:       []remoteExecResult{},
	}
	installCmd := "curl -fsSL --proto '=https' --tlsv1.2 https://openclaw.ai/install.sh | bash -s -- --no-prompt --no-onboard 2>&1"
	installRes, err := runRemoteCommand(ctx, host, installCmd)
	if err != nil {
		return result, err
	}
	result.Steps = append(result.Steps, installRes)
	if installRes.ExitCode != 0 {
		return result, remoteCommandError(installRes, "install openclaw")
	}
	if host.RuntimeMode == RemoteRuntimeModeManagedGateway {
		restartRes, runErr := runRemoteCommand(ctx, host, "openclaw gateway restart 2>&1 || openclaw gateway start 2>&1")
		if runErr != nil {
			return result, runErr
		}
		result.Steps = append(result.Steps, restartRes)
		if restartRes.ExitCode != 0 {
			return result, remoteCommandError(restartRes, "start managed gateway")
		}
	}
	result.Installed = true
	return result, nil
}

func remoteInstallOpenClawStreaming(
	ctx context.Context,
	host RemoteHost,
	hostID, agentID string,
	onChunk func(remoteStreamChunk),
) (*remoteInstallResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, err
	}
	result := &remoteInstallResult{
		HostID:      hostID,
		AgentID:     agentID,
		Installed:   false,
		GatewayMode: host.RuntimeMode,
		Steps:       []remoteExecResult{},
	}

	installCmd := "curl -fsSL --proto '=https' --tlsv1.2 https://openclaw.ai/install.sh | bash -s -- --no-prompt --no-onboard 2>&1"
	installRes, err := runRemoteCommandStream(ctx, host, installCmd, onChunk)
	if err != nil {
		return result, err
	}
	result.Steps = append(result.Steps, installRes)
	if installRes.ExitCode != 0 {
		return result, remoteCommandError(installRes, "install openclaw")
	}
	if host.RuntimeMode == RemoteRuntimeModeManagedGateway {
		restartRes, runErr := runRemoteCommandStream(ctx, host, "openclaw gateway restart 2>&1 || openclaw gateway start 2>&1", onChunk)
		if runErr != nil {
			return result, runErr
		}
		result.Steps = append(result.Steps, restartRes)
		if restartRes.ExitCode != 0 {
			return result, remoteCommandError(restartRes, "start managed gateway")
		}
	}
	result.Installed = true
	return result, nil
}

func remoteListInstancesForHost(ctx context.Context, host RemoteHost, hostID string) ([]RemoteInstance, []remoteExecResult, error) {
	instances, _, steps, err := remoteDiscoverInstancesAndSyncProfiles(ctx, host, hostID, remoteDiscoveryPullOptions{})
	if err != nil {
		return nil, steps, err
	}
	if len(instances) == 0 {
		return []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}, steps, nil
	}
	return instances, steps, nil
}

func remoteDiscoverInstancesAndSyncProfiles(
	ctx context.Context,
	host RemoteHost,
	hostID string,
	opts remoteDiscoveryPullOptions,
) ([]RemoteInstance, []RemoteInstance, []remoteExecResult, error) {
	steps := []remoteExecResult{}
	instances := []RemoteInstance{}
	pendingPull := []RemoteInstance{}

	openClawInstalledRes, runErr := runRemoteCommand(ctx, host, "command -v openclaw >/dev/null 2>&1")
	if runErr != nil {
		return nil, nil, append(steps, openClawInstalledRes), runErr
	}
	steps = append(steps, openClawInstalledRes)
	if openClawInstalledRes.ExitCode == 0 {
		openClawInstances, openClawPendingPull, openClawSteps, err := remoteDiscoverOpenClawInstances(ctx, host, hostID, opts)
		steps = append(steps, openClawSteps...)
		if err != nil {
			return nil, nil, steps, err
		}
		instances = append(instances, openClawInstances...)
		pendingPull = append(pendingPull, openClawPendingPull...)
	}

	picoExists, picoSteps, err := remoteConfigFileExists(ctx, host, remotePicoClawConfigPath)
	steps = append(steps, picoSteps...)
	if err != nil {
		return nil, nil, steps, err
	}
	if picoExists {
		now := nowTimestamp()
		picoInst := RemoteInstance{
			ID:           hostID + ":picoclaw",
			HostID:       hostID,
			AgentID:      "picoclaw",
			RuntimeState: "unknown",
			Health:       "unknown",
			ConfigPath:   remotePicoClawConfigPath,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		instances = append(instances, picoInst)
		if remoteInstanceAlreadyTracked(hostID, "picoclaw") || opts.allowPullNewFor("picoclaw") {
			cfg, _, cfgSteps, cfgErr := remoteReadConfigForAgent(ctx, host, "picoclaw")
			steps = append(steps, cfgSteps...)
			if cfgErr != nil {
				return nil, nil, steps, cfgErr
			}
			if err := remoteSaveDiscoveredProfile(hostID, "picoclaw", cfg); err != nil {
				return nil, nil, steps, err
			}
		} else {
			pendingPull = append(pendingPull, picoInst)
		}
	}

	zeroExists, zeroSteps, err := remoteConfigFileExists(ctx, host, remoteZeroClawConfigPath)
	steps = append(steps, zeroSteps...)
	if err != nil {
		return nil, nil, steps, err
	}
	if zeroExists {
		now := nowTimestamp()
		zeroInst := RemoteInstance{
			ID:           hostID + ":zeroclaw",
			HostID:       hostID,
			AgentID:      "zeroclaw",
			RuntimeState: "unknown",
			Health:       "unknown",
			ConfigPath:   remoteZeroClawConfigPath,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		instances = append(instances, zeroInst)
		if remoteInstanceAlreadyTracked(hostID, "zeroclaw") || opts.allowPullNewFor("zeroclaw") {
			cfg, _, cfgSteps, cfgErr := remoteReadConfigForAgent(ctx, host, "zeroclaw")
			steps = append(steps, cfgSteps...)
			if cfgErr != nil {
				return nil, nil, steps, cfgErr
			}
			if err := remoteSaveDiscoveredProfile(hostID, "zeroclaw", cfg); err != nil {
				return nil, nil, steps, err
			}
		} else {
			pendingPull = append(pendingPull, zeroInst)
		}
	}

	return dedupeRemoteInstances(instances), dedupeRemoteInstances(pendingPull), steps, nil
}

func remoteDiscoverOpenClawInstances(
	ctx context.Context,
	host RemoteHost,
	hostID string,
	opts remoteDiscoveryPullOptions,
) ([]RemoteInstance, []RemoteInstance, []remoteExecResult, error) {
	steps := []remoteExecResult{}
	listRes, runErr := runRemoteCommand(ctx, host, "openclaw agents list --json 2>/dev/null")
	if runErr != nil {
		return nil, nil, append(steps, listRes), runErr
	}
	steps = append(steps, listRes)

	var instances []RemoteInstance
	if listRes.ExitCode != 0 {
		instances = []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}
	} else {
		instances = parseOpenClawInstances(hostID, listRes.Stdout)
		if len(instances) == 0 {
			instances = []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}
		}
	}

	toPull := make([]RemoteInstance, 0, len(instances))
	pendingPull := []RemoteInstance{}
	for _, inst := range instances {
		if remoteInstanceAlreadyTracked(hostID, inst.AgentID) || opts.allowPullNewFor(inst.AgentID) {
			toPull = append(toPull, inst)
			continue
		}
		pendingPull = append(pendingPull, inst)
	}

	if len(toPull) > 0 {
		cfg, _, cfgSteps, cfgErr := remoteReadConfigForAgent(ctx, host, "main")
		steps = append(steps, cfgSteps...)
		if cfgErr != nil {
			return nil, nil, steps, cfgErr
		}
		for _, inst := range toPull {
			if err := remoteSaveDiscoveredProfile(hostID, inst.AgentID, cfg); err != nil {
				return nil, nil, steps, err
			}
		}
	}
	return instances, dedupeRemoteInstances(pendingPull), steps, nil
}

func remoteConfigFileExists(ctx context.Context, host RemoteHost, path string) (bool, []remoteExecResult, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false, nil, nil
	}
	// Use double quotes (not %q) so shell variables like $HOME expand correctly.
	cmd := fmt.Sprintf(`if [ -f "%s" ]; then echo 1; else echo 0; fi`, trimmed)
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return false, []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return false, steps, remoteCommandError(res, "check remote config file")
	}
	out := strings.TrimSpace(strings.ToLower(res.Stdout))
	return out == "1" || out == "true" || out == "yes", steps, nil
}

func remoteSaveDiscoveredProfile(hostID, agentID string, config map[string]interface{}) error {
	instanceID := remoteInstanceProfileID(hostID, agentID)
	localCommit, _, err := profilesync.SaveInstanceProfile(instanceID, hostID, agentID, config, "remote-discovery-pull")
	if err != nil {
		return err
	}
	now := nowTimestamp()
	status := RemoteInstanceSyncStatus{
		HostID:              hostID,
		AgentID:             agentID,
		SyncMode:            providerBindingSyncModePullValidatePush,
		DriftState:          "in_sync",
		LastSyncStatus:      "success",
		LastSyncAt:          now,
		LastRemoteHash:      hashRemoteConfig(config),
		LastCanonicalConfig: config,
		LastLocalCommit:     localCommit,
		LastCommonCommit:    localCommit,
	}
	_, err = upsertRemoteInstanceSyncStatus(status)
	return err
}

func remoteInstanceAlreadyTracked(hostID, agentID string) bool {
	_, ok, err := getRemoteInstanceSyncStatus(hostID, agentID)
	return err == nil && ok
}

func dedupeRemoteInstances(instances []RemoteInstance) []RemoteInstance {
	if len(instances) <= 1 {
		return instances
	}
	seen := map[string]bool{}
	out := make([]RemoteInstance, 0, len(instances))
	for _, inst := range instances {
		key := strings.ToLower(strings.TrimSpace(inst.ID))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, inst)
	}
	return out
}

func parseOpenClawInstances(hostID, stdout string) []RemoteInstance {
	payload := strings.TrimSpace(stdout)
	if payload == "" {
		return nil
	}
	jsonPayload := extractJSONObjectOrArray(payload)
	if strings.TrimSpace(jsonPayload) == "" {
		return nil
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPayload), &entries); err != nil {
		return nil
	}
	if len(entries) == 0 {
		return nil
	}
	instances := make([]RemoteInstance, 0, len(entries))
	now := nowTimestamp()
	for _, item := range entries {
		agentID := strings.TrimSpace(anyToString(item["id"]))
		if agentID == "" {
			agentID = strings.TrimSpace(anyToString(item["agentId"]))
		}
		if agentID == "" {
			agentID = "main"
		}
		runtimeState := strings.TrimSpace(anyToString(item["runtimeState"]))
		if runtimeState == "" {
			runtimeState = strings.TrimSpace(anyToString(item["status"]))
		}
		if runtimeState == "" {
			runtimeState = "unknown"
		}
		health := strings.TrimSpace(anyToString(item["health"]))
		if health == "" {
			health = "unknown"
		}
		instances = append(instances, RemoteInstance{
			ID:           hostID + ":" + agentID,
			HostID:       hostID,
			AgentID:      agentID,
			RuntimeState: runtimeState,
			Health:       health,
			ConfigPath:   remoteOpenClawConfigPath,
			MemoryPath:   fmt.Sprintf("$HOME/.openclaw/agents/%s/memory", agentID),
			SessionPath:  fmt.Sprintf("$HOME/.openclaw/agents/%s/sessions", agentID),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	return instances
}

func remoteReadConfigForAgent(ctx context.Context, host RemoteHost, agentID string) (map[string]interface{}, string, []remoteExecResult, error) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "picoclaw":
		return remoteReadPicoClawConfig(ctx, host)
	case "zeroclaw":
		return remoteReadZeroClawConfig(ctx, host)
	default:
		return remoteReadConfig(ctx, host)
	}
}

func remotePatchConfigForAgent(ctx context.Context, host RemoteHost, agentID string, patch map[string]interface{}) (map[string]interface{}, string, []remoteExecResult, error) {
	switch strings.ToLower(strings.TrimSpace(agentID)) {
	case "picoclaw":
		return remotePatchPicoClawConfig(ctx, host, patch)
	case "zeroclaw":
		return remotePatchZeroClawConfig(ctx, host, patch)
	default:
		return remotePatchConfig(ctx, host, patch)
	}
}

func remoteGetInstanceStatus(ctx context.Context, host RemoteHost, hostID, agentID string) (RemoteInstance, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return RemoteInstance{}, nil, err
	}
	instances, steps, err := remoteListInstancesForHost(ctx, host, hostID)
	if err != nil {
		return RemoteInstance{}, steps, err
	}
	for _, inst := range instances {
		if strings.EqualFold(inst.AgentID, agentID) {
			return inst, steps, nil
		}
	}
	fallback := newDefaultRemoteInstance(hostID, agentID, "unknown")
	return fallback, steps, nil
}

func remoteSyncInstanceConfig(ctx context.Context, host RemoteHost, hostID, agentID, mode string) (*remoteSyncResult, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	mode = normalizeProviderBindingSyncMode(mode)
	if err := validateProviderBindingSyncMode(mode); err != nil {
		return nil, nil, err
	}
	config, _, steps, err := remoteReadConfigForAgent(ctx, host, agentID)
	if err != nil {
		return nil, steps, err
	}
	instanceID := remoteInstanceProfileID(hostID, agentID)
	localCommit, _, err := profilesync.SaveInstanceProfile(instanceID, hostID, agentID, config, "sync-pull")
	if err != nil {
		return nil, steps, err
	}
	now := nowTimestamp()
	status := RemoteInstanceSyncStatus{
		HostID:              hostID,
		AgentID:             agentID,
		SyncMode:            mode,
		DriftState:          "in_sync",
		LastSyncStatus:      "success",
		LastSyncAt:          now,
		LastRemoteHash:      hashRemoteConfig(config),
		LastCanonicalConfig: config,
		LastLocalCommit:     localCommit,
		LastCommonCommit:    localCommit,
	}
	if _, err := upsertRemoteInstanceSyncStatus(status); err != nil {
		return nil, steps, err
	}
	return &remoteSyncResult{
		HostID:         hostID,
		AgentID:        agentID,
		Mode:           mode,
		Status:         "in_sync",
		DriftState:     "in_sync",
		LastRemoteHash: status.LastRemoteHash,
		LastSyncAt:     now,
	}, steps, nil
}

func remoteGetInstanceSyncStatus(hostID, agentID string) (*RemoteInstanceSyncStatus, error) {
	status, ok, err := getRemoteInstanceSyncStatus(hostID, agentID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &RemoteInstanceSyncStatus{
			HostID:         hostID,
			AgentID:        agentID,
			SyncMode:       providerBindingSyncModeAlwaysPush,
			DriftState:     "unknown",
			LastSyncStatus: "not_synced",
			UpdatedAt:      nowTimestamp(),
		}, nil
	}
	return &status, nil
}

func remoteDiagnoseInstanceConfig(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteDiagnoseResult, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	config, _, steps, err := remoteReadConfigForAgent(ctx, host, agentID)
	if err != nil {
		return nil, steps, err
	}
	remoteHash := hashRemoteConfig(config)
	status, err := remoteGetInstanceSyncStatus(hostID, agentID)
	if err != nil {
		return nil, steps, err
	}
	result := "healthy"
	driftState := "in_sync"
	if strings.TrimSpace(status.LastRemoteHash) != "" && !strings.EqualFold(strings.TrimSpace(status.LastRemoteHash), remoteHash) {
		result = "drift_detected"
		driftState = "drift"
	}
	status.DriftState = driftState
	status.LastDiagnoseAt = nowTimestamp()
	status.LastDiagnoseResult = result
	status.LastRemoteHash = remoteHash
	if _, err := upsertRemoteInstanceSyncStatus(*status); err != nil {
		return nil, steps, err
	}
	return &remoteDiagnoseResult{
		HostID:         hostID,
		AgentID:        agentID,
		Result:         result,
		DriftState:     driftState,
		LastRemoteHash: remoteHash,
		LastDiagnoseAt: status.LastDiagnoseAt,
	}, steps, nil
}

func remoteReconcileInstanceConfig(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteReconcileResult, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	status, err := remoteGetInstanceSyncStatus(hostID, agentID)
	if err != nil {
		return nil, nil, err
	}
	if len(status.LastCanonicalConfig) == 0 {
		return nil, nil, fmt.Errorf("instance is not synced yet")
	}
	updatedConfig, _, steps, err := remotePatchConfigForAgent(ctx, host, agentID, status.LastCanonicalConfig)
	if err != nil {
		return nil, steps, err
	}
	instanceID := remoteInstanceProfileID(hostID, agentID)
	localCommit, _, err := profilesync.SaveInstanceProfile(instanceID, hostID, agentID, status.LastCanonicalConfig, "diagnose-reconcile")
	if err != nil {
		return nil, steps, err
	}
	now := nowTimestamp()
	status.DriftState = "in_sync"
	status.LastSyncStatus = "success"
	status.LastReconcileAt = now
	status.LastSyncAt = now
	status.LastRemoteHash = hashRemoteConfig(updatedConfig)
	status.LastLocalCommit = localCommit
	status.LastCommonCommit = localCommit
	if _, err := upsertRemoteInstanceSyncStatus(*status); err != nil {
		return nil, steps, err
	}
	return &remoteReconcileResult{
		HostID:          hostID,
		AgentID:         agentID,
		Reconciled:      true,
		DriftState:      "in_sync",
		LastRemoteHash:  status.LastRemoteHash,
		LastReconcileAt: now,
	}, steps, nil
}

func remoteRollbackInstanceConfig(ctx context.Context, host RemoteHost, hostID, agentID, commit string) (*remoteRollbackResult, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	status, err := remoteGetInstanceSyncStatus(hostID, agentID)
	if err != nil {
		return nil, nil, err
	}
	targetCommit := strings.TrimSpace(commit)
	if targetCommit == "" {
		targetCommit = strings.TrimSpace(status.LastCommonCommit)
	}
	if targetCommit == "" {
		targetCommit = strings.TrimSpace(status.LastLocalCommit)
	}
	if targetCommit == "" {
		return nil, nil, fmt.Errorf("rollback commit is required")
	}

	instanceID := remoteInstanceProfileID(hostID, agentID)
	profileAtCommit, err := profilesync.LoadInstanceProfileAtCommit(instanceID, targetCommit)
	if err != nil {
		return nil, nil, err
	}
	updatedConfig, _, steps, err := remotePatchConfigForAgent(ctx, host, agentID, profileAtCommit)
	if err != nil {
		return nil, steps, err
	}
	newCommit, _, err := profilesync.SaveInstanceProfile(instanceID, hostID, agentID, profileAtCommit, "rollback")
	if err != nil {
		return nil, steps, err
	}
	rollbackAt := nowTimestamp()
	status.DriftState = "in_sync"
	status.LastSyncStatus = "success"
	status.LastRollbackAt = rollbackAt
	status.LastSyncAt = rollbackAt
	status.LastRemoteHash = hashRemoteConfig(updatedConfig)
	status.LastCanonicalConfig = profileAtCommit
	status.LastLocalCommit = newCommit
	status.LastCommonCommit = newCommit
	if _, err := upsertRemoteInstanceSyncStatus(*status); err != nil {
		return nil, steps, err
	}
	return &remoteRollbackResult{
		HostID:           hostID,
		AgentID:          agentID,
		RolledBack:       true,
		FromCommit:       targetCommit,
		NewCommit:        newCommit,
		DriftState:       "in_sync",
		LastRemoteHash:   status.LastRemoteHash,
		LastRollbackAt:   rollbackAt,
		RestoredSnapshot: true,
	}, steps, nil
}

func remoteRepairOpenClaw(ctx context.Context, host RemoteHost, hostID, agentID string) (*remoteRepairResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, err
	}
	result := &remoteRepairResult{
		HostID:  hostID,
		AgentID: agentID,
		Steps:   []remoteExecResult{},
	}
	healthy, repaired, steps, err := ensureRemoteHealthyForOperation(ctx, host)
	result.Steps = append(result.Steps, steps...)
	result.GatewayHealthy = healthy
	result.Repaired = repaired
	if err != nil {
		logTail, tailErr := remoteTailGatewayLogs(ctx, host, 120)
		if tailErr == nil {
			result.LogTail = logTail
		}
		return result, err
	}
	if !healthy {
		logTail, tailErr := remoteTailGatewayLogs(ctx, host, 120)
		if tailErr == nil {
			result.LogTail = logTail
		}
		return result, fmt.Errorf("gateway remains unhealthy after repair")
	}
	logTail, tailErr := remoteTailGatewayLogs(ctx, host, 80)
	if tailErr == nil {
		result.LogTail = logTail
	}
	return result, nil
}

func remoteTailGatewayLogs(ctx context.Context, host RemoteHost, tail int) (string, error) {
	if tail <= 0 {
		tail = 80
	}
	cmd := fmt.Sprintf("tail -n %d \"$HOME/.openclaw/logs/gateway.log\" 2>/dev/null || true", tail)
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", nil
	}
	return strings.TrimSpace(res.Stdout), nil
}

func remoteGetLogs(ctx context.Context, host RemoteHost, agentID string, tail int) (string, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return "", nil, err
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > 2000 {
		tail = 2000
	}
	cmd := fmt.Sprintf("set -e; g=\"$HOME/.openclaw/logs/gateway.log\"; if [ -f \"$g\" ]; then echo \"--- gateway.log ---\"; tail -n %d \"$g\"; fi; base=\"$HOME/.openclaw/agents/%s/logs\"; if [ -d \"$base\" ]; then for f in \"$base\"/*.log; do [ -f \"$f\" ] || continue; echo \"--- $(basename \"$f\") ---\"; tail -n %d \"$f\"; done; fi", tail, agentID, tail)
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return "", []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return "", steps, remoteCommandError(res, "fetch remote logs")
	}
	return strings.TrimSpace(res.Stdout), steps, nil
}

func remoteReadConfig(ctx context.Context, host RemoteHost) (map[string]interface{}, string, []remoteExecResult, error) {
	cmd := "if [ -f \"$HOME/.openclaw/openclaw.json\" ]; then cat \"$HOME/.openclaw/openclaw.json\"; else echo '{}'; fi"
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return nil, "", []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return nil, "", steps, remoteCommandError(res, "read remote config")
	}
	text := strings.TrimSpace(res.Stdout)
	if text == "" {
		return map[string]interface{}{}, "{}", steps, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return openClawCanonicalConfig(text), text, steps, nil
	}
	return out, text, steps, nil
}

func remoteReadPicoClawConfig(ctx context.Context, host RemoteHost) (map[string]interface{}, string, []remoteExecResult, error) {
	cmd := "if [ -f \"$HOME/.picoclaw/config.json\" ]; then cat \"$HOME/.picoclaw/config.json\"; else echo '{}'; fi"
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return nil, "", []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return nil, "", steps, remoteCommandError(res, "read picoclaw config")
	}
	text := strings.TrimSpace(res.Stdout)
	if text == "" {
		return map[string]interface{}{}, "{}", steps, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, text, steps, fmt.Errorf("parse picoclaw config json: %w", err)
	}
	return out, text, steps, nil
}

func remoteReadZeroClawConfig(ctx context.Context, host RemoteHost) (map[string]interface{}, string, []remoteExecResult, error) {
	cmd := "if [ -f \"$HOME/.zeroclaw/config.toml\" ]; then cat \"$HOME/.zeroclaw/config.toml\"; else echo ''; fi"
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return nil, "", []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return nil, "", steps, remoteCommandError(res, "read zeroclaw config")
	}
	text := strings.TrimSpace(res.Stdout)
	return zeroClawCanonicalConfig(text), text, steps, nil
}

func zeroClawCanonicalConfig(raw string) map[string]interface{} {
	return map[string]interface{}{
		"_carrier_meta": map[string]interface{}{
			"runtime":    "zeroclaw",
			"format":     "toml",
			"configPath": remoteZeroClawConfigPath,
		},
		"raw_toml": strings.TrimSpace(raw),
	}
}

func openClawCanonicalConfig(raw string) map[string]interface{} {
	return map[string]interface{}{
		"_carrier_meta": map[string]interface{}{
			"runtime":    "openclaw",
			"format":     "json5",
			"configPath": remoteOpenClawConfigPath,
		},
		"raw_json5": strings.TrimSpace(raw),
	}
}

func openClawRawFromCanonical(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	return strings.TrimSpace(anyToString(payload["raw_json5"]))
}

func isOpenClawCanonical(payload map[string]interface{}) bool {
	return strings.TrimSpace(openClawRawFromCanonical(payload)) != ""
}

func zeroClawRawFromCanonical(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	return strings.TrimSpace(anyToString(payload["raw_toml"]))
}

func remotePatchConfig(ctx context.Context, host RemoteHost, patch map[string]interface{}) (map[string]interface{}, string, []remoteExecResult, error) {
	rawPatch := openClawRawFromCanonical(patch)
	sanitizedPatch, secretsPatch := sanitizeOpenClawPatchPayload(patch)
	steps := []remoteExecResult{}

	if len(secretsPatch) > 0 {
		secretWriteRes, secretErr := remoteWriteOpenClawCarrierSecrets(ctx, host, secretsPatch)
		steps = append(steps, secretWriteRes)
		if secretErr != nil {
			return nil, "", steps, secretErr
		}
		if secretWriteRes.ExitCode != 0 {
			return nil, "", steps, remoteCommandError(secretWriteRes, "write openclaw carrier secrets")
		}
	}

	if strings.TrimSpace(rawPatch) != "" {
		snapshotPath, writeRes, writeErr := remoteWriteOpenClawRawConfig(ctx, host, rawPatch)
		steps = append(steps, writeRes)
		if writeErr != nil {
			return nil, snapshotPath, steps, writeErr
		}
		if writeRes.ExitCode != 0 {
			return nil, snapshotPath, steps, remoteCommandError(writeRes, "write remote config")
		}
		return openClawCanonicalConfig(rawPatch), snapshotPath, steps, nil
	}

	current, _, readSteps, err := remoteReadConfig(ctx, host)
	steps = append(steps, readSteps...)
	if err != nil {
		return nil, "", steps, err
	}
	if len(sanitizedPatch) == 0 {
		return current, "", steps, nil
	}
	if isOpenClawCanonical(current) {
		updated, snapshotPath, setSteps, setErr := remotePatchOpenClawViaConfigSet(ctx, host, sanitizedPatch)
		steps = append(steps, setSteps...)
		if setErr != nil {
			return nil, snapshotPath, steps, setErr
		}
		return updated, snapshotPath, steps, nil
	}

	merged := deepMergeJSON(current, sanitizedPatch)
	raw, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, "", steps, fmt.Errorf("marshal merged remote config: %w", err)
	}
	snapshotPath, writeRes, runErr := remoteWriteOpenClawRawConfig(ctx, host, string(raw))
	steps = append(steps, writeRes)
	if runErr != nil {
		return nil, snapshotPath, steps, runErr
	}
	if writeRes.ExitCode != 0 {
		return nil, snapshotPath, steps, remoteCommandError(writeRes, "write remote config")
	}
	return merged, snapshotPath, steps, nil
}

type openClawConfigMutation struct {
	Path    string
	Value   string
	IsUnset bool
}

func sanitizeOpenClawPatchPayload(patch map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	clean := map[string]interface{}{}
	var secretsPatch map[string]interface{}
	for key, value := range patch {
		trimmed := strings.TrimSpace(key)
		switch trimmed {
		case openclawcfg.CarrierSecretFilePatchKey:
			if typed, ok := value.(map[string]interface{}); ok {
				secretsPatch = typed
			}
		case "_carrier_meta", "raw_json5":
			// internal canonical metadata, never write as config keys
			continue
		default:
			clean[key] = value
		}
	}
	return clean, secretsPatch
}

func remotePatchOpenClawViaConfigSet(ctx context.Context, host RemoteHost, patch map[string]interface{}) (map[string]interface{}, string, []remoteExecResult, error) {
	ops := make([]openClawConfigMutation, 0)
	flattenOpenClawConfigMutations("", patch, &ops)
	if len(ops) == 0 {
		cfg, _, steps, err := remoteReadConfig(ctx, host)
		return cfg, "", steps, err
	}
	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return !ops[i].IsUnset && ops[j].IsUnset
		}
		return ops[i].Path < ops[j].Path
	})

	snapshotUnix := time.Now().Unix()
	snapshotPath := fmt.Sprintf("$HOME/.openclaw/snapshots/openclaw-%d.json", snapshotUnix)
	cmd := buildRemoteOpenClawConfigSetCommand(snapshotUnix, ops)
	writeRes, runErr := runRemoteCommand(ctx, host, cmd)
	steps := []remoteExecResult{writeRes}
	if runErr != nil {
		return nil, snapshotPath, steps, runErr
	}
	if writeRes.ExitCode != 0 {
		return nil, snapshotPath, steps, remoteCommandError(writeRes, "apply openclaw config patch via config set")
	}
	updated, _, readSteps, readErr := remoteReadConfig(ctx, host)
	steps = append(steps, readSteps...)
	if readErr != nil {
		return nil, snapshotPath, steps, readErr
	}
	return updated, snapshotPath, steps, nil
}

func flattenOpenClawConfigMutations(prefix string, value interface{}, out *[]openClawConfigMutation) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if prefix != "" && len(typed) == 0 {
			*out = append(*out, openClawConfigMutation{
				Path:  prefix,
				Value: "{}",
			})
			return
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			next := trimmed
			if prefix != "" {
				next = prefix + "." + trimmed
			}
			flattenOpenClawConfigMutations(next, typed[key], out)
		}
	case nil:
		if prefix == "" {
			return
		}
		*out = append(*out, openClawConfigMutation{
			Path:    prefix,
			IsUnset: true,
		})
	default:
		if prefix == "" {
			return
		}
		raw, err := json.Marshal(typed)
		if err != nil {
			return
		}
		*out = append(*out, openClawConfigMutation{
			Path:  prefix,
			Value: string(raw),
		})
	}
}

func buildRemoteOpenClawConfigSetCommand(snapshotUnix int64, ops []openClawConfigMutation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "set -e\nmkdir -p \"$HOME/.openclaw\" \"$HOME/.openclaw/snapshots\"\nsnapshot_path=\"$HOME/.openclaw/snapshots/openclaw-%d.json\"\ncp \"$HOME/.openclaw/openclaw.json\" \"$snapshot_path\" 2>/dev/null || true\n", snapshotUnix)
	seed := time.Now().UnixNano()
	for idx, op := range ops {
		if strings.TrimSpace(op.Path) == "" {
			continue
		}
		if op.IsUnset {
			fmt.Fprintf(&b, "openclaw config unset %s >/dev/null\n", shellSingleQuote(op.Path))
			continue
		}
		delimiter := fmt.Sprintf("CARRIER_CFG_%d_%d", seed, idx)
		fmt.Fprintf(&b, "openclaw config set %s \"$(cat <<'%s'\n%s\n%s\n)\" --strict-json >/dev/null\n", shellSingleQuote(op.Path), delimiter, op.Value, delimiter)
	}
	return b.String()
}

func remoteWriteOpenClawRawConfig(ctx context.Context, host RemoteHost, raw string) (string, remoteExecResult, error) {
	delimiter := fmt.Sprintf("CARRIER_EOF_%d", time.Now().UnixNano())
	snapshotUnix := time.Now().Unix()
	snapshotPath := fmt.Sprintf("$HOME/.openclaw/snapshots/openclaw-%d.json", snapshotUnix)
	writeCmd := fmt.Sprintf(
		"mkdir -p \"$HOME/.openclaw\" \"$HOME/.openclaw/snapshots\"; snapshot_path=\"$HOME/.openclaw/snapshots/openclaw-%d.json\"; cp \"$HOME/.openclaw/openclaw.json\" \"$snapshot_path\" 2>/dev/null || true; cat > \"$HOME/.openclaw/openclaw.json\" <<'%s'\n%s\n%s",
		snapshotUnix,
		delimiter,
		raw,
		delimiter,
	)
	writeRes, runErr := runRemoteCommand(ctx, host, writeCmd)
	if runErr != nil {
		return snapshotPath, writeRes, runErr
	}
	return snapshotPath, writeRes, nil
}

func remoteWriteOpenClawCarrierSecrets(ctx context.Context, host RemoteHost, payload map[string]interface{}) (remoteExecResult, error) {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return remoteExecResult{}, fmt.Errorf("marshal openclaw carrier secrets: %w", err)
	}
	delimiter := fmt.Sprintf("CARRIER_SECRETS_%d", time.Now().UnixNano())
	cmd := fmt.Sprintf(
		"mkdir -p \"$HOME/.openclaw\"; cat > \"%s\" <<'%s'\n%s\n%s\nchmod 600 \"%s\"",
		remoteOpenClawCarrierSecretsPath,
		delimiter,
		string(raw),
		delimiter,
		remoteOpenClawCarrierSecretsPath,
	)
	res, runErr := runRemoteCommand(ctx, host, cmd)
	if runErr != nil {
		return res, runErr
	}
	return res, nil
}

func remotePatchPicoClawConfig(ctx context.Context, host RemoteHost, patch map[string]interface{}) (map[string]interface{}, string, []remoteExecResult, error) {
	current, currentRaw, steps, err := remoteReadPicoClawConfig(ctx, host)
	if err != nil {
		return nil, "", steps, err
	}
	merged := deepMergeJSON(current, patch)
	raw, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, "", steps, fmt.Errorf("marshal merged picoclaw config: %w", err)
	}
	delimiter := fmt.Sprintf("CARRIER_EOF_%d", time.Now().UnixNano())
	snapshotUnix := time.Now().Unix()
	snapshotPath := fmt.Sprintf("$HOME/.picoclaw/snapshots/picoclaw-%d.json", snapshotUnix)
	writeCmd := fmt.Sprintf(
		"mkdir -p \"$HOME/.picoclaw\" \"$HOME/.picoclaw/snapshots\"; snapshot_path=\"$HOME/.picoclaw/snapshots/picoclaw-%d.json\"; cp \"$HOME/.picoclaw/config.json\" \"$snapshot_path\" 2>/dev/null || true; cat > \"$HOME/.picoclaw/config.json\" <<'%s'\n%s\n%s",
		snapshotUnix,
		delimiter,
		string(raw),
		delimiter,
	)
	writeRes, runErr := runRemoteCommand(ctx, host, writeCmd)
	steps = append(steps, writeRes)
	if runErr != nil {
		return nil, snapshotPath, steps, runErr
	}
	if writeRes.ExitCode != 0 {
		_ = currentRaw
		return nil, snapshotPath, steps, remoteCommandError(writeRes, "write picoclaw config")
	}
	return merged, snapshotPath, steps, nil
}

func remotePatchZeroClawConfig(ctx context.Context, host RemoteHost, patch map[string]interface{}) (map[string]interface{}, string, []remoteExecResult, error) {
	rawToml := zeroClawRawFromCanonical(patch)
	if rawToml == "" {
		return nil, "", nil, fmt.Errorf("zeroclaw patch requires raw_toml")
	}
	readCmd := "if [ -f \"$HOME/.zeroclaw/config.toml\" ]; then cat \"$HOME/.zeroclaw/config.toml\"; else echo ''; fi"
	readRes, runErr := runRemoteCommand(ctx, host, readCmd)
	if runErr != nil {
		return nil, "", []remoteExecResult{readRes}, runErr
	}
	steps := []remoteExecResult{readRes}
	if readRes.ExitCode != 0 {
		return nil, "", steps, remoteCommandError(readRes, "read zeroclaw config before patch")
	}

	delimiter := fmt.Sprintf("CARRIER_EOF_%d", time.Now().UnixNano())
	snapshotUnix := time.Now().Unix()
	snapshotPath := fmt.Sprintf("$HOME/.zeroclaw/snapshots/zeroclaw-%d.toml", snapshotUnix)
	writeCmd := fmt.Sprintf(
		"mkdir -p \"$HOME/.zeroclaw\" \"$HOME/.zeroclaw/snapshots\"; snapshot_path=\"$HOME/.zeroclaw/snapshots/zeroclaw-%d.toml\"; cp \"$HOME/.zeroclaw/config.toml\" \"$snapshot_path\" 2>/dev/null || true; cat > \"$HOME/.zeroclaw/config.toml\" <<'%s'\n%s\n%s",
		snapshotUnix,
		delimiter,
		rawToml,
		delimiter,
	)
	writeRes, writeErr := runRemoteCommand(ctx, host, writeCmd)
	steps = append(steps, writeRes)
	if writeErr != nil {
		return nil, snapshotPath, steps, writeErr
	}
	if writeRes.ExitCode != 0 {
		return nil, snapshotPath, steps, remoteCommandError(writeRes, "write zeroclaw config")
	}
	return zeroClawCanonicalConfig(rawToml), snapshotPath, steps, nil
}

func deepMergeJSON(base map[string]interface{}, patch map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, pv := range patch {
		if pv == nil {
			delete(result, k)
			continue
		}
		patchMap, patchIsMap := pv.(map[string]interface{})
		if !patchIsMap {
			result[k] = pv
			continue
		}
		existingMap, existingIsMap := result[k].(map[string]interface{})
		if !existingIsMap {
			existingMap = map[string]interface{}{}
		}
		result[k] = deepMergeJSON(existingMap, patchMap)
	}
	return result
}

func hashRemoteConfig(payload map[string]interface{}) string {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func remoteInstanceProfileID(hostID, agentID string) string {
	host := strings.TrimSpace(hostID)
	agent := strings.TrimSpace(agentID)
	if host == "" {
		host = "unknown-host"
	}
	if agent == "" {
		agent = "main"
	}
	return host + "_" + agent
}

func remoteListSessions(ctx context.Context, host RemoteHost, agentID string) ([]RemoteSessionEntry, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	script := fmt.Sprintf("set -e; base=\"$HOME/.openclaw/agents/%s\"; for kind in sessions sessions_archive; do dir=\"$base/$kind\"; [ -d \"$dir\" ] || continue; find \"$dir\" -maxdepth 1 -type f -name '*.jsonl' | while read -r f; do sid=$(basename \"$f\" .jsonl); sz=$(wc -c < \"$f\" | tr -d ' '); mt=$(stat -c %%Y \"$f\" 2>/dev/null || stat -f %%m \"$f\" 2>/dev/null || echo 0); printf '%%s\t%%s\t%%s\t%%s\t%%s\\n' \"$sid\" \"$kind\" \"$sz\" \"$mt\" \"$f\"; done; done", agentID)
	res, err := runRemoteCommand(ctx, host, script)
	if err != nil {
		return nil, []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return nil, steps, remoteCommandError(res, "list remote sessions")
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	entries := make([]RemoteSessionEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		sz, _ := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		mt, _ := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		entries = append(entries, RemoteSessionEntry{
			SessionID:  strings.TrimSpace(parts[0]),
			Kind:       strings.TrimSpace(parts[1]),
			SizeBytes:  sz,
			ModifiedAt: mt,
			Path:       strings.TrimSpace(parts[4]),
		})
	}
	return entries, steps, nil
}

func remoteArchiveSession(ctx context.Context, host RemoteHost, agentID, sessionID string) ([]remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, err
	}
	trimmedSession, err := validateRemoteSessionIdentifier(sessionID)
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf("set -e; sid=%s; base=\"$HOME/.openclaw/agents/%s\"; src=\"$base/sessions/$sid.jsonl\"; dst=\"$base/sessions_archive/$sid.jsonl\"; if [ -f \"$src\" ]; then mkdir -p \"$base/sessions_archive\"; mv \"$src\" \"$dst\"; exit 0; fi; if [ -f \"$dst\" ]; then exit 0; fi; exit 44", shellSingleQuote(trimmedSession), agentID)
	res, err := runRemoteCommand(ctx, host, cmd)
	steps := []remoteExecResult{res}
	if err != nil {
		return steps, err
	}
	if res.ExitCode == 44 {
		return steps, fmt.Errorf("session %s not found", trimmedSession)
	}
	if res.ExitCode != 0 {
		return steps, remoteCommandError(res, "archive session")
	}
	return steps, nil
}

func remoteDeleteSession(ctx context.Context, host RemoteHost, agentID, sessionID string) ([]remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, err
	}
	trimmedSession, err := validateRemoteSessionIdentifier(sessionID)
	if err != nil {
		return nil, err
	}
	cmd := fmt.Sprintf("set -e; sid=%s; base=\"$HOME/.openclaw/agents/%s\"; rm -f \"$base/sessions/$sid.jsonl\" \"$base/sessions_archive/$sid.jsonl\"", shellSingleQuote(trimmedSession), agentID)
	res, err := runRemoteCommand(ctx, host, cmd)
	steps := []remoteExecResult{res}
	if err != nil {
		return steps, err
	}
	if res.ExitCode != 0 {
		return steps, remoteCommandError(res, "delete session")
	}
	return steps, nil
}

func remoteListMemory(ctx context.Context, host RemoteHost, agentID string) ([]RemoteMemoryEntry, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	cmd := fmt.Sprintf("set -e; base=\"$HOME/.openclaw/agents/%s/memory\"; [ -d \"$base\" ] || exit 0; find \"$base\" -type f | head -n 200 | while read -r f; do rel=\"${f#$base/}\"; sz=$(wc -c < \"$f\" | tr -d ' '); mt=$(stat -c %%Y \"$f\" 2>/dev/null || stat -f %%m \"$f\" 2>/dev/null || echo 0); printf '%%s\t%%s\t%%s\\n' \"$rel\" \"$sz\" \"$mt\"; done", agentID)
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return nil, []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return nil, steps, remoteCommandError(res, "list memory files")
	}
	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	entries := make([]RemoteMemoryEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		sz, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		mt, _ := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		entries = append(entries, RemoteMemoryEntry{
			Path:       strings.TrimSpace(parts[0]),
			SizeBytes:  sz,
			ModifiedAt: mt,
		})
	}
	return entries, steps, nil
}

func remoteChatViaOpenClaw(ctx context.Context, host RemoteHost, agentID, message, sessionID string) (map[string]interface{}, []remoteExecResult, error) {
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(message) == "" {
		return nil, nil, fmt.Errorf("message is required")
	}
	cmd := fmt.Sprintf("openclaw agent --local --agent %s --message %s --json --no-color", shellSingleQuote(agentID), shellSingleQuote(strings.TrimSpace(message)))
	if strings.TrimSpace(sessionID) != "" {
		cmd += " --session-id " + shellSingleQuote(strings.TrimSpace(sessionID))
	}
	res, err := runRemoteCommand(ctx, host, cmd)
	if err != nil {
		return nil, []remoteExecResult{res}, err
	}
	steps := []remoteExecResult{res}
	if res.ExitCode != 0 {
		return nil, steps, remoteCommandError(res, "remote chat")
	}
	payload := strings.TrimSpace(extractJSONObjectOrArray(res.Stdout))
	if payload == "" {
		return map[string]interface{}{"message": strings.TrimSpace(res.Stdout)}, steps, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return map[string]interface{}{"message": strings.TrimSpace(res.Stdout)}, steps, nil
	}
	return out, steps, nil
}

func applyProviderProfileToRemote(ctx context.Context, host RemoteHost, profile ProviderProfile, agentID string) (map[string]interface{}, string, []remoteExecResult, error) {
	providerKey := mapCarrierProviderToManagedProvider(profile.Provider)
	patch := map[string]interface{}{}
	model := strings.TrimSpace(profile.Model)
	if model != "" {
		setNestedMapValue(patch, []string{"agents", "defaults", "model", "primary"}, model)
		if trimmedAgentID := strings.TrimSpace(agentID); trimmedAgentID != "" && !strings.EqualFold(trimmedAgentID, "main") {
			setNestedMapValue(patch, []string{"agents", "overrides", trimmedAgentID, "model", "primary"}, model)
		}
	}
	if providerKey != "" && strings.TrimSpace(profile.BaseURL) != "" {
		setNestedMapValue(patch, []string{"models", "providers", providerKey, "baseUrl"}, strings.TrimSpace(profile.BaseURL))
	}
	if providerKey != "" && strings.TrimSpace(profile.AuthRef) != "" {
		setNestedMapValue(patch, []string{"models", "providers", providerKey, "apiKey"}, strings.TrimSpace(profile.AuthRef))
	}
	return remotePatchConfig(ctx, host, patch)
}

func setNestedMapValue(root map[string]interface{}, path []string, value interface{}) {
	if len(path) == 0 {
		return
	}
	current := root
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		next, ok := current[key].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}

func extractJSONObjectOrArray(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	firstObject := strings.Index(trimmed, "{")
	firstArray := strings.Index(trimmed, "[")
	start := -1
	if firstObject >= 0 && firstArray >= 0 {
		if firstObject < firstArray {
			start = firstObject
		} else {
			start = firstArray
		}
	} else if firstObject >= 0 {
		start = firstObject
	} else if firstArray >= 0 {
		start = firstArray
	}
	if start < 0 {
		return ""
	}
	candidate := strings.TrimSpace(trimmed[start:])
	if json.Valid([]byte(candidate)) {
		return candidate
	}
	lastObject := strings.LastIndex(candidate, "}")
	lastArray := strings.LastIndex(candidate, "]")
	end := lastObject
	if lastArray > end {
		end = lastArray
	}
	if end >= 0 {
		maybe := strings.TrimSpace(candidate[:end+1])
		if json.Valid([]byte(maybe)) {
			return maybe
		}
	}
	return ""
}

func anyToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func newDefaultRemoteInstance(hostID, agentID, runtimeState string) RemoteInstance {
	now := nowTimestamp()
	if strings.TrimSpace(runtimeState) == "" {
		runtimeState = "unknown"
	}
	return RemoteInstance{
		ID:           hostID + ":" + agentID,
		HostID:       hostID,
		AgentID:      agentID,
		RuntimeState: runtimeState,
		Health:       "unknown",
		ConfigPath:   remoteOpenClawConfigPath,
		MemoryPath:   fmt.Sprintf("$HOME/.openclaw/agents/%s/memory", agentID),
		SessionPath:  fmt.Sprintf("$HOME/.openclaw/agents/%s/sessions", agentID),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func extractChatResponseText(payload map[string]interface{}) string {
	for _, key := range []string{"message", "response", "text", "content", "output_text"} {
		if value := extractChatTextFromAny(payload[key]); value != "" {
			return value
		}
	}
	for _, key := range []string{"output", "payload", "payloads", "choices", "result", "data"} {
		if value := extractChatTextFromAny(payload[key]); value != "" {
			return value
		}
	}
	return ""
}

func extractChatTextFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if segment := extractChatTextFromAny(item); segment != "" {
				parts = append(parts, segment)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]interface{}:
		for _, key := range []string{"message", "response", "text", "content", "output_text"} {
			if segment := extractChatTextFromAny(typed[key]); segment != "" {
				return segment
			}
		}
		for _, key := range []string{"delta", "payload", "payloads", "choices", "output", "result", "data"} {
			if segment := extractChatTextFromAny(typed[key]); segment != "" {
				return segment
			}
		}
	}
	return ""
}
