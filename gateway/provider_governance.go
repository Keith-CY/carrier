package gateway

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	sharedconfig "carrier/shared/config"
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
		"source":      resolution.Source,
		"status":      resolution.Status,
		"profileId":   resolution.ProfileID,
		"provider":    resolution.Provider,
		"model":       resolution.Model,
		"driftState":  resolution.DriftState,
		"driftReason": resolution.DriftReason,
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
			Source:     "none",
			Status:     "unbound",
			HostID:     hostID,
			AgentID:    agentID,
			DriftState: "unbound",
			Message:    "no provider binding matched",
		}
		if hostID == orchestratorLocalHostID {
			out = append(out, resolution)
			continue
		}

		candidates := collectProviderGovernanceBindingCandidates(bindings, hostID, agentID)
		if len(candidates) == 0 {
			out = append(out, resolution)
			continue
		}

		trace := make([]ProviderGovernanceTraceEntry, 0, len(candidates))
		for idx, candidate := range candidates {
			entry, profile, profileFound, profileErr := buildProviderGovernanceTraceEntry(candidate.Binding, candidate.Source)
			if profileErr != nil {
				return nil, profileErr
			}
			if idx == 0 {
				entry.Selected = true
				resolution.Source = entry.Source
				resolution.Status = entry.Status
				resolution.BindingID = entry.BindingID
				resolution.BindingTargetType = entry.BindingTargetType
				resolution.BindingTargetID = entry.BindingTargetID
				resolution.ProfileID = entry.ProfileID
				resolution.ProfileName = entry.ProfileName
				resolution.Provider = entry.Provider
				resolution.Model = entry.Model
				resolution.SyncMode = entry.SyncMode
				resolution.Enabled = entry.Enabled
				resolution.Message = entry.Message
				if profileFound {
					resolution.BaseURL = strings.TrimSpace(profile.BaseURL)
					resolution.AuthRef = strings.TrimSpace(profile.AuthRef)
				}
			} else if entry.Status == "resolved" {
				entry.Status = "shadowed"
				entry.Message = "shadowed by higher precedence binding"
			}
			trace = append(trace, entry)
		}
		resolution.Trace = trace
		resolution.DriftState, resolution.DriftReason = deriveProviderGovernanceDriftState(trace)
		out = append(out, resolution)
	}
	return out, nil
}

type providerGovernanceBindingCandidate struct {
	Binding ProviderBinding
	Source  string
}

func collectProviderGovernanceBindingCandidates(bindings []ProviderBinding, hostID, agentID string) []providerGovernanceBindingCandidate {
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
	sortProviderBindingsByRecency(hostMatches)
	out := make([]providerGovernanceBindingCandidate, 0, 2)
	if len(instanceMatches) > 0 {
		out = append(out, providerGovernanceBindingCandidate{Binding: instanceMatches[0], Source: "instance"})
	}
	if len(hostMatches) > 0 {
		out = append(out, providerGovernanceBindingCandidate{Binding: hostMatches[0], Source: "host"})
	}
	return out
}

func buildProviderGovernanceTraceEntry(binding ProviderBinding, source string) (ProviderGovernanceTraceEntry, ProviderProfile, bool, error) {
	entry := ProviderGovernanceTraceEntry{
		Source:            strings.TrimSpace(source),
		Status:            "resolved",
		BindingID:         strings.TrimSpace(binding.ID),
		BindingTargetType: strings.TrimSpace(binding.TargetType),
		BindingTargetID:   strings.TrimSpace(binding.TargetID),
		ProfileID:         strings.TrimSpace(binding.ProfileID),
		SyncMode:          strings.TrimSpace(binding.SyncMode),
	}
	profile, found, err := getProviderProfile(binding.ProfileID)
	if err != nil {
		return ProviderGovernanceTraceEntry{}, ProviderProfile{}, false, err
	}
	if !found {
		entry.Status = "broken_profile"
		entry.Message = fmt.Sprintf("profile %s not found", strings.TrimSpace(binding.ProfileID))
		return entry, ProviderProfile{}, false, nil
	}
	if candidates, err := sharedconfig.LoadCarrierModelProfilesForAlias(profile.Provider, profile.Model); err == nil && len(candidates) > 0 {
		profile.Model = strings.TrimSpace(candidates[0].ModelID)
		if strings.TrimSpace(profile.BaseURL) == "" {
			profile.BaseURL = strings.TrimSpace(candidates[0].BaseURL)
		}
	}
	entry.ProfileName = strings.TrimSpace(profile.Name)
	entry.Provider = strings.TrimSpace(profile.Provider)
	entry.Model = strings.TrimSpace(profile.Model)
	entry.Enabled = profile.Enabled
	if !profile.Enabled {
		entry.Status = "disabled_profile"
		entry.Message = "bound profile is disabled"
	}
	return entry, profile, true, nil
}

func deriveProviderGovernanceDriftState(trace []ProviderGovernanceTraceEntry) (string, string) {
	if len(trace) == 0 {
		return "unbound", ""
	}
	selected := trace[0]
	switch selected.Status {
	case "broken_profile", "disabled_profile":
		return selected.Status, strings.TrimSpace(selected.Message)
	case "resolved":
		if selected.Source == "instance" {
			for _, candidate := range trace[1:] {
				if candidate.Source != "host" {
					continue
				}
				if bindingResolutionsDiffer(selected, candidate) {
					reason := "instance binding overrides host binding"
					if candidate.ProfileName != "" || candidate.ProfileID != "" {
						reason += " (" + strings.TrimSpace(firstNonEmpty(candidate.ProfileName, candidate.ProfileID)) + ")"
					}
					return "override", reason
				}
			}
		}
		return "in_sync", ""
	default:
		return strings.TrimSpace(selected.Status), strings.TrimSpace(selected.Message)
	}
}

func bindingResolutionsDiffer(selected, fallback ProviderGovernanceTraceEntry) bool {
	if !strings.EqualFold(strings.TrimSpace(selected.ProfileID), strings.TrimSpace(fallback.ProfileID)) {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(selected.Provider), strings.TrimSpace(fallback.Provider)) {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(selected.Model), strings.TrimSpace(fallback.Model)) {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(selected.SyncMode), strings.TrimSpace(fallback.SyncMode))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
