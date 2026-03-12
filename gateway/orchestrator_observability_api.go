package gateway

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	sharedconfig "carrier/shared/config"
)

type orchestratorExecutionMetricsSnapshot struct {
	Total                int   `json:"total"`
	PendingAuthorization int   `json:"pendingAuthorization"`
	Provisioning         int   `json:"provisioning"`
	Running              int   `json:"running"`
	Completed            int   `json:"completed"`
	PartialCompleted     int   `json:"partialCompleted"`
	Failed               int   `json:"failed"`
	RetryableFailed      int   `json:"retryableFailed"`
	Cancelled            int   `json:"cancelled"`
	Declined             int   `json:"declined"`
	RetryCount           int   `json:"retryCount"`
	AvgLatencyMs         int64 `json:"avgLatencyMs"`
}

type orchestratorWorkerMetricsSnapshot struct {
	Total        int `json:"total"`
	Provisioning int `json:"provisioning"`
	Ready        int `json:"ready"`
	Busy         int `json:"busy"`
	Reclaiming   int `json:"reclaiming"`
	Reclaimed    int `json:"reclaimed"`
	Error        int `json:"error"`
	Stale        int `json:"stale"`
}

type orchestratorProviderMetricsSnapshot struct {
	RequestedFailures     map[string]int                               `json:"requestedFailures"`
	ResolvedFailures      map[string]int                               `json:"resolvedFailures"`
	DriftStates           map[string]int                               `json:"driftStates,omitempty"`
	Attribution           orchestratorProviderAttributionSnapshot      `json:"attribution,omitempty"`
	Aggregates            []orchestratorProviderAggregateSnapshot      `json:"aggregates,omitempty"`
	Models                []orchestratorProviderModelAggregateSnapshot `json:"models,omitempty"`
	TotalEstimatedCostUSD float64                                      `json:"totalEstimatedCostUsd,omitempty"`
}

type orchestratorProviderAttributionSnapshot struct {
	Teams     []orchestratorUsageAttributionAggregateSnapshot `json:"teams,omitempty"`
	Projects  []orchestratorUsageAttributionAggregateSnapshot `json:"projects,omitempty"`
	Templates []orchestratorUsageAttributionAggregateSnapshot `json:"templates,omitempty"`
	Triggers  []orchestratorUsageAttributionAggregateSnapshot `json:"triggers,omitempty"`
}

type orchestratorUsageAttributionAggregateSnapshot struct {
	Key              string  `json:"key"`
	Label            string  `json:"label"`
	Executions       int     `json:"executions"`
	Successes        int     `json:"successes"`
	Failures         int     `json:"failures"`
	AvgLatencyMs     int64   `json:"avgLatencyMs,omitempty"`
	EstimatedCostUSD float64 `json:"estimatedCostUsd,omitempty"`
}

type orchestratorProviderAggregateSnapshot struct {
	Provider         string  `json:"provider"`
	Successes        int     `json:"successes"`
	Failures         int     `json:"failures"`
	AvgLatencyMs     int64   `json:"avgLatencyMs,omitempty"`
	EstimatedCostUSD float64 `json:"estimatedCostUsd,omitempty"`
}

type orchestratorProviderModelAggregateSnapshot struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	ModelAlias       string  `json:"modelAlias,omitempty"`
	FallbackGroup    string  `json:"fallbackGroup,omitempty"`
	AliasGroupSize   int     `json:"aliasGroupSize,omitempty"`
	Successes        int     `json:"successes"`
	Failures         int     `json:"failures"`
	AvgLatencyMs     int64   `json:"avgLatencyMs,omitempty"`
	EstimatedCostUSD float64 `json:"estimatedCostUsd,omitempty"`
}

type orchestratorPolicyMetricsSnapshot struct {
	Allow int `json:"allow"`
	Ask   int `json:"ask"`
	Deny  int `json:"deny"`
}

type orchestratorMetricsSnapshot struct {
	Timestamp  string                               `json:"timestamp"`
	Executions orchestratorExecutionMetricsSnapshot `json:"executions"`
	Workers    orchestratorWorkerMetricsSnapshot    `json:"workers"`
	Providers  orchestratorProviderMetricsSnapshot  `json:"providers"`
	Policies   orchestratorPolicyMetricsSnapshot    `json:"policies"`
	Queue      OrchestratorWorkerQueueSummary       `json:"queue"`
}

func handleOrchestratorMetrics(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view orchestrator metrics"); !ok {
		return
	}

	executions, err := listOrchestratorExecutions()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator executions", "list orchestrator executions for metrics", err)
		return
	}
	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator worker leases", "list orchestrator worker leases for metrics", err)
		return
	}

	now := time.Now().UTC()
	metrics := buildOrchestratorMetricsSnapshot(executions, leases, cfg, now)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"metrics":   metrics,
	})
}

func buildOrchestratorMetricsSnapshot(executions []OrchestratorExecution, leases []OrchestratorWorkerLease, cfg *GatewayConfig, now time.Time) orchestratorMetricsSnapshot {
	executionsByID := buildOrchestratorExecutionIndex(executions)
	markedLeases := markStaleWorkerLeases(leases, executionsByID, now, cfg)

	executionMetrics := orchestratorExecutionMetricsSnapshot{}
	workerMetrics := orchestratorWorkerMetricsSnapshot{}
	providerMetrics := orchestratorProviderMetricsSnapshot{
		RequestedFailures: map[string]int{},
		ResolvedFailures:  map[string]int{},
		DriftStates:       map[string]int{},
	}
	providerAggregates := map[string]*orchestratorProviderAggregateSnapshot{}
	providerLatencyTotals := map[string]int64{}
	providerLatencyCounts := map[string]int64{}
	modelAggregates := map[string]*orchestratorProviderModelAggregateSnapshot{}
	modelLatencyTotals := map[string]int64{}
	modelLatencyCounts := map[string]int64{}
	teamAttributions := map[string]*orchestratorUsageAttributionAggregateSnapshot{}
	teamLatencyTotals := map[string]int64{}
	teamLatencyCounts := map[string]int64{}
	projectAttributions := map[string]*orchestratorUsageAttributionAggregateSnapshot{}
	projectLatencyTotals := map[string]int64{}
	projectLatencyCounts := map[string]int64{}
	templateAttributions := map[string]*orchestratorUsageAttributionAggregateSnapshot{}
	templateLatencyTotals := map[string]int64{}
	templateLatencyCounts := map[string]int64{}
	triggerAttributions := map[string]*orchestratorUsageAttributionAggregateSnapshot{}
	triggerLatencyTotals := map[string]int64{}
	triggerLatencyCounts := map[string]int64{}
	policyMetrics := orchestratorPolicyMetricsSnapshot{}

	var latencyTotal int64
	var latencyCount int64
	for _, execution := range executions {
		executionMetrics.Total++
		switch execution.Status {
		case OrchestratorExecutionStatusPendingAuthorization:
			executionMetrics.PendingAuthorization++
		case OrchestratorExecutionStatusProvisioning:
			executionMetrics.Provisioning++
		case OrchestratorExecutionStatusRunning:
			executionMetrics.Running++
		case OrchestratorExecutionStatusCompleted:
			executionMetrics.Completed++
		case OrchestratorExecutionStatusPartialCompleted:
			executionMetrics.PartialCompleted++
		case OrchestratorExecutionStatusFailed:
			executionMetrics.Failed++
		case OrchestratorExecutionStatusRetryableFailed:
			executionMetrics.RetryableFailed++
		case OrchestratorExecutionStatusCancelled:
			executionMetrics.Cancelled++
		case OrchestratorExecutionStatusDeclined:
			executionMetrics.Declined++
		}

		for _, result := range execution.Results {
			if result.Attempts > 1 {
				executionMetrics.RetryCount += result.Attempts - 1
			}
		}

		if latencyMs, ok := orchestratorExecutionLatencyMs(execution); ok {
			latencyTotal += latencyMs
			latencyCount++
		}

		switch strings.ToLower(strings.TrimSpace(execution.Policy.Decision)) {
		case "allow":
			policyMetrics.Allow++
		case "ask":
			policyMetrics.Ask++
		case "deny":
			policyMetrics.Deny++
		}

		for _, resolution := range execution.Governance.ProviderResolutions {
			provider := strings.ToLower(strings.TrimSpace(resolution.Provider))
			driftState := strings.ToLower(strings.TrimSpace(resolution.DriftState))
			if driftState != "" {
				providerMetrics.DriftStates[driftState]++
			}
			if provider == "" {
				continue
			}
			aggregate := providerAggregates[provider]
			if aggregate == nil {
				aggregate = &orchestratorProviderAggregateSnapshot{Provider: provider}
				providerAggregates[provider] = aggregate
			}
			aggregate.Successes += resolution.SuccessfulTasks
			aggregate.Failures += resolution.FailedTasks
			aggregate.EstimatedCostUSD += resolution.EstimatedCostUSD
			providerMetrics.TotalEstimatedCostUSD += resolution.EstimatedCostUSD
			taskCount := int64(resolution.SuccessfulTasks + resolution.FailedTasks)
			if resolution.AvgLatencyMs > 0 && taskCount > 0 {
				providerLatencyTotals[provider] += resolution.AvgLatencyMs * taskCount
				providerLatencyCounts[provider] += taskCount
			}

			model := strings.TrimSpace(resolution.Model)
			if model != "" {
				modelKey := provider + "\x00" + strings.ToLower(model)
				modelAggregate := modelAggregates[modelKey]
				if modelAggregate == nil {
					modelAlias, fallbackGroup, aliasGroupSize := resolveProviderModelAliasMetadata(provider, model)
					modelAggregate = &orchestratorProviderModelAggregateSnapshot{
						Provider:       provider,
						Model:          model,
						ModelAlias:     modelAlias,
						FallbackGroup:  fallbackGroup,
						AliasGroupSize: aliasGroupSize,
					}
					modelAggregates[modelKey] = modelAggregate
				}
				modelAggregate.Successes += resolution.SuccessfulTasks
				modelAggregate.Failures += resolution.FailedTasks
				modelAggregate.EstimatedCostUSD += resolution.EstimatedCostUSD
				if resolution.AvgLatencyMs > 0 && taskCount > 0 {
					modelLatencyTotals[modelKey] += resolution.AvgLatencyMs * taskCount
					modelLatencyCounts[modelKey] += taskCount
				}
			}

			accumulateUsageAttribution(teamAttributions, teamLatencyTotals, teamLatencyCounts, execution.Team, execution.Team, resolution)
			accumulateUsageAttribution(projectAttributions, projectLatencyTotals, projectLatencyCounts, execution.Project, execution.Project, resolution)
			accumulateUsageAttribution(templateAttributions, templateLatencyTotals, templateLatencyCounts, execution.TemplateID, execution.TemplateID, resolution)
			triggerKey, triggerLabel := executionTriggerAttributionKey(execution)
			accumulateUsageAttribution(triggerAttributions, triggerLatencyTotals, triggerLatencyCounts, triggerKey, triggerLabel, resolution)
		}

		if !executionHasProviderFailure(execution) {
			continue
		}
		requestedProvider := strings.ToLower(strings.TrimSpace(execution.RequestedProvider))
		if requestedProvider != "" {
			providerMetrics.RequestedFailures[requestedProvider]++
		}
		resolved := map[string]struct{}{}
		for _, resolution := range execution.Governance.ProviderResolutions {
			provider := strings.ToLower(strings.TrimSpace(resolution.Provider))
			if provider == "" {
				continue
			}
			if _, seen := resolved[provider]; seen {
				continue
			}
			resolved[provider] = struct{}{}
			providerMetrics.ResolvedFailures[provider]++
		}
	}
	if latencyCount > 0 {
		executionMetrics.AvgLatencyMs = latencyTotal / latencyCount
	}

	for provider, aggregate := range providerAggregates {
		if count := providerLatencyCounts[provider]; count > 0 {
			aggregate.AvgLatencyMs = providerLatencyTotals[provider] / count
		}
		aggregate.EstimatedCostUSD = roundProviderAggregateCost(aggregate.EstimatedCostUSD)
		providerMetrics.Aggregates = append(providerMetrics.Aggregates, *aggregate)
	}
	sort.SliceStable(providerMetrics.Aggregates, func(i, j int) bool {
		if providerMetrics.Aggregates[i].Provider != providerMetrics.Aggregates[j].Provider {
			return providerMetrics.Aggregates[i].Provider < providerMetrics.Aggregates[j].Provider
		}
		return providerMetrics.Aggregates[i].EstimatedCostUSD > providerMetrics.Aggregates[j].EstimatedCostUSD
	})

	for modelKey, aggregate := range modelAggregates {
		if count := modelLatencyCounts[modelKey]; count > 0 {
			aggregate.AvgLatencyMs = modelLatencyTotals[modelKey] / count
		}
		aggregate.EstimatedCostUSD = roundProviderAggregateCost(aggregate.EstimatedCostUSD)
		providerMetrics.Models = append(providerMetrics.Models, *aggregate)
	}
	sort.SliceStable(providerMetrics.Models, func(i, j int) bool {
		if providerMetrics.Models[i].Provider != providerMetrics.Models[j].Provider {
			return providerMetrics.Models[i].Provider < providerMetrics.Models[j].Provider
		}
		if providerMetrics.Models[i].Model != providerMetrics.Models[j].Model {
			return providerMetrics.Models[i].Model < providerMetrics.Models[j].Model
		}
		return providerMetrics.Models[i].EstimatedCostUSD > providerMetrics.Models[j].EstimatedCostUSD
	})
	providerMetrics.TotalEstimatedCostUSD = roundProviderAggregateCost(providerMetrics.TotalEstimatedCostUSD)
	providerMetrics.Attribution = orchestratorProviderAttributionSnapshot{
		Teams:     flattenUsageAttributionAggregates(teamAttributions, teamLatencyTotals, teamLatencyCounts),
		Projects:  flattenUsageAttributionAggregates(projectAttributions, projectLatencyTotals, projectLatencyCounts),
		Templates: flattenUsageAttributionAggregates(templateAttributions, templateLatencyTotals, templateLatencyCounts),
		Triggers:  flattenUsageAttributionAggregates(triggerAttributions, triggerLatencyTotals, triggerLatencyCounts),
	}

	for _, lease := range markedLeases {
		workerMetrics.Total++
		switch lease.State {
		case OrchestratorWorkerStateProvisioning:
			workerMetrics.Provisioning++
		case OrchestratorWorkerStateReady:
			workerMetrics.Ready++
		case OrchestratorWorkerStateBusy:
			workerMetrics.Busy++
		case OrchestratorWorkerStateReclaiming:
			workerMetrics.Reclaiming++
		case OrchestratorWorkerStateReclaimed:
			workerMetrics.Reclaimed++
		case OrchestratorWorkerStateError:
			workerMetrics.Error++
		}
		if lease.Stale && isActiveOrchestratorWorkerState(lease.State) {
			workerMetrics.Stale++
		}
	}

	return orchestratorMetricsSnapshot{
		Timestamp:  now.UTC().Format(time.RFC3339Nano),
		Executions: executionMetrics,
		Workers:    workerMetrics,
		Providers:  providerMetrics,
		Policies:   policyMetrics,
		Queue:      buildOrchestratorWorkerQueueSummary(executions, markedLeases, cfg, now),
	}
}

func resolveProviderModelAliasMetadata(provider, model string) (string, string, int) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return "", "", 0
	}
	profiles, err := sharedconfig.LoadCarrierModelProfilesForProvider(provider)
	if err != nil || len(profiles) == 0 {
		return "", "", 0
	}
	targetModel := strings.ToLower(model)
	var alias string
	for _, profile := range profiles {
		if strings.EqualFold(strings.TrimSpace(profile.ModelID), targetModel) || strings.EqualFold(strings.TrimSpace(profile.ModelID), model) {
			alias = strings.TrimSpace(profile.ModelAlias)
			if alias == "" {
				alias = strings.TrimSpace(profile.ModelName)
			}
			break
		}
	}
	if alias == "" {
		return "", "", 0
	}
	group := strings.ToLower(strings.TrimSpace(provider)) + ":" + strings.ToLower(alias)
	count := 0
	for _, profile := range profiles {
		candidate := strings.TrimSpace(profile.ModelAlias)
		if candidate == "" {
			candidate = strings.TrimSpace(profile.ModelName)
		}
		if strings.EqualFold(candidate, alias) {
			count++
		}
	}
	return alias, group, count
}

func roundProviderAggregateCost(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return math.Round(value*1_000_000) / 1_000_000
}

func accumulateUsageAttribution(
	target map[string]*orchestratorUsageAttributionAggregateSnapshot,
	latencyTotals map[string]int64,
	latencyCounts map[string]int64,
	key string,
	label string,
	resolution ProviderGovernanceResolution,
) {
	trimmedKey := strings.TrimSpace(key)
	trimmedLabel := strings.TrimSpace(label)
	if trimmedKey == "" || trimmedLabel == "" {
		return
	}
	item := target[trimmedKey]
	if item == nil {
		item = &orchestratorUsageAttributionAggregateSnapshot{
			Key:   trimmedKey,
			Label: trimmedLabel,
		}
		target[trimmedKey] = item
	}
	item.Executions++
	item.Successes += resolution.SuccessfulTasks
	item.Failures += resolution.FailedTasks
	item.EstimatedCostUSD += resolution.EstimatedCostUSD
	taskCount := int64(resolution.SuccessfulTasks + resolution.FailedTasks)
	if resolution.AvgLatencyMs > 0 && taskCount > 0 {
		latencyTotals[trimmedKey] += resolution.AvgLatencyMs * taskCount
		latencyCounts[trimmedKey] += taskCount
	}
}

func executionTriggerAttributionKey(execution OrchestratorExecution) (string, string) {
	source := strings.TrimSpace(execution.TriggerSource)
	triggerID := strings.TrimSpace(execution.TriggerID)
	switch {
	case source != "" && triggerID != "":
		label := source + ":" + triggerID
		return label, label
	case source != "":
		return source, source
	case triggerID != "":
		return triggerID, triggerID
	default:
		return "", ""
	}
}

func flattenUsageAttributionAggregates(
	input map[string]*orchestratorUsageAttributionAggregateSnapshot,
	latencyTotals map[string]int64,
	latencyCounts map[string]int64,
) []orchestratorUsageAttributionAggregateSnapshot {
	if len(input) == 0 {
		return nil
	}
	out := make([]orchestratorUsageAttributionAggregateSnapshot, 0, len(input))
	for key, aggregate := range input {
		item := *aggregate
		if latencyCounts[key] > 0 {
			item.AvgLatencyMs = latencyTotals[key] / latencyCounts[key]
		}
		item.EstimatedCostUSD = roundProviderAggregateCost(item.EstimatedCostUSD)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EstimatedCostUSD != out[j].EstimatedCostUSD {
			return out[i].EstimatedCostUSD > out[j].EstimatedCostUSD
		}
		if out[i].Executions != out[j].Executions {
			return out[i].Executions > out[j].Executions
		}
		return out[i].Label < out[j].Label
	})
	return out
}

func isActiveOrchestratorWorkerState(state OrchestratorWorkerState) bool {
	switch state {
	case OrchestratorWorkerStateProvisioning,
		OrchestratorWorkerStateBusy,
		OrchestratorWorkerStateReclaiming:
		return true
	default:
		return false
	}
}

func orchestratorExecutionLatencyMs(execution OrchestratorExecution) (int64, bool) {
	startedAt := strings.TrimSpace(execution.StartedAt)
	completedAt := strings.TrimSpace(execution.CompletedAt)
	if startedAt == "" || completedAt == "" {
		return 0, false
	}
	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		start, err = time.Parse(time.RFC3339Nano, startedAt)
		if err != nil {
			return 0, false
		}
	}
	end, err := time.Parse(time.RFC3339, completedAt)
	if err != nil {
		end, err = time.Parse(time.RFC3339Nano, completedAt)
		if err != nil {
			return 0, false
		}
	}
	if end.Before(start) {
		return 0, false
	}
	return end.Sub(start).Milliseconds(), true
}

func executionHasProviderFailure(execution OrchestratorExecution) bool {
	if strings.Contains(strings.ToLower(strings.TrimSpace(execution.Outcome.FailureCategory)), "provider") {
		return true
	}
	for _, result := range execution.Results {
		if strings.Contains(strings.ToLower(strings.TrimSpace(result.FailureCategory)), "provider") {
			return true
		}
	}
	return false
}
