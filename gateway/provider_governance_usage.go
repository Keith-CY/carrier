package gateway

import (
	"math"
	"strings"
	"unicode/utf8"
)

type providerCostRate struct {
	InputUSDPer1K  float64
	OutputUSDPer1K float64
}

func hydrateProviderGovernanceUsage(execution OrchestratorExecution) OrchestratorExecutionGovernance {
	governance := execution.Governance
	if len(governance.ProviderResolutions) == 0 {
		return governance
	}

	taskIndex := make(map[string]OrchestratorTaskUnit, len(execution.TaskUnits))
	for _, task := range execution.TaskUnits {
		taskIndex[strings.TrimSpace(task.ID)] = task
	}

	resolutions := make([]ProviderGovernanceResolution, 0, len(governance.ProviderResolutions))
	for _, resolution := range governance.ProviderResolutions {
		current := normalizeProviderGovernanceResolution(resolution)
		var inputTokens int
		var outputTokens int
		var successCount int
		var failureCount int
		var latencyTotal int64
		var latencyCount int64

		for _, task := range execution.TaskUnits {
			if !providerResolutionMatchesTask(current, task) {
				continue
			}
			inputTokens += estimateProviderTokens(task.Input)
		}
		for _, result := range execution.Results {
			if !providerResolutionMatchesResult(current, result, taskIndex) {
				continue
			}
			outputTokens += estimateProviderTokens(preferredProviderResultText(result))
			switch result.Status {
			case OrchestratorTaskStatusCompleted:
				successCount++
			case OrchestratorTaskStatusFailed:
				failureCount++
			}
			if result.LatencyMs > 0 {
				latencyTotal += result.LatencyMs
				latencyCount++
			}
		}

		current.EstimatedInputTokens = inputTokens
		current.EstimatedOutputTokens = outputTokens
		current.EstimatedTotalTokens = inputTokens + outputTokens
		current.EstimatedCostUSD = estimateProviderCostUSD(current.Provider, current.Model, inputTokens, outputTokens)
		current.SuccessfulTasks = successCount
		current.FailedTasks = failureCount
		if latencyCount > 0 {
			current.AvgLatencyMs = latencyTotal / latencyCount
		}
		resolutions = append(resolutions, current)
	}
	governance.ProviderResolutions = resolutions
	return governance
}

func normalizeProviderGovernanceResolution(in ProviderGovernanceResolution) ProviderGovernanceResolution {
	out := in
	out.Source = strings.TrimSpace(out.Source)
	out.Status = strings.TrimSpace(out.Status)
	out.HostID = strings.TrimSpace(out.HostID)
	out.AgentID = strings.ToLower(strings.TrimSpace(out.AgentID))
	out.BindingID = strings.TrimSpace(out.BindingID)
	out.BindingTargetType = strings.TrimSpace(out.BindingTargetType)
	out.BindingTargetID = strings.TrimSpace(out.BindingTargetID)
	out.ProfileID = strings.TrimSpace(out.ProfileID)
	out.ProfileName = strings.TrimSpace(out.ProfileName)
	out.Provider = strings.TrimSpace(out.Provider)
	out.Model = strings.TrimSpace(out.Model)
	out.BaseURL = strings.TrimSpace(out.BaseURL)
	out.AuthRef = strings.TrimSpace(out.AuthRef)
	out.SyncMode = strings.TrimSpace(out.SyncMode)
	out.DriftState = strings.TrimSpace(out.DriftState)
	out.DriftReason = strings.TrimSpace(out.DriftReason)
	out.Message = strings.TrimSpace(out.Message)
	if len(out.Trace) > 0 {
		trace := make([]ProviderGovernanceTraceEntry, 0, len(out.Trace))
		for _, item := range out.Trace {
			trace = append(trace, normalizeProviderGovernanceTraceEntry(item))
		}
		out.Trace = trace
	}
	if out.EstimatedInputTokens < 0 {
		out.EstimatedInputTokens = 0
	}
	if out.EstimatedOutputTokens < 0 {
		out.EstimatedOutputTokens = 0
	}
	if out.EstimatedTotalTokens < 0 {
		out.EstimatedTotalTokens = 0
	}
	if out.EstimatedCostUSD < 0 {
		out.EstimatedCostUSD = 0
	}
	if out.SuccessfulTasks < 0 {
		out.SuccessfulTasks = 0
	}
	if out.FailedTasks < 0 {
		out.FailedTasks = 0
	}
	if out.AvgLatencyMs < 0 {
		out.AvgLatencyMs = 0
	}
	return out
}

func normalizeProviderGovernanceTraceEntry(in ProviderGovernanceTraceEntry) ProviderGovernanceTraceEntry {
	out := in
	out.Source = strings.TrimSpace(out.Source)
	out.Status = strings.TrimSpace(out.Status)
	out.BindingID = strings.TrimSpace(out.BindingID)
	out.BindingTargetType = strings.TrimSpace(out.BindingTargetType)
	out.BindingTargetID = strings.TrimSpace(out.BindingTargetID)
	out.ProfileID = strings.TrimSpace(out.ProfileID)
	out.ProfileName = strings.TrimSpace(out.ProfileName)
	out.Provider = strings.TrimSpace(out.Provider)
	out.Model = strings.TrimSpace(out.Model)
	out.SyncMode = strings.TrimSpace(out.SyncMode)
	out.Message = strings.TrimSpace(out.Message)
	return out
}

func providerResolutionMatchesTask(resolution ProviderGovernanceResolution, task OrchestratorTaskUnit) bool {
	taskHostID := strings.TrimSpace(task.HostID)
	if taskHostID == "" {
		taskHostID = orchestratorLocalHostID
	}
	resolutionHostID := strings.TrimSpace(resolution.HostID)
	if resolutionHostID == "" {
		resolutionHostID = orchestratorLocalHostID
	}
	if !strings.EqualFold(taskHostID, resolutionHostID) {
		return false
	}

	taskAgentID := strings.ToLower(strings.TrimSpace(task.AgentID))
	if taskAgentID == "" {
		taskAgentID = "zeroclaw"
	}
	resolutionAgentID := strings.ToLower(strings.TrimSpace(resolution.AgentID))
	if resolutionAgentID == "" {
		resolutionAgentID = "zeroclaw"
	}
	return taskAgentID == resolutionAgentID
}

func providerResolutionMatchesResult(resolution ProviderGovernanceResolution, result OrchestratorTaskResult, tasks map[string]OrchestratorTaskUnit) bool {
	resultHostID := strings.TrimSpace(result.HostID)
	resultAgentID := strings.ToLower(strings.TrimSpace(result.AgentID))
	if task, ok := tasks[strings.TrimSpace(result.TaskID)]; ok {
		if resultHostID == "" {
			resultHostID = strings.TrimSpace(task.HostID)
		}
		if resultAgentID == "" {
			resultAgentID = strings.ToLower(strings.TrimSpace(task.AgentID))
		}
	}
	if resultHostID == "" {
		resultHostID = orchestratorLocalHostID
	}
	if resultAgentID == "" {
		resultAgentID = "zeroclaw"
	}

	resolutionHostID := strings.TrimSpace(resolution.HostID)
	if resolutionHostID == "" {
		resolutionHostID = orchestratorLocalHostID
	}
	resolutionAgentID := strings.ToLower(strings.TrimSpace(resolution.AgentID))
	if resolutionAgentID == "" {
		resolutionAgentID = "zeroclaw"
	}
	return strings.EqualFold(resultHostID, resolutionHostID) && resultAgentID == resolutionAgentID
}

func preferredProviderResultText(result OrchestratorTaskResult) string {
	for _, candidate := range []string{
		strings.TrimSpace(result.Output),
		strings.TrimSpace(result.Error),
		strings.TrimSpace(result.Summary),
		strings.TrimSpace(result.FailureReason),
	} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func estimateProviderTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	runes := utf8.RuneCountInString(trimmed)
	if runes <= 0 {
		return 0
	}
	tokens := int(math.Ceil(float64(runes) / 4.0))
	if tokens < 1 {
		return 1
	}
	return tokens
}

func estimateProviderCostUSD(provider, model string, inputTokens, outputTokens int) float64 {
	rate := resolveProviderCostRate(provider, model)
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	cost := (float64(inputTokens) / 1000.0 * rate.InputUSDPer1K) +
		(float64(outputTokens) / 1000.0 * rate.OutputUSDPer1K)
	return math.Round(cost*1_000_000) / 1_000_000
}

func resolveProviderCostRate(provider, model string) providerCostRate {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(normalizedModel, "claude"):
		return providerCostRate{InputUSDPer1K: 0.003, OutputUSDPer1K: 0.015}
	case strings.Contains(normalizedModel, "gpt-4o"), strings.Contains(normalizedModel, "gpt-4.1"):
		return providerCostRate{InputUSDPer1K: 0.005, OutputUSDPer1K: 0.015}
	case strings.Contains(normalizedModel, "gemini"):
		return providerCostRate{InputUSDPer1K: 0.00125, OutputUSDPer1K: 0.005}
	}

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		return providerCostRate{InputUSDPer1K: 0.003, OutputUSDPer1K: 0.015}
	case "openai":
		return providerCostRate{InputUSDPer1K: 0.005, OutputUSDPer1K: 0.015}
	case "openrouter":
		return providerCostRate{InputUSDPer1K: 0.003, OutputUSDPer1K: 0.012}
	default:
		return providerCostRate{InputUSDPer1K: 0.001, OutputUSDPer1K: 0.002}
	}
}
