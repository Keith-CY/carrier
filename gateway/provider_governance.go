package gateway

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func handleProviderGovernanceResolve(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if !flags.ProviderBindingEnabled {
		writeJSON(w, http.StatusForbidden, gatewayErrBody("E_FEATURE_DISABLED", "provider binding feature is disabled"))
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	hostID := strings.TrimSpace(r.URL.Query().Get("hostId"))
	agentID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("agentId")))
	if hostID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "hostId is required"))
		return
	}
	if agentID != "" {
		if err := validateAgentIdentifier(agentID); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
	}

	resolution, err := resolveProviderGovernance(hostID, agentID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to resolve provider governance", "resolve provider governance", err)
		return
	}
	emitRemoteAuditEvent(requestID, "provider_governance_resolve", hostID+":"+agentID, "success", map[string]interface{}{
		"source":    resolution.Source,
		"status":    resolution.Status,
		"profileId": resolution.ProfileID,
		"provider":  resolution.Provider,
		"model":     resolution.Model,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId":  requestID,
		"result":     "ok",
		"resolution": resolution,
	})
}

func resolveProviderGovernance(hostID, agentID string) (ProviderGovernanceResolution, error) {
	resolutions, err := resolveProviderGovernanceForWorkers([]OrchestratorRequiredWorker{{
		HostID:  hostID,
		AgentID: agentID,
		Count:   1,
	}})
	if err != nil {
		return ProviderGovernanceResolution{}, err
	}
	if len(resolutions) == 0 {
		return ProviderGovernanceResolution{
			Source:  "none",
			Status:  "unbound",
			HostID:  strings.TrimSpace(hostID),
			AgentID: strings.ToLower(strings.TrimSpace(agentID)),
		}, nil
	}
	return resolutions[0], nil
}

func resolveProviderGovernanceForWorkers(workers []OrchestratorRequiredWorker) ([]ProviderGovernanceResolution, error) {
	bindings, err := listProviderBindings()
	if err != nil {
		return nil, err
	}

	out := make([]ProviderGovernanceResolution, 0, len(workers))
	for _, worker := range workers {
		hostID := strings.TrimSpace(worker.HostID)
		agentID := strings.ToLower(strings.TrimSpace(worker.AgentID))
		if hostID == "" {
			hostID = orchestratorLocalHostID
		}
		resolution := ProviderGovernanceResolution{
			Source:  "none",
			Status:  "unbound",
			HostID:  hostID,
			AgentID: agentID,
			Message: "no provider binding matched",
		}
		if hostID == orchestratorLocalHostID {
			out = append(out, resolution)
			continue
		}

		binding, source, found := selectEffectiveProviderBinding(bindings, hostID, agentID)
		if !found {
			out = append(out, resolution)
			continue
		}

		resolution.Source = source
		resolution.BindingID = strings.TrimSpace(binding.ID)
		resolution.BindingTargetType = strings.TrimSpace(binding.TargetType)
		resolution.BindingTargetID = strings.TrimSpace(binding.TargetID)
		resolution.SyncMode = strings.TrimSpace(binding.SyncMode)
		resolution.ProfileID = strings.TrimSpace(binding.ProfileID)

		profile, profileFound, profileErr := getProviderProfile(binding.ProfileID)
		if profileErr != nil {
			return nil, profileErr
		}
		if !profileFound {
			resolution.Status = "broken_profile"
			resolution.Message = fmt.Sprintf("profile %s not found", strings.TrimSpace(binding.ProfileID))
			out = append(out, resolution)
			continue
		}

		resolution.Status = "resolved"
		resolution.ProfileName = strings.TrimSpace(profile.Name)
		resolution.Provider = strings.TrimSpace(profile.Provider)
		resolution.Model = strings.TrimSpace(profile.Model)
		resolution.BaseURL = strings.TrimSpace(profile.BaseURL)
		resolution.AuthRef = strings.TrimSpace(profile.AuthRef)
		resolution.Enabled = profile.Enabled
		if !profile.Enabled {
			resolution.Status = "disabled_profile"
			resolution.Message = "bound profile is disabled"
		} else {
			resolution.Message = ""
		}
		out = append(out, resolution)
	}
	return out, nil
}

func selectEffectiveProviderBinding(bindings []ProviderBinding, hostID, agentID string) (ProviderBinding, string, bool) {
	trimmedHostID := strings.TrimSpace(hostID)
	trimmedAgentID := strings.ToLower(strings.TrimSpace(agentID))
	instanceMatches := make([]ProviderBinding, 0)
	hostMatches := make([]ProviderBinding, 0)
	for _, binding := range bindings {
		normalized := normalizeProviderBinding(binding)
		switch normalized.TargetType {
		case "instance":
			bindingHostID, bindingAgentID := splitInstanceBindingTarget(normalized.TargetID)
			if strings.EqualFold(strings.TrimSpace(bindingHostID), trimmedHostID) &&
				strings.EqualFold(strings.ToLower(strings.TrimSpace(bindingAgentID)), trimmedAgentID) &&
				trimmedAgentID != "" {
				instanceMatches = append(instanceMatches, normalized)
			}
		case "host":
			if strings.EqualFold(strings.TrimSpace(normalized.TargetID), trimmedHostID) {
				hostMatches = append(hostMatches, normalized)
			}
		}
	}
	sortProviderBindingsByRecency(instanceMatches)
	if len(instanceMatches) > 0 {
		return instanceMatches[0], "instance", true
	}
	sortProviderBindingsByRecency(hostMatches)
	if len(hostMatches) > 0 {
		return hostMatches[0], "host", true
	}
	return ProviderBinding{}, "", false
}

func sortProviderBindingsByRecency(bindings []ProviderBinding) {
	sort.SliceStable(bindings, func(i, j int) bool {
		left := parseRFC3339OrNow(bindings[i].UpdatedAt)
		right := parseRFC3339OrNow(bindings[j].UpdatedAt)
		if !left.Equal(right) {
			return left.After(right)
		}
		return strings.ToLower(strings.TrimSpace(bindings[i].ID)) < strings.ToLower(strings.TrimSpace(bindings[j].ID))
	})
}
