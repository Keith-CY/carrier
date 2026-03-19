package gateway

import (
	"carrier/shared/work"
	"net/http"
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
