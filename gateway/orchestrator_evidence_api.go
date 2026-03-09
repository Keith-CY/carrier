package gateway

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
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

type OrchestratorEvidenceBundle struct {
	GeneratedAt      string                              `json:"generatedAt"`
	Execution        OrchestratorExecution               `json:"execution"`
	Plan             OrchestratorEvidencePlanSnapshot    `json:"plan"`
	Policy           OrchestratorExecutionPolicySnapshot `json:"policy,omitempty"`
	Governance       OrchestratorExecutionGovernance     `json:"governance,omitempty"`
	Authorization    OrchestratorAuthorization           `json:"authorization,omitempty"`
	WorkerLeases     []OrchestratorWorkerLease           `json:"workerLeases,omitempty"`
	Results          []OrchestratorTaskResult            `json:"results,omitempty"`
	ResultSummary    OrchestratorEvidenceResultSummary   `json:"resultSummary"`
	ArtifactManifest []OrchestratorArtifact              `json:"artifactManifest,omitempty"`
	Audit            []gatewayAuditEvent                 `json:"audit,omitempty"`
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
	if executionID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "executionId is required"))
		return
	}
	events, err := listGatewayAuditEventsForExecution(executionID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load gateway audit events", "list gateway audit events for execution", err)
		return
	}
	emitRemoteAuditEvent(requestID, "gateway_audit_export", executionID, "ok", map[string]interface{}{
		"count": len(events),
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId":   requestID,
		"result":      "ok",
		"executionId": executionID,
		"events":      events,
	})
}

func buildOrchestratorEvidenceBundle(execution OrchestratorExecution) (OrchestratorEvidenceBundle, error) {
	leases, err := orchestratorListLeasesByExecution(execution.ID)
	if err != nil {
		return OrchestratorEvidenceBundle{}, err
	}
	auditEvents, err := listGatewayAuditEventsForExecution(execution.ID)
	if err != nil {
		return OrchestratorEvidenceBundle{}, err
	}
	return OrchestratorEvidenceBundle{
		GeneratedAt:      nowTimestamp(),
		Execution:        execution,
		Plan:             buildEvidencePlanSnapshot(execution),
		Policy:           execution.Policy,
		Governance:       execution.Governance,
		Authorization:    execution.Authorization,
		WorkerLeases:     leases,
		Results:          execution.Results,
		ResultSummary:    summarizeOrchestratorEvidenceResults(execution.Results),
		ArtifactManifest: execution.Outcome.Artifacts,
		Audit:            auditEvents,
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
