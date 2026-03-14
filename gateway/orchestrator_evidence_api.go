package gateway

import (
	"archive/zip"
	"bytes"
	"carrier/shared/work"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type OrchestratorEvidencePlanSnapshot struct {
	Goal                 string                       `json:"goal"`
	Team                 string                       `json:"team,omitempty"`
	Project              string                       `json:"project,omitempty"`
	Environment          string                       `json:"environment,omitempty"`
	TemplateID           string                       `json:"templateId,omitempty"`
	TriggerSource        string                       `json:"triggerSource,omitempty"`
	TriggerID            string                       `json:"triggerId,omitempty"`
	TriggerEvent         string                       `json:"triggerEvent,omitempty"`
	TriggerPayloadDigest string                       `json:"triggerPayloadDigest,omitempty"`
	Initiator            string                       `json:"initiator,omitempty"`
	RequestedProvider    string                       `json:"requestedProvider,omitempty"`
	RequiredMemory       []string                     `json:"requiredMemory,omitempty"`
	MemoryContractDigest string                       `json:"memoryContractDigest,omitempty"`
	MemoryProvenance     []string                     `json:"memoryProvenance,omitempty"`
	DistillOutputs       []string                     `json:"distillOutputs,omitempty"`
	ParentExecutionID    string                       `json:"parentExecutionId,omitempty"`
	SourceExecutionID    string                       `json:"sourceExecutionId,omitempty"`
	LaunchReason         string                       `json:"launchReason,omitempty"`
	ApprovalScope        string                       `json:"approvalScope"`
	ToolPolicy           OrchestratorToolPolicy       `json:"toolPolicy,omitempty"`
	RequiredWorkers      []OrchestratorRequiredWorker `json:"requiredWorkers,omitempty"`
	TaskUnits            []OrchestratorTaskUnit       `json:"taskUnits,omitempty"`
	MaxConcurrency       int                          `json:"maxConcurrency,omitempty"`
}

type OrchestratorEvidenceResultSummary struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type OrchestratorEvidenceProviderAttribution struct {
	Team                  string                                       `json:"team,omitempty"`
	Project               string                                       `json:"project,omitempty"`
	Environment           string                                       `json:"environment,omitempty"`
	TemplateID            string                                       `json:"templateId,omitempty"`
	Trigger               string                                       `json:"trigger,omitempty"`
	Initiator             string                                       `json:"initiator,omitempty"`
	TotalEstimatedCostUSD float64                                      `json:"totalEstimatedCostUsd,omitempty"`
	Providers             []orchestratorProviderAggregateSnapshot      `json:"providers,omitempty"`
	Models                []orchestratorProviderModelAggregateSnapshot `json:"models,omitempty"`
}

type OrchestratorEvidenceMediaOutput struct {
	ArtifactID     string `json:"artifactId,omitempty"`
	AttachmentID   string `json:"attachmentId,omitempty"`
	Name           string `json:"name,omitempty"`
	Kind           string `json:"kind,omitempty"`
	OutputRole     string `json:"outputRole,omitempty"`
	MediaType      string `json:"mediaType,omitempty"`
	RenderMode     string `json:"renderMode,omitempty"`
	Transport      string `json:"transport,omitempty"`
	DeliveryMethod string `json:"deliveryMethod,omitempty"`
	DeliveryKind   string `json:"deliveryKind,omitempty"`
	DeliveryRef    string `json:"deliveryRef,omitempty"`
	PreviewText    string `json:"previewText,omitempty"`
	Source         string `json:"source,omitempty"`
}

type OrchestratorEvidenceWorkspaceManifest struct {
	WorkspaceID        string `json:"workspaceId,omitempty"`
	WorkspacePath      string `json:"workspacePath,omitempty"`
	Backend            string `json:"backend,omitempty"`
	WorkflowDigest     string `json:"workflowDigest,omitempty"`
	Phase              string `json:"phase,omitempty"`
	LeaseOwner         string `json:"leaseOwner,omitempty"`
	LeaseExpiresAt     string `json:"leaseExpiresAt,omitempty"`
	VerificationStatus string `json:"verificationStatus,omitempty"`
	PublishStatus      string `json:"publishStatus,omitempty"`
	Exists             bool   `json:"exists"`
}

type OrchestratorEvidenceBundle struct {
	GeneratedAt         string                                  `json:"generatedAt"`
	RenderMode          string                                  `json:"renderMode,omitempty"`
	Execution           OrchestratorExecution                   `json:"execution"`
	Plan                OrchestratorEvidencePlanSnapshot        `json:"plan"`
	Policy              OrchestratorExecutionPolicySnapshot     `json:"policy,omitempty"`
	Governance          OrchestratorExecutionGovernance         `json:"governance,omitempty"`
	ProviderAttribution OrchestratorEvidenceProviderAttribution `json:"providerAttribution,omitempty"`
	Authorization       OrchestratorAuthorization               `json:"authorization,omitempty"`
	WorkerLeases        []OrchestratorWorkerLease               `json:"workerLeases,omitempty"`
	Results             []OrchestratorTaskResult                `json:"results,omitempty"`
	ResultSummary       OrchestratorEvidenceResultSummary       `json:"resultSummary"`
	ArtifactManifest    []OrchestratorArtifact                  `json:"artifactManifest,omitempty"`
	MediaOutputs        []OrchestratorEvidenceMediaOutput       `json:"mediaOutputs,omitempty"`
	WorkItemSnapshot    *work.WorkItem                          `json:"workItemSnapshot,omitempty"`
	RunSnapshot         *work.Run                               `json:"runSnapshot,omitempty"`
	WorkspaceManifest   *OrchestratorEvidenceWorkspaceManifest  `json:"workspaceManifest,omitempty"`
	PublishRecords      []workGitHubPublishRecord               `json:"publishRecords,omitempty"`
	Audit               []gatewayAuditEvent                     `json:"audit,omitempty"`
}

func handleOrchestratorExecutionEvidence(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig, execution OrchestratorExecution, parts []string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "evidence bundle not found"))
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "zip" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "format must be json or zip"))
		return
	}

	bundle, err := buildOrchestratorEvidenceBundle(execution)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to build execution evidence bundle", "build orchestrator evidence bundle", err)
		return
	}

	emitRemoteAuditEvent(requestID, "orchestrator_execution_evidence_export", execution.ID, "ok", map[string]interface{}{
		"format": format,
	})

	if format == "zip" {
		data, err := buildOrchestratorEvidenceArchive(bundle, cfg)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to archive execution evidence bundle", "archive orchestrator evidence bundle", err)
			return
		}
		filename := strings.TrimSpace(execution.ID)
		if filename == "" {
			filename = "execution"
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", BuildContentDisposition(filename+"-evidence.zip"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"evidence":  bundle,
	})
}

func handleGatewayAuditExport(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot export execution audit events"); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	executionID := strings.TrimSpace(r.URL.Query().Get("executionId"))
	filter := gatewayAuditExecutionFilter{
		ExecutionID: executionID,
		Team:        strings.TrimSpace(r.URL.Query().Get("team")),
		Project:     strings.TrimSpace(r.URL.Query().Get("project")),
		TemplateID:  strings.TrimSpace(r.URL.Query().Get("templateId")),
		Trigger:     strings.TrimSpace(r.URL.Query().Get("trigger")),
	}
	if gatewayAuditExecutionFilterIsEmpty(normalizeGatewayAuditExecutionFilter(filter)) {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "executionId, team, project, templateId, or trigger is required"))
		return
	}
	events, err := listGatewayAuditEventsForFilter(filter)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load gateway audit events", "list gateway audit events for execution", err)
		return
	}
	emitRemoteAuditEvent(requestID, "gateway_audit_export", executionID, "ok", map[string]interface{}{
		"count":      len(events),
		"team":       filter.Team,
		"project":    filter.Project,
		"templateId": filter.TemplateID,
		"trigger":    filter.Trigger,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId":   requestID,
		"result":      "ok",
		"executionId": executionID,
		"events":      events,
	})
}

func buildOrchestratorEvidenceBundle(execution OrchestratorExecution) (OrchestratorEvidenceBundle, error) {
	executionWithUsage := execution
	executionWithUsage.Governance = hydrateProviderGovernanceUsage(executionWithUsage)

	leases, err := orchestratorListLeasesByExecution(execution.ID)
	if err != nil {
		return OrchestratorEvidenceBundle{}, err
	}
	auditEvents, err := listGatewayAuditEventsForExecution(execution.ID)
	if err != nil {
		return OrchestratorEvidenceBundle{}, err
	}
	workItemSnapshot, runSnapshot, workspaceManifest, publishRecords, err := buildOrchestratorEvidenceWorkContext(executionWithUsage)
	if err != nil {
		return OrchestratorEvidenceBundle{}, err
	}
	return OrchestratorEvidenceBundle{
		GeneratedAt:         nowTimestamp(),
		RenderMode:          strings.TrimSpace(executionWithUsage.Outcome.RenderMode),
		Execution:           executionWithUsage,
		Plan:                buildEvidencePlanSnapshot(executionWithUsage),
		Policy:              executionWithUsage.Policy,
		Governance:          executionWithUsage.Governance,
		ProviderAttribution: buildOrchestratorEvidenceProviderAttribution(executionWithUsage),
		Authorization:       executionWithUsage.Authorization,
		WorkerLeases:        leases,
		Results:             executionWithUsage.Results,
		ResultSummary:       summarizeOrchestratorEvidenceResults(executionWithUsage.Results),
		ArtifactManifest:    executionWithUsage.Outcome.Artifacts,
		MediaOutputs:        buildOrchestratorEvidenceMediaOutputs(executionWithUsage),
		WorkItemSnapshot:    workItemSnapshot,
		RunSnapshot:         runSnapshot,
		WorkspaceManifest:   workspaceManifest,
		PublishRecords:      publishRecords,
		Audit:               auditEvents,
	}, nil
}

func buildEvidencePlanSnapshot(execution OrchestratorExecution) OrchestratorEvidencePlanSnapshot {
	return OrchestratorEvidencePlanSnapshot{
		Goal:                 strings.TrimSpace(execution.Goal),
		Team:                 strings.TrimSpace(execution.Team),
		Project:              strings.TrimSpace(execution.Project),
		Environment:          strings.TrimSpace(execution.Environment),
		TemplateID:           strings.TrimSpace(execution.TemplateID),
		TriggerSource:        strings.TrimSpace(execution.TriggerSource),
		TriggerID:            strings.TrimSpace(execution.TriggerID),
		TriggerEvent:         strings.TrimSpace(execution.TriggerEvent),
		TriggerPayloadDigest: strings.TrimSpace(execution.TriggerPayloadDigest),
		Initiator:            strings.TrimSpace(execution.Initiator),
		RequestedProvider:    strings.TrimSpace(execution.RequestedProvider),
		RequiredMemory:       append([]string(nil), execution.RequiredMemory...),
		MemoryContractDigest: strings.TrimSpace(execution.MemoryContractDigest),
		MemoryProvenance:     append([]string(nil), execution.MemoryProvenance...),
		DistillOutputs:       append([]string(nil), execution.DistillOutputs...),
		ParentExecutionID:    strings.TrimSpace(execution.ParentExecutionID),
		SourceExecutionID:    strings.TrimSpace(execution.SourceExecutionID),
		LaunchReason:         strings.TrimSpace(execution.LaunchReason),
		ApprovalScope:        strings.TrimSpace(execution.ApprovalScope),
		ToolPolicy:           execution.ToolPolicy,
		RequiredWorkers:      execution.RequiredWorkers,
		TaskUnits:            execution.TaskUnits,
		MaxConcurrency:       execution.MaxConcurrency,
	}
}

func summarizeOrchestratorEvidenceResults(results []OrchestratorTaskResult) OrchestratorEvidenceResultSummary {
	summary := OrchestratorEvidenceResultSummary{Total: len(results)}
	for _, result := range results {
		switch result.Status {
		case OrchestratorTaskStatusCompleted:
			summary.Completed++
		case OrchestratorTaskStatusFailed:
			summary.Failed++
		}
	}
	return summary
}

func buildOrchestratorEvidenceArchive(bundle OrchestratorEvidenceBundle, cfg *GatewayConfig) ([]byte, error) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	addJSON := func(name string, value interface{}) error {
		writer, err := archive.Create(name)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = writer.Write(append(data, '\n'))
		return err
	}

	if err := addJSON("bundle.json", bundle); err != nil {
		return nil, err
	}
	if err := addJSON("execution.json", bundle.Execution); err != nil {
		return nil, err
	}
	if err := addJSON("plan.json", bundle.Plan); err != nil {
		return nil, err
	}
	if err := addJSON("policy.json", bundle.Policy); err != nil {
		return nil, err
	}
	if err := addJSON("governance.json", bundle.Governance); err != nil {
		return nil, err
	}
	if err := addJSON("provider-attribution.json", bundle.ProviderAttribution); err != nil {
		return nil, err
	}
	if err := addJSON("authorization.json", bundle.Authorization); err != nil {
		return nil, err
	}
	if err := addJSON("worker-leases.json", bundle.WorkerLeases); err != nil {
		return nil, err
	}
	if err := addJSON("results.json", bundle.Results); err != nil {
		return nil, err
	}
	if err := addJSON("result-summary.json", bundle.ResultSummary); err != nil {
		return nil, err
	}
	if err := addJSON("artifact-manifest.json", bundle.ArtifactManifest); err != nil {
		return nil, err
	}
	if err := addJSON("media-outputs.json", bundle.MediaOutputs); err != nil {
		return nil, err
	}
	if bundle.WorkItemSnapshot != nil {
		if err := addJSON("work-item-snapshot.json", bundle.WorkItemSnapshot); err != nil {
			return nil, err
		}
	}
	if bundle.RunSnapshot != nil {
		if err := addJSON("run-snapshot.json", bundle.RunSnapshot); err != nil {
			return nil, err
		}
	}
	if bundle.WorkspaceManifest != nil {
		if err := addJSON("workspace-manifest.json", bundle.WorkspaceManifest); err != nil {
			return nil, err
		}
	}
	if len(bundle.PublishRecords) > 0 {
		if err := addJSON("publish-records.json", bundle.PublishRecords); err != nil {
			return nil, err
		}
	}
	if err := addJSON("audit.json", bundle.Audit); err != nil {
		return nil, err
	}

	usedNames := map[string]int{}
	for _, artifact := range bundle.ArtifactManifest {
		data, filename, _, err := loadExecutionArtifact(cfg, artifact)
		if err != nil {
			continue
		}
		entryName := evidenceArchiveArtifactEntryName(filename, artifact.ID, usedNames)
		writer, err := archive.Create(entryName)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func evidenceArchiveArtifactEntryName(filename, artifactID string, used map[string]int) string {
	base := filepath.Base(strings.TrimSpace(filename))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = strings.TrimSpace(artifactID)
	}
	if base == "" {
		base = "artifact"
	}
	entry := "artifacts/" + base
	if used[entry] == 0 {
		used[entry] = 1
		return entry
	}
	trimmedID := strings.TrimSpace(artifactID)
	if trimmedID == "" {
		trimmedID = "artifact"
	}
	entry = "artifacts/" + trimmedID + "-" + base
	if used[entry] == 0 {
		used[entry] = 1
		return entry
	}
	used[entry]++
	return "artifacts/" + trimmedID + "-" + strconv.Itoa(used[entry]) + "-" + base
}

func buildOrchestratorEvidenceProviderAttribution(execution OrchestratorExecution) OrchestratorEvidenceProviderAttribution {
	triggerLabel, _ := executionTriggerAttributionKey(execution)
	out := OrchestratorEvidenceProviderAttribution{
		Team:        strings.TrimSpace(execution.Team),
		Project:     strings.TrimSpace(execution.Project),
		Environment: strings.TrimSpace(execution.Environment),
		TemplateID:  strings.TrimSpace(execution.TemplateID),
		Trigger:     strings.TrimSpace(triggerLabel),
		Initiator:   strings.TrimSpace(execution.Initiator),
	}
	if len(execution.Governance.ProviderResolutions) == 0 {
		return out
	}

	providerAggregates := map[string]*orchestratorProviderAggregateSnapshot{}
	providerLatencyTotals := map[string]int64{}
	providerLatencyCounts := map[string]int64{}
	modelAggregates := map[string]*orchestratorProviderModelAggregateSnapshot{}
	modelLatencyTotals := map[string]int64{}
	modelLatencyCounts := map[string]int64{}
	var totalEstimatedCostUSD float64

	for _, resolution := range execution.Governance.ProviderResolutions {
		provider := strings.ToLower(strings.TrimSpace(resolution.Provider))
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
		totalEstimatedCostUSD += resolution.EstimatedCostUSD

		taskCount := int64(resolution.SuccessfulTasks + resolution.FailedTasks)
		if resolution.AvgLatencyMs > 0 && taskCount > 0 {
			providerLatencyTotals[provider] += resolution.AvgLatencyMs * taskCount
			providerLatencyCounts[provider] += taskCount
		}

		model := strings.TrimSpace(resolution.Model)
		if model == "" {
			continue
		}
		modelKey := provider + "\x00" + strings.ToLower(model)
		modelAggregate := modelAggregates[modelKey]
		if modelAggregate == nil {
			modelAggregate = &orchestratorProviderModelAggregateSnapshot{
				Provider: provider,
				Model:    model,
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

	for provider, aggregate := range providerAggregates {
		if count := providerLatencyCounts[provider]; count > 0 {
			aggregate.AvgLatencyMs = providerLatencyTotals[provider] / count
		}
		aggregate.EstimatedCostUSD = roundProviderAggregateCost(aggregate.EstimatedCostUSD)
		out.Providers = append(out.Providers, *aggregate)
	}
	sort.SliceStable(out.Providers, func(i, j int) bool {
		if out.Providers[i].EstimatedCostUSD != out.Providers[j].EstimatedCostUSD {
			return out.Providers[i].EstimatedCostUSD > out.Providers[j].EstimatedCostUSD
		}
		return out.Providers[i].Provider < out.Providers[j].Provider
	})

	for modelKey, aggregate := range modelAggregates {
		if count := modelLatencyCounts[modelKey]; count > 0 {
			aggregate.AvgLatencyMs = modelLatencyTotals[modelKey] / count
		}
		aggregate.EstimatedCostUSD = roundProviderAggregateCost(aggregate.EstimatedCostUSD)
		out.Models = append(out.Models, *aggregate)
	}
	sort.SliceStable(out.Models, func(i, j int) bool {
		if out.Models[i].EstimatedCostUSD != out.Models[j].EstimatedCostUSD {
			return out.Models[i].EstimatedCostUSD > out.Models[j].EstimatedCostUSD
		}
		if out.Models[i].Provider != out.Models[j].Provider {
			return out.Models[i].Provider < out.Models[j].Provider
		}
		return out.Models[i].Model < out.Models[j].Model
	})

	out.TotalEstimatedCostUSD = roundProviderAggregateCost(totalEstimatedCostUSD)
	return out
}

func buildOrchestratorEvidenceMediaOutputs(execution OrchestratorExecution) []OrchestratorEvidenceMediaOutput {
	renderMode := strings.TrimSpace(execution.Outcome.RenderMode)
	out := make([]OrchestratorEvidenceMediaOutput, 0, len(execution.Outcome.Artifacts))
	for _, artifact := range execution.Outcome.Artifacts {
		mediaType := strings.TrimSpace(artifact.MediaType)
		outputRole := strings.TrimSpace(artifact.OutputRole)
		if mediaType == "" && outputRole != "generated" {
			continue
		}
		deliveryRef := strings.TrimSpace(firstString(artifact.DownloadURL, artifact.ExternalID, artifact.Path))
		out = append(out, OrchestratorEvidenceMediaOutput{
			ArtifactID:     strings.TrimSpace(artifact.ID),
			AttachmentID:   strings.TrimSpace(artifact.AttachmentID),
			Name:           strings.TrimSpace(artifact.Name),
			Kind:           strings.TrimSpace(artifact.Kind),
			OutputRole:     outputRole,
			MediaType:      mediaType,
			RenderMode:     renderMode,
			Transport:      resolveEvidenceMediaTransport(artifact),
			DeliveryMethod: resolveEvidenceMediaDeliveryMethod(artifact),
			DeliveryKind:   evidenceMediaDeliveryKind(artifact),
			DeliveryRef:    deliveryRef,
			PreviewText:    resolveEvidenceMediaPreviewText(artifact),
			Source:         strings.TrimSpace(artifact.Source),
		})
	}
	return out
}

func buildOrchestratorEvidenceWorkContext(execution OrchestratorExecution) (*work.WorkItem, *work.Run, *OrchestratorEvidenceWorkspaceManifest, []workGitHubPublishRecord, error) {
	workContext := normalizeOrchestratorExecutionWorkContext(execution.Work)
	if execution.Mode != OrchestratorExecutionModeWork && workContext.ProjectID == "" && workContext.WorkItemID == "" && workContext.RunID == "" && workContext.WorkspaceID == "" {
		return nil, nil, nil, nil, nil
	}

	var runSnapshot *work.Run
	runID := strings.TrimSpace(workContext.RunID)
	if runID != "" {
		run, ok, err := getWorkRun(runID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if ok {
			runCopy := run
			runSnapshot = &runCopy
		}
	}

	workItemID := strings.TrimSpace(workContext.WorkItemID)
	if workItemID == "" && runSnapshot != nil {
		workItemID = strings.TrimSpace(runSnapshot.WorkItemID)
	}
	var workItemSnapshot *work.WorkItem
	if workItemID != "" {
		item, ok, err := getWorkItem(workItemID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if ok {
			itemCopy := item
			workItemSnapshot = &itemCopy
		}
	}

	var workspaceManifest *OrchestratorEvidenceWorkspaceManifest
	workspacePath := strings.TrimSpace(workContext.WorkspacePath)
	if workspacePath == "" && runSnapshot != nil {
		workspacePath = strings.TrimSpace(runSnapshot.WorkspacePath)
	}
	if workContext.WorkspaceID != "" || workspacePath != "" || runSnapshot != nil {
		manifest := &OrchestratorEvidenceWorkspaceManifest{
			WorkspaceID:        strings.TrimSpace(firstString(workContext.WorkspaceID, valueFromRun(runSnapshot, func(r work.Run) string { return r.WorkspaceID }))),
			WorkspacePath:      workspacePath,
			Backend:            strings.TrimSpace(firstString(workContext.Backend, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.Backend) }))),
			WorkflowDigest:     strings.TrimSpace(firstString(workContext.WorkflowDigest, valueFromRun(runSnapshot, func(r work.Run) string { return r.WorkflowDigest }))),
			Phase:              strings.TrimSpace(firstString(workContext.Phase, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.Phase) }))),
			LeaseOwner:         valueFromRun(runSnapshot, func(r work.Run) string { return r.LeaseOwner }),
			LeaseExpiresAt:     valueFromRun(runSnapshot, func(r work.Run) string { return r.LeaseExpiresAt }),
			VerificationStatus: strings.TrimSpace(firstString(workContext.VerificationStatus, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.VerificationStatus) }))),
			PublishStatus:      strings.TrimSpace(firstString(workContext.PublishStatus, valueFromRun(runSnapshot, func(r work.Run) string { return string(r.PublishStatus) }))),
		}
		if manifest.WorkspacePath != "" {
			if _, err := os.Stat(manifest.WorkspacePath); err == nil {
				manifest.Exists = true
			} else if err != nil && !os.IsNotExist(err) {
				return nil, nil, nil, nil, err
			}
		}
		workspaceManifest = manifest
	}

	projectID := strings.TrimSpace(workContext.ProjectID)
	if projectID == "" && runSnapshot != nil {
		projectID = strings.TrimSpace(runSnapshot.ProjectID)
	}
	publishRecords, err := listGitHubPublishRecords(projectID, runID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return workItemSnapshot, runSnapshot, workspaceManifest, publishRecords, nil
}

func valueFromRun(run *work.Run, selector func(work.Run) string) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(selector(*run))
}

func evidenceMediaDeliveryKind(artifact OrchestratorArtifact) string {
	mediaType := strings.ToLower(strings.TrimSpace(artifact.MediaType))
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return "image"
	case strings.HasPrefix(mediaType, "audio/ogg") || strings.Contains(mediaType, "opus") || kind == "voice":
		return "voice"
	case strings.HasPrefix(mediaType, "audio/") || kind == "audio":
		return "audio"
	case strings.HasPrefix(mediaType, "video/") || kind == "video":
		return "video"
	case mediaType != "" || kind == "document" || kind == "file":
		return "document"
	default:
		return "unknown"
	}
}

func resolveEvidenceMediaTransport(artifact OrchestratorArtifact) string {
	return strings.TrimSpace(firstString(artifact.Transport, artifact.Source))
}

func resolveEvidenceMediaDeliveryMethod(artifact OrchestratorArtifact) string {
	if method := strings.TrimSpace(artifact.DeliveryMethod); method != "" {
		return method
	}
	kind := evidenceMediaDeliveryKind(artifact)
	switch strings.TrimSpace(resolveEvidenceMediaTransport(artifact)) {
	case "telegram":
		method, _ := telegramMediaMethodAndField(kind)
		if method != "" {
			return method
		}
	}
	if kind == "unknown" {
		return ""
	}
	return kind
}

func resolveEvidenceMediaPreviewText(artifact OrchestratorArtifact) string {
	if preview := strings.TrimSpace(artifact.PreviewText); preview != "" {
		return preview
	}
	name := strings.TrimSpace(artifact.Name)
	if name == "" {
		return ""
	}
	switch evidenceMediaDeliveryKind(artifact) {
	case "image":
		return "Image: " + name
	case "audio", "voice":
		return "Audio: " + name
	case "video":
		return "Video: " + name
	default:
		return "Attachment: " + name
	}
}
