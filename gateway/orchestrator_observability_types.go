package gateway

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
	ManagedOverrideHits   int                                          `json:"managedOverrideHits,omitempty"`
	ManagedFallbackHits   int                                          `json:"managedFallbackHits,omitempty"`
	ManagedRuns           []orchestratorManagedModelRunSnapshot        `json:"managedRuns,omitempty"`
	Attribution           orchestratorProviderAttributionSnapshot      `json:"attribution,omitempty"`
	Aggregates            []orchestratorProviderAggregateSnapshot      `json:"aggregates,omitempty"`
	Models                []orchestratorProviderModelAggregateSnapshot `json:"models,omitempty"`
	TotalEstimatedCostUSD float64                                      `json:"totalEstimatedCostUsd,omitempty"`
}

type orchestratorManagedModelRunSnapshot struct {
	AgentID           string `json:"agentId,omitempty"`
	RequestedAlias    string `json:"requestedAlias,omitempty"`
	RequestedModel    string `json:"requestedModel,omitempty"`
	ResolvedModel     string `json:"resolvedModel,omitempty"`
	ResolvedProfile   string `json:"resolvedProfile,omitempty"`
	FallbackGroup     string `json:"fallbackGroup,omitempty"`
	SelectionStrategy string `json:"selectionStrategy,omitempty"`
	SelectionOrdinal  int    `json:"selectionOrdinal,omitempty"`
	OverrideHit       bool   `json:"overrideHit,omitempty"`
	FallbackHit       bool   `json:"fallbackHit,omitempty"`
	LastRunAt         string `json:"lastRunAt,omitempty"`
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
