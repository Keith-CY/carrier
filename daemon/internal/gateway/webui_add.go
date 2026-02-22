package gateway

import (
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
	if !isPicoclawAgent(agentID) && !isOpenclawAgent(agentID) && !isZeroclawAgent(agentID) {
		instanceID := strings.TrimSpace(req.InstanceID)
		if instanceID == "" {
			generatedID, genErr := generateManagedInstanceID(agentID)
			if genErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to allocate instance id", "generate managed instance id", genErr)
				return
			}
			instanceID = generatedID
		}
		if err := daemon.InstallAgent(r.Context(), agentID, actor, requestID); err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		if err := daemon.StartAgent(r.Context(), agentID, actor, requestID); err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		inst := managedAgentInstance{
			ID:           instanceID,
			Type:         agentID,
			AgentID:      agentID,
			GatewayURL:   gatewayURLFromRequest(r),
			RuntimeState: "running",
			CreatedAt:    now,
			UpdatedAt:    now,
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
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}

	var (
		ch picoclawChannel
		ok bool
	)
	switch {
	case isPicoclawAgent(agentID):
		ch, ok = parsePicoclawChannel(req.Channel)
	case isOpenclawAgent(agentID):
		ch, ok = parseOpenclawChannel(req.Channel)
	case isZeroclawAgent(agentID):
		ch, ok = parseZeroclawChannel(req.Channel)
	}
	if !ok && strings.TrimSpace(req.Channel) == "" {
		if strings.TrimSpace(os.Getenv("CARRIER_TELEGRAM_BOT_TOKEN")) != "" {
			req.Channel = "telegram"
			switch {
			case isPicoclawAgent(agentID):
				ch, ok = parsePicoclawChannel(req.Channel)
			case isOpenclawAgent(agentID):
				ch, ok = parseOpenclawChannel(req.Channel)
			case isZeroclawAgent(agentID):
				ch, ok = parseZeroclawChannel(req.Channel)
			}
		}
	}
	if !ok {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", fmt.Sprintf("unsupported channel for %s", agentID)))
		return
	}
	channelToken := strings.TrimSpace(req.ChannelToken)
	if channelToken == "" && ch.ID == "telegram" {
		channelToken = strings.TrimSpace(os.Getenv("CARRIER_TELEGRAM_BOT_TOKEN"))
	}
	if channelToken == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "channelToken is required"))
		return
	}
	provider := GetLLMProvider(req.ProviderID)
	if provider == nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_PROVIDER_NOT_FOUND", "providerId is invalid"))
		return
	}

	envVars := sanitizeEnvVars(req.EnvVars)
	token := strings.TrimSpace(req.ProviderToken)
	if provider.AuthMode != AuthModeNone {
		if token == "" || req.ReuseCredential {
			value, _, hasSaved, err := loadProviderCredential(provider.ID)
			if err != nil {
				writeInternalGatewayError(w, http.StatusBadRequest, "E_AUTH_INPUT", "failed to read saved credential for selected provider", "load saved provider credential", err)
				return
			}
			if !hasSaved {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_AUTH_INPUT", "saved credential is required for the selected provider"))
				return
			}
			token = strings.TrimSpace(value)
		}
		if token == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_AUTH_INPUT", fmt.Sprintf("provider %s requires credential", provider.ID)))
			return
		}
		for k, v := range ProviderEnvVarsToSet(provider, token) {
			envVars[k] = v
		}
	}
	if isOpenclawAgent(agentID) {
		if strings.TrimSpace(envVars["OPENAI_API_KEY"]) == "" {
			openAIKey := token
			if openAIKey == "" {
				openAIKey = strings.TrimSpace(envVars[provider.EnvVar])
			}
			if openAIKey == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_AUTH_INPUT", "openclaw requires OPENAI_API_KEY"))
				return
			}
			envVars["OPENAI_API_KEY"] = strings.TrimSpace(openAIKey)
		}
	}
	if isZeroclawAgent(agentID) && strings.TrimSpace(envVars["ZEROCLAW_API_KEY"]) == "" {
		zeroKey := token
		if zeroKey == "" {
			zeroKey = strings.TrimSpace(envVars[provider.EnvVar])
		}
		if zeroKey != "" {
			envVars["ZEROCLAW_API_KEY"] = strings.TrimSpace(zeroKey)
		}
	}

	sess := &OnboardSession{
		SelectedAgent:    agentID,
		SelectedChannel:  ch.ID,
		ChannelToken:     channelToken,
		SelectedProvider: provider.ID,
		EnvVars:          envVars,
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
		switch {
		case isPicoclawAgent(agentID):
			sess.WorkspacePath = filepath.Join(home, ".picoclaw", "instances", instanceID, "workspace")
		case isOpenclawAgent(agentID):
			sess.WorkspacePath = filepath.Join(home, ".openclaw", "instances", instanceID, "workspace")
		case isZeroclawAgent(agentID):
			sess.WorkspacePath = filepath.Join(home, ".zeroclaw", "instances", instanceID, "workspace")
		}
	}

	prefetchedChatID := strings.TrimSpace(req.ChannelChatID)
	if strings.EqualFold(ch.ID, "telegram") && prefetchedChatID != "" {
		if actorChatID("telegram:"+prefetchedChatID) == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "channelChatId must be a numeric telegram chat id"))
			return
		}
		actor = "telegram:" + prefetchedChatID
	}

	workspacePath := ""
	configPath := ""
	recordPath := ""
	switch {
	case isPicoclawAgent(agentID):
		result, prepErr := preparePicoclawManagedOnboard(sess, actor)
		if prepErr != nil {
			writeInternalGatewayError(w, http.StatusBadRequest, "E_ENV", "failed to prepare picoclaw configuration", "prepare picoclaw managed onboarding artifacts", prepErr)
			return
		}
		workspacePath = result.WorkspacePath
		configPath = result.ConfigPath
		recordPath = result.RecordPath
	case isOpenclawAgent(agentID):
		result, prepErr := prepareOpenclawManagedOnboard(sess, actor)
		if prepErr != nil {
			writeInternalGatewayError(w, http.StatusBadRequest, "E_ENV", "failed to prepare openclaw configuration", "prepare openclaw managed onboarding artifacts", prepErr)
			return
		}
		workspacePath = result.WorkspacePath
		configPath = result.ConfigPath
		recordPath = result.RecordPath
	case isZeroclawAgent(agentID):
		result, prepErr := prepareZeroclawManagedOnboard(sess, actor)
		if prepErr != nil {
			writeInternalGatewayError(w, http.StatusBadRequest, "E_ENV", "failed to prepare zeroclaw configuration", "prepare zeroclaw managed onboarding artifacts", prepErr)
			return
		}
		workspacePath = result.WorkspacePath
		configPath = result.ConfigPath
		recordPath = result.RecordPath
	}
	if err := applyOnboardEnvVars(sess.EnvVars); err != nil {
		writeInternalGatewayError(w, http.StatusBadRequest, "E_ENV", "failed to apply environment variables", "apply onboarding environment", err)
		return
	}
	if err := daemon.InstallAgent(r.Context(), agentID, actor, requestID); err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	if err := daemon.StartAgent(r.Context(), agentID, actor, requestID); err != nil {
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
	pairRequired := strings.EqualFold(ch.ID, "telegram") && strings.TrimSpace(pairedChatID) == ""
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
		ID:           instanceID,
		Type:         agentID,
		AgentID:      agentID,
		GatewayURL:   gatewayURLFromRequest(r),
		Workspace:    workspacePath,
		ConfigPath:   configPath,
		RecordPath:   recordPath,
		Channel:      ch.ID,
		Provider:     provider.ID,
		PairRequired: pairRequired,
		PairCode:     pairCode,
		PairedChatID: pairedChatID,
		RuntimeState: runtimeState,
		CreatedAt:    now,
		UpdatedAt:    now,
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
