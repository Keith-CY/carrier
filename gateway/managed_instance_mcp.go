package gateway

import (
	"context"
	"sort"
	"strings"
	"time"

	"carrier/baseagent"
)

func normalizeManagedAgentMCPServers(states []managedAgentMCPServerState) []managedAgentMCPServerState {
	if len(states) == 0 {
		return nil
	}
	normalized := make([]managedAgentMCPServerState, 0, len(states))
	for _, state := range states {
		name := strings.TrimSpace(strings.ToLower(state.Name))
		if name == "" {
			continue
		}
		normalized = append(normalized, managedAgentMCPServerState{
			Name:            name,
			Health:          strings.TrimSpace(state.Health),
			Enabled:         state.Enabled,
			Attached:        state.Attached,
			HealthDetail:    strings.TrimSpace(state.HealthDetail),
			RemediationHint: strings.TrimSpace(state.RemediationHint),
			ConfigDigest:    strings.TrimSpace(state.ConfigDigest),
			ConfigSummary:   strings.TrimSpace(state.ConfigSummary),
			UpdatedAt:       strings.TrimSpace(state.UpdatedAt),
		})
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Name < normalized[j].Name
	})
	deduped := normalized[:0]
	for _, state := range normalized {
		if len(deduped) > 0 && deduped[len(deduped)-1].Name == state.Name {
			deduped[len(deduped)-1] = state
			continue
		}
		deduped = append(deduped, state)
	}
	return deduped
}

func managedAgentMCPServerStateFromDetail(detail baseagent.MCPServerCapability) managedAgentMCPServerState {
	return managedAgentMCPServerState{
		Name:            strings.TrimSpace(strings.ToLower(detail.Name)),
		Health:          strings.TrimSpace(detail.Health),
		Enabled:         detail.Enabled,
		Attached:        detail.Attached,
		HealthDetail:    strings.TrimSpace(detail.HealthDetail),
		RemediationHint: strings.TrimSpace(detail.RemediationHint),
		ConfigDigest:    strings.TrimSpace(detail.ConfigDigest),
		ConfigSummary:   strings.TrimSpace(detail.ConfigSummary),
		UpdatedAt:       nowRFC3339Nano(),
	}
}

func mergeManagedAgentMCPServerState(states []managedAgentMCPServerState, next managedAgentMCPServerState) []managedAgentMCPServerState {
	next.Name = strings.TrimSpace(strings.ToLower(next.Name))
	if next.Name == "" {
		return normalizeManagedAgentMCPServers(states)
	}
	for i := range states {
		if strings.EqualFold(strings.TrimSpace(states[i].Name), next.Name) {
			states[i] = next
			return normalizeManagedAgentMCPServers(states)
		}
	}
	states = append(states, next)
	return normalizeManagedAgentMCPServers(states)
}

func persistManagedAgentMCPServerDetail(agentID string, detail baseagent.MCPServerCapability) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	idx := findManagedInstanceIndexByAgentID(instances, agentID)
	if idx < 0 {
		return nil
	}
	instances[idx].MCPServers = mergeManagedAgentMCPServerState(instances[idx].MCPServers, managedAgentMCPServerStateFromDetail(detail))
	instances[idx].UpdatedAt = nowRFC3339Nano()
	return saveManagedInstances(path, instances)
}

func persistManagedAgentMCPSummary(agentID string, summary baseagent.MCPCapabilitySummary) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	idx := findManagedInstanceIndexByAgentID(instances, agentID)
	if idx < 0 {
		return nil
	}
	if len(summary.Servers) == 0 {
		return nil
	}
	for _, server := range summary.Servers {
		name := strings.TrimSpace(strings.ToLower(server.Name))
		if name == "" {
			continue
		}
		next := managedAgentMCPServerState{
			Name:          name,
			Health:        strings.TrimSpace(server.Health),
			Enabled:       server.Enabled,
			Attached:      server.Attached,
			ConfigDigest:  strings.TrimSpace(server.ConfigDigest),
			ConfigSummary: strings.TrimSpace(server.ConfigSummary),
			UpdatedAt:     nowRFC3339Nano(),
		}
		for _, existing := range instances[idx].MCPServers {
			if !strings.EqualFold(strings.TrimSpace(existing.Name), name) {
				continue
			}
			next.HealthDetail = firstNonEmpty(strings.TrimSpace(existing.HealthDetail), next.HealthDetail)
			next.RemediationHint = firstNonEmpty(strings.TrimSpace(existing.RemediationHint), next.RemediationHint)
			next.ConfigDigest = firstNonEmpty(next.ConfigDigest, strings.TrimSpace(existing.ConfigDigest))
			next.ConfigSummary = firstNonEmpty(next.ConfigSummary, strings.TrimSpace(existing.ConfigSummary))
			break
		}
		instances[idx].MCPServers = mergeManagedAgentMCPServerState(instances[idx].MCPServers, next)
	}
	instances[idx].UpdatedAt = nowRFC3339Nano()
	return saveManagedInstances(path, instances)
}

func reconcileManagedAgentMCPState(ctx context.Context, daemon *DaemonClient, agentID, requestID string) error {
	if daemon == nil {
		return nil
	}
	inst, ok := latestManagedInstanceForAgent(agentID)
	if !ok || len(inst.MCPServers) == 0 {
		return nil
	}
	for _, server := range inst.MCPServers {
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		if config := strings.TrimSpace(server.ConfigSummary); config != "" {
			if _, err := daemon.UpdateAgentMCPServerConfig(ctx, agentID, name, config, "webui:agents:mcp:reconcile", requestID); err != nil {
				return err
			}
		}
		if _, err := daemon.SetAgentMCPServerAttached(ctx, agentID, name, server.Attached, "webui:agents:mcp:reconcile", requestID); err != nil {
			return err
		}
		if server.Attached {
			if _, err := daemon.SetAgentMCPServerEnabled(ctx, agentID, name, server.Enabled, "webui:agents:mcp:reconcile", requestID); err != nil {
				return err
			}
		}
		detail, err := daemon.GetAgentMCPServerDetail(ctx, agentID, name, "webui:agents:mcp:reconcile", requestID)
		if err != nil {
			return err
		}
		if persistErr := persistManagedAgentMCPServerDetail(agentID, detail); persistErr != nil {
			return persistErr
		}
	}
	return nil
}

func managedAgentMCPReconcileWarning(err error) string {
	if err == nil {
		return ""
	}
	return "managed MCP runtime controls were not fully re-applied after start: " + RedactErrorMessage(err.Error())
}

func staleMCPHeartbeatCutoff(now time.Time, updatedAt string) bool {
	if strings.TrimSpace(updatedAt) == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(updatedAt))
	if err != nil {
		return false
	}
	return now.Sub(parsed) > 10*time.Minute
}
