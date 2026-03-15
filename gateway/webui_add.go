package gateway

import (
	"carrier/shared/catalog"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type webUIAddRequest struct {
	AgentID         string            `json:"agentId"`
	InstanceID      string            `json:"instanceId,omitempty"`
	Isolation       bool              `json:"isolation,omitempty"`
	Channel         string            `json:"channel"`
	ChannelToken    string            `json:"channelToken"`
	ChannelChatID   string            `json:"channelChatId,omitempty"`
	ProviderID      string            `json:"providerId"`
	ProviderToken   string            `json:"providerToken"`
	ReuseCredential bool              `json:"reuseCredential"`
	EnvVars         map[string]string `json:"envVars"`
}

func handleWebUIAdd(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient) {
	var req webUIAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	agentID := strings.ToLower(strings.TrimSpace(req.AgentID))
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "agentId is required"))
		return
	}

	actor := "webui:add"
	if !isManagedAgent(agentID) {
		instanceID := strings.TrimSpace(req.InstanceID)
		if instanceID == "" {
			generatedID, genErr := generateManagedInstanceID(agentID)
			if genErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to allocate instance id", "generate managed instance id", genErr)
				return
			}
			instanceID = generatedID
		}
		daemonOpts := InstallAgentOptions{Isolation: req.Isolation}
		if err := daemon.InstallAgentWithOptions(r.Context(), agentID, daemonOpts, actor, requestID); err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		if err := daemon.StartAgentWithOptions(r.Context(), agentID, StartAgentOptions{Isolation: req.Isolation}, actor, requestID); err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		inst := managedAgentInstance{
			ID:                  instanceID,
			Type:                agentID,
			AgentID:             agentID,
			Isolation:           req.Isolation,
			GatewayURL:          gatewayURLFromRequest(r),
			RuntimeState:        "running",
			AgentLifecycleMode:  managedAgentLifecyclePersistent,
			MemoryBindingMode:   managedMemoryBindingLiveMount,
			MemoryRefreshPolicy: managedMemoryRefreshNextTurn,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := upsertManagedInstance(inst); err != nil {
			writeStatePersistenceError(w, requestID, "add", agentID, instanceID, err)
			return
		}
		payload := map[string]interface{}{
			"requestId":  requestID,
			"result":     "ok",
			"message":    fmt.Sprintf("%s installed and started", agentID),
			"agentId":    agentID,
			"instanceId": instanceID,
			"isolation":  req.Isolation,
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}

	channelID, webUIOnly, err := normalizeAddChannel(req.Channel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
		return
	}
	req.Channel = channelID
	var ch picoclawChannel
	if !webUIOnly {
		var ok bool
		ch, ok = parseManagedChannel(agentID, channelID)
		if !ok {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", fmt.Sprintf("unsupported channel for %s", agentID)))
			return
		}
	}
	channelToken := strings.TrimSpace(req.ChannelToken)
	if !webUIOnly && channelToken == "" && ch.ID == "telegram" {
		channelToken = strings.TrimSpace(os.Getenv("CARRIER_TELEGRAM_BOT_TOKEN"))
	}
	providerID := strings.TrimSpace(req.ProviderID)
	if providerID == "" {
		providerID = resolveWebUIAddProviderID(agentID)
	}
	provider := GetLLMProvider(providerID)
	if provider == nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_PROVIDER_NOT_FOUND", "providerId is invalid"))
		return
	}
	req.ProviderID = provider.ID

	envVars := sanitizeEnvVars(req.EnvVars)
	token := strings.TrimSpace(req.ProviderToken)
	if provider.AuthMode != AuthModeNone {
		if req.ReuseCredential || token == "" {
			value, _, hasSaved, err := loadProviderCredential(provider.ID)
			if err == nil && hasSaved {
				token = strings.TrimSpace(value)
			} else if err != nil && req.ReuseCredential && token == "" {
				writeInternalGatewayError(w, http.StatusBadRequest, "E_AUTH_INPUT", "failed to read saved credential for selected provider", "load saved provider credential", err)
				return
			}
		}
		if token == "" && strings.TrimSpace(provider.EnvVar) != "" {
			token = strings.TrimSpace(envVars[provider.EnvVar])
		}
		if token == "" && strings.TrimSpace(provider.EnvVar) != "" {
			token = strings.TrimSpace(os.Getenv(provider.EnvVar))
		}
		if token == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_AUTH_INPUT", fmt.Sprintf("provider %s requires credential", provider.ID)))
			return
		}
	}
	for k, v := range ProviderEnvVarsToSet(provider, token, "") {
		envVars[k] = v
	}
	if cfg, managed := managedAgentByID(agentID); managed {
		if cfg.RequiredEnvKey != "" && strings.TrimSpace(envVars[cfg.RequiredEnvKey]) == "" {
			requiredValue := token
			if requiredValue == "" {
				requiredValue = strings.TrimSpace(envVars[provider.EnvVar])
			}
			if requiredValue == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_AUTH_INPUT", fmt.Sprintf("%s requires %s", agentID, cfg.RequiredEnvKey)))
				return
			}
			envVars[cfg.RequiredEnvKey] = strings.TrimSpace(requiredValue)
		}
		if strings.EqualFold(agentID, "zeroclaw") && strings.TrimSpace(envVars["ZEROCLAW_API_KEY"]) == "" {
			zeroKey := token
			if zeroKey == "" {
				zeroKey = strings.TrimSpace(envVars[provider.EnvVar])
			}
			if zeroKey != "" {
				envVars["ZEROCLAW_API_KEY"] = strings.TrimSpace(zeroKey)
			}
		}
	}

	channelSetupPending := webUIOnly || (!webUIOnly && channelToken == "")
	sess := &OnboardSession{
		SelectedAgent:       agentID,
		SelectedChannel:     channelID,
		ChannelToken:        channelToken,
		ChannelSetupPending: channelSetupPending,
		SelectedProvider:    provider.ID,
		EnvVars:             envVars,
	}
	instanceID := strings.TrimSpace(req.InstanceID)
	if instanceID == "" {
		generatedID, genErr := generateManagedInstanceID(agentID)
		if genErr != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to allocate instance id", "generate managed instance id", genErr)
			return
		}
		instanceID = generatedID
	}
	sess.InstanceID = instanceID
	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		if cfg, managed := managedAgentByID(agentID); managed {
			sess.WorkspacePath = filepath.Join(home, cfg.ConfigDir, "instances", instanceID, "workspace")
		}
	}

	prefetchedChatID := strings.TrimSpace(req.ChannelChatID)
	if strings.EqualFold(channelID, "telegram") && prefetchedChatID != "" {
		if actorChatID("telegram:"+prefetchedChatID) == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "channelChatId must be a numeric telegram chat id"))
			return
		}
		actor = "telegram:" + prefetchedChatID
	}

	result, prepErr := prepareManagedOnboard(agentID, sess, actor)
	if prepErr != nil {
		writeInternalGatewayError(w, http.StatusBadRequest, "E_ENV", fmt.Sprintf("failed to prepare %s configuration", agentID), fmt.Sprintf("prepare %s managed onboarding artifacts", agentID), prepErr)
		return
	}
	workspacePath := result.WorkspacePath
	configPath := result.ConfigPath
	recordPath := result.RecordPath
	if err := applyOnboardEnvVars(sess.EnvVars); err != nil {
		writeInternalGatewayError(w, http.StatusBadRequest, "E_ENV", "failed to apply environment variables", "apply onboarding environment", err)
		return
	}
	if err := daemon.InstallAgentWithOptions(r.Context(), agentID, InstallAgentOptions{Isolation: req.Isolation}, actor, requestID); err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	if err := daemon.StartAgentWithOptions(r.Context(), agentID, StartAgentOptions{Isolation: req.Isolation}, actor, requestID); err != nil {
		writeDaemonAPIError(w, err)
		return
	}

	pairCode := ""
	pairedChatID := prefetchedChatID
	if isPicoclawAgent(agentID) {
		if logs, err := daemon.GetLogs(r.Context(), agentID, 120, actor, requestID); err == nil && logs != nil {
			pairCode = extractPairCode(logs.Lines)
			if strings.TrimSpace(pairedChatID) == "" {
				pairedChatID = extractPairedTelegramChatID(logs.Lines)
			}
		}
	}
	pairRequired := strings.EqualFold(channelID, "telegram") && strings.TrimSpace(channelToken) != "" && strings.TrimSpace(pairedChatID) == ""
	if !isPicoclawAgent(agentID) {
		pairRequired = false
	}
	runtimeState := "running"
	if pairRequired {
		runtimeState = "pending_pair"
	}

	envKeys := make([]string, 0, len(envVars))
	for k := range envVars {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	inst := managedAgentInstance{
		ID:                  instanceID,
		Type:                agentID,
		AgentID:             agentID,
		Isolation:           req.Isolation,
		GatewayURL:          gatewayURLFromRequest(r),
		Workspace:           workspacePath,
		ConfigPath:          configPath,
		RecordPath:          recordPath,
		Channel:             channelID,
		Provider:            provider.ID,
		ModelSurface:        &result.ModelSurface,
		PairRequired:        pairRequired,
		PairCode:            pairCode,
		PairedChatID:        pairedChatID,
		RuntimeState:        runtimeState,
		AgentLifecycleMode:  managedAgentLifecyclePersistent,
		MemoryBindingMode:   managedMemoryBindingLiveMount,
		MemoryRefreshPolicy: managedMemoryRefreshNextTurn,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := upsertManagedInstance(inst); err != nil {
		writeStatePersistenceError(w, requestID, "add", agentID, instanceID, err)
		return
	}
	payload := map[string]interface{}{
		"requestId":     requestID,
		"result":        "ok",
		"message":       fmt.Sprintf("%s configured, installed, and started", agentID),
		"agentId":       agentID,
		"instanceId":    instanceID,
		"isolation":     req.Isolation,
		"pairCode":      pairCode,
		"pairRequired":  pairRequired,
		"pairedChatId":  pairedChatID,
		"workspacePath": workspacePath,
		"configPath":    configPath,
		"recordPath":    recordPath,
		"envKeys":       envKeys,
	}
	writeJSON(w, http.StatusOK, payload)
}

func sanitizeEnvVars(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	sanitized := make(map[string]string, len(input))
	for k, v := range input {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		sanitized[key] = strings.TrimSpace(v)
	}
	return sanitized
}

func normalizeAddChannel(raw string) (string, bool, error) {
	channelID := strings.ToLower(strings.TrimSpace(raw))
	switch channelID {
	case "", "skip", "none", "webui":
		return "", true, nil
	case "telegram", "discord", "feishu":
		return channelID, false, nil
	default:
		return "", false, fmt.Errorf("unsupported channel %q; expected telegram, discord, feishu, or skip", raw)
	}
}

func gatewayURLFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.ToLower(forwarded)
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_HOST"))
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		port := strings.TrimSpace(os.Getenv("CARRIER_GATEWAY_PORT"))
		if port == "" {
			port = "8787"
		}
		host = host + ":" + port
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func inferManagedChannelID(agentID string) string {
	if inst, ok := latestManagedInstanceForAgent(agentID); ok {
		if channelID := strings.TrimSpace(inst.Channel); channelID != "" {
			return channelID
		}
	}
	return ""
}

func resolveWebUIAddProviderID(agentID string) string {
	if configuredDefault := strings.TrimSpace(readCarrierDefaultProviderID()); configuredDefault != "" {
		if provider := GetLLMProvider(configuredDefault); provider != nil && providerCompatibleForManagedAgent(agentID, provider) {
			return provider.ID
		}
	}

	if inst, ok := latestManagedInstanceForAgent(agentID); ok {
		if provider := GetLLMProvider(catalog.NormalizeProviderID(strings.TrimSpace(inst.Provider))); provider != nil && providerCompatibleForManagedAgent(agentID, provider) {
			return provider.ID
		}
	}

	for _, provider := range ListLLMProviders() {
		if !providerCompatibleForManagedAgent(agentID, &provider) {
			continue
		}
		if provider.AuthMode == AuthModeNone {
			continue
		}
		if _, _, hasSaved, err := loadProviderCredential(provider.ID); err == nil && hasSaved {
			return provider.ID
		}
	}

	if provider := GetLLMProvider("openai-codex"); provider != nil && providerCompatibleForManagedAgent(agentID, provider) {
		return provider.ID
	}
	if provider := GetLLMProvider("openai"); provider != nil && providerCompatibleForManagedAgent(agentID, provider) {
		return provider.ID
	}

	for _, provider := range ListLLMProviders() {
		if providerCompatibleForManagedAgent(agentID, &provider) {
			return provider.ID
		}
	}
	return ""
}

func providerCompatibleForManagedAgent(agentID string, provider *LLMProvider) bool {
	cfg, ok := managedAgentByID(agentID)
	if !ok || strings.TrimSpace(cfg.RequiredEnvKey) == "" {
		return true
	}
	if provider == nil {
		return false
	}
	if strings.TrimSpace(provider.EnvVar) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv(cfg.RequiredEnvKey)) != ""
}

func latestManagedInstanceForAgent(agentID string) (managedAgentInstance, bool) {
	instances, _, err := loadManagedInstances()
	if err != nil || len(instances) == 0 {
		return managedAgentInstance{}, false
	}

	target := strings.ToLower(strings.TrimSpace(agentID))
	bestIdx := -1
	var bestTime time.Time
	bestHasTime := false

	for i, inst := range instances {
		if target != "" &&
			!strings.EqualFold(strings.TrimSpace(inst.AgentID), target) &&
			!strings.EqualFold(strings.TrimSpace(inst.Type), target) {
			continue
		}

		updatedAt, hasTime := parseManagedInstanceTimestamp(inst.UpdatedAt)
		if bestIdx == -1 {
			bestIdx = i
			bestTime = updatedAt
			bestHasTime = hasTime
			continue
		}

		if hasTime && !bestHasTime {
			bestIdx = i
			bestTime = updatedAt
			bestHasTime = true
			continue
		}
		if hasTime && bestHasTime {
			if updatedAt.After(bestTime) || updatedAt.Equal(bestTime) {
				bestIdx = i
				bestTime = updatedAt
			}
			continue
		}
		if !hasTime && !bestHasTime {
			bestIdx = i
		}
	}

	if bestIdx < 0 {
		return managedAgentInstance{}, false
	}
	return instances[bestIdx], true
}

func parseManagedInstanceTimestamp(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
