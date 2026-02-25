package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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

func remoteListInstancesForHost(ctx context.Context, host RemoteHost, hostID string) ([]RemoteInstance, []remoteExecResult, error) {
	steps := []remoteExecResult{}
	_, _, preflightSteps, err := ensureRemoteHealthyForOperation(ctx, host)
	steps = append(steps, preflightSteps...)
	if err != nil {
		return nil, steps, err
	}

	listRes, runErr := runRemoteCommand(ctx, host, "openclaw agents list --json 2>/dev/null")
	if runErr != nil {
		return nil, steps, runErr
	}
	steps = append(steps, listRes)
	if listRes.ExitCode != 0 {
		fallback := []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}
		return fallback, steps, nil
	}
	payload := strings.TrimSpace(listRes.Stdout)
	if payload == "" {
		return []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}, steps, nil
	}
	jsonPayload := extractJSONObjectOrArray(payload)
	if strings.TrimSpace(jsonPayload) == "" {
		return []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}, steps, nil
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPayload), &entries); err != nil {
		return []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}, steps, nil
	}
	if len(entries) == 0 {
		return []RemoteInstance{newDefaultRemoteInstance(hostID, "main", "unknown")}, steps, nil
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
			ConfigPath:   "$HOME/.openclaw/openclaw.json",
			MemoryPath:   fmt.Sprintf("$HOME/.openclaw/agents/%s/memory", agentID),
			SessionPath:  fmt.Sprintf("$HOME/.openclaw/agents/%s/sessions", agentID),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	return instances, steps, nil
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
	cmd := "cat \"$HOME/.openclaw/openclaw.json\""
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
		return nil, text, steps, fmt.Errorf("parse remote config json: %w", err)
	}
	return out, text, steps, nil
}

func remotePatchConfig(ctx context.Context, host RemoteHost, patch map[string]interface{}) (map[string]interface{}, string, []remoteExecResult, error) {
	current, currentRaw, steps, err := remoteReadConfig(ctx, host)
	if err != nil {
		return nil, "", steps, err
	}
	merged := deepMergeJSON(current, patch)
	raw, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, "", steps, fmt.Errorf("marshal merged remote config: %w", err)
	}
	delimiter := fmt.Sprintf("CARRIER_EOF_%d", time.Now().UnixNano())
	snapshotUnix := time.Now().Unix()
	snapshotPath := fmt.Sprintf("$HOME/.openclaw/snapshots/openclaw-%d.json", snapshotUnix)
	writeCmd := fmt.Sprintf(
		"mkdir -p \"$HOME/.openclaw\" \"$HOME/.openclaw/snapshots\"; snapshot_path=\"$HOME/.openclaw/snapshots/openclaw-%d.json\"; cp \"$HOME/.openclaw/openclaw.json\" \"$snapshot_path\" 2>/dev/null || true; cat > \"$HOME/.openclaw/openclaw.json\" <<'%s'\n%s\n%s",
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
		return nil, snapshotPath, steps, remoteCommandError(writeRes, "write remote config")
	}
	return merged, snapshotPath, steps, nil
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
	if providerKey != "" {
		setNestedMapValue(patch, []string{"agents", "defaults", "provider"}, providerKey)
	}
	if strings.TrimSpace(profile.Model) != "" {
		setNestedMapValue(patch, []string{"agents", "defaults", "model"}, strings.TrimSpace(profile.Model))
	}
	if strings.TrimSpace(agentID) != "" {
		setNestedMapValue(patch, []string{"agents", "overrides", strings.TrimSpace(agentID), "model"}, strings.TrimSpace(profile.Model))
	}
	if strings.TrimSpace(profile.BaseURL) != "" {
		setNestedMapValue(patch, []string{"providers", providerKey, "base_url"}, strings.TrimSpace(profile.BaseURL))
	}
	if strings.TrimSpace(profile.AuthRef) != "" {
		setNestedMapValue(patch, []string{"providers", providerKey, "credential_ref"}, strings.TrimSpace(profile.AuthRef))
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
		ConfigPath:   "$HOME/.openclaw/openclaw.json",
		MemoryPath:   fmt.Sprintf("$HOME/.openclaw/agents/%s/memory", agentID),
		SessionPath:  fmt.Sprintf("$HOME/.openclaw/agents/%s/sessions", agentID),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func extractChatResponseText(payload map[string]interface{}) string {
	for _, key := range []string{"message", "response", "text", "content"} {
		if value := strings.TrimSpace(anyToString(payload[key])); value != "" {
			return value
		}
	}
	if output, ok := payload["output"].(map[string]interface{}); ok {
		for _, key := range []string{"message", "text", "content"} {
			if value := strings.TrimSpace(anyToString(output[key])); value != "" {
				return value
			}
		}
	}
	return ""
}
