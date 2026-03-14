package gateway

import (
	"archive/zip"
	"bytes"
	"carrier/shared/work"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleOrchestratorExecutionEvidenceJSONAndAuditExport(t *testing.T) {
	tmp := t.TempDir()
	artifactRoot, err := os.MkdirTemp(".", "evidence-artifacts-json-*")
	if err != nil {
		t.Fatalf("mkdir artifact root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(artifactRoot) })
	artifactPath := filepath.Join(artifactRoot, "release-notes.txt")
	if err := os.WriteFile(artifactPath, []byte("release notes"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	auditLog := filepath.Join(tmp, "gateway-audit.jsonl")
	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", auditLog)

	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		ArtifactRoot:              artifactRoot,
	}, nil)
	hostID := createRemoteHostForTests(t, mux)

	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:                "exec-evidence-json",
		Goal:              "collect incident evidence",
		TemplateID:        "incident-diagnosis",
		RequestedProvider: "openrouter",
		ApprovalScope:     "infrastructure_only",
		Status:            OrchestratorExecutionStatusFailed,
		MaxConcurrency:    2,
		Authorization: OrchestratorAuthorization{
			InfrastructureApproved: true,
			ApprovedBy:             "alice",
			ApprovedAt:             "2026-03-09T09:00:00Z",
		},
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision:                       orchestratorPolicyDecisionAsk,
			Reason:                         "prod incident access requires approval",
			Summary:                        "ask for production incident workflows",
			MatchedRuleID:                  "policy-prod",
			MatchedRuleName:                "prod guardrail",
			RequiresInfrastructureApproval: true,
			EffectiveMaxConcurrency:        2,
		},
		Governance: OrchestratorExecutionGovernance{
			ProviderResolutions: []ProviderGovernanceResolution{{
				HostID:      hostID,
				AgentID:     "zeroclaw",
				ProfileName: "openrouter-prod",
				Provider:    "openrouter",
				Model:       "anthropic/claude-3.7-sonnet",
				Source:      "host_binding",
			}},
		},
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  hostID,
			AgentID: "zeroclaw",
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:          "task-1",
			Input:       "collect deployment diagnostics",
			AgentID:     "zeroclaw",
			TimeoutMs:   45000,
			RetryBudget: 1,
			HostID:      hostID,
		}},
		Results: []OrchestratorTaskResult{{
			TaskID:          "task-1",
			Status:          OrchestratorTaskStatusFailed,
			WorkerID:        "worker-1",
			HostID:          hostID,
			AgentID:         "zeroclaw",
			Attempts:        2,
			Summary:         "diagnostics upload failed",
			Error:           "ssh session lost",
			FailureReason:   "worker disconnected",
			FailureCategory: "worker_failed",
		}},
		Outcome: OrchestratorExecutionOutcome{
			Summary:         "incident evidence collection failed",
			FailureReason:   "remote diagnostics upload failed",
			FailureCategory: "worker_failed",
			RenderMode:      "rich_media",
			Artifacts: []OrchestratorArtifact{{
				ID:             "artifact-1",
				AttachmentID:   "attach-1",
				TaskID:         "task-1",
				Name:           "release-notes.txt",
				Kind:           "text",
				OutputRole:     "generated",
				MediaType:      "text/plain",
				ContentType:    "text/plain",
				SizeBytes:      13,
				Path:           artifactPath,
				Source:         "telegram",
				ExternalID:     "tg-doc-1",
				DownloadURL:    "/downloads/tok-1/release-notes.txt",
				Transport:      "telegram",
				DeliveryMethod: "sendDocument",
				PreviewText:    "Attachment: release-notes.txt",
				CreatedAt:      "2026-03-09T09:01:00Z",
			}},
		},
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution failed: %v", err)
	}
	if _, err := upsertOrchestratorWorkerLease(OrchestratorWorkerLease{
		ID:              "lease-1",
		ExecutionID:     seed.ID,
		HostID:          hostID,
		AgentID:         "zeroclaw",
		State:           OrchestratorWorkerStateBusy,
		LeaseState:      "leased",
		Ephemeral:       true,
		TaskCount:       1,
		LastHeartbeatAt: "2026-03-09T09:00:30Z",
		LeaseExpireAt:   "2026-03-09T09:05:00Z",
		CreatedAt:       "2026-03-09T09:00:00Z",
		UpdatedAt:       "2026-03-09T09:01:00Z",
	}); err != nil {
		t.Fatalf("upsertOrchestratorWorkerLease failed: %v", err)
	}

	emitRemoteAuditEvent("req-create", "orchestrator_execution_create", seed.ID, "ok", map[string]interface{}{
		"executionId": seed.ID,
		"goal":        seed.Goal,
	})
	emitRemoteAuditEvent("req-authorize", "orchestrator_execution_authorize", seed.ID, "ok", map[string]interface{}{
		"executionId": seed.ID,
		"actor":       "alice",
	})

	rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/evidence?format=json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected evidence json status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	evidence, _ := payload["evidence"].(map[string]interface{})
	if evidence == nil {
		t.Fatalf("missing evidence payload: %+v", payload)
	}
	execution, _ := evidence["execution"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(execution["id"])); got != seed.ID {
		t.Fatalf("execution.id=%q want %q payload=%+v", got, seed.ID, payload)
	}
	plan, _ := evidence["plan"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(plan["templateId"])); got != "incident-diagnosis" {
		t.Fatalf("plan.templateId=%q want incident-diagnosis plan=%+v", got, plan)
	}
	policy, _ := evidence["policy"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(policy["matchedRuleId"])); got != "policy-prod" {
		t.Fatalf("policy.matchedRuleId=%q want policy-prod policy=%+v", got, policy)
	}
	governance, _ := evidence["governance"].(map[string]interface{})
	resolutions, _ := governance["providerResolutions"].([]interface{})
	if len(resolutions) != 1 {
		t.Fatalf("providerResolutions len=%d want 1 governance=%+v", len(resolutions), governance)
	}
	providerAttribution, _ := evidence["providerAttribution"].(map[string]interface{})
	if got := anyToFloat(providerAttribution["totalEstimatedCostUsd"]); got <= 0 {
		t.Fatalf("providerAttribution.totalEstimatedCostUsd=%f want > 0 attribution=%+v", got, providerAttribution)
	}
	providerUsage, _ := providerAttribution["providers"].([]interface{})
	if len(providerUsage) != 1 {
		t.Fatalf("providerAttribution.providers len=%d want 1 attribution=%+v", len(providerUsage), providerAttribution)
	}
	leases, _ := evidence["workerLeases"].([]interface{})
	if len(leases) != 1 {
		t.Fatalf("workerLeases len=%d want 1 evidence=%+v", len(leases), evidence)
	}
	results, _ := evidence["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results len=%d want 1 evidence=%+v", len(results), evidence)
	}
	resultSummary, _ := evidence["resultSummary"].(map[string]interface{})
	if got := int(anyToFloat(resultSummary["failed"])); got != 1 {
		t.Fatalf("resultSummary.failed=%d want 1 summary=%+v", got, resultSummary)
	}
	if got := strings.TrimSpace(anyToString(evidence["renderMode"])); got != "rich_media" {
		t.Fatalf("evidence.renderMode=%q want rich_media evidence=%+v", got, evidence)
	}
	manifest, _ := evidence["artifactManifest"].([]interface{})
	if len(manifest) != 1 {
		t.Fatalf("artifactManifest len=%d want 1 evidence=%+v", len(manifest), evidence)
	}
	manifestEntry, _ := manifest[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(manifestEntry["mediaType"])); got != "text/plain" {
		t.Fatalf("artifactManifest.mediaType=%q want text/plain entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["source"])); got != "telegram" {
		t.Fatalf("artifactManifest.source=%q want telegram entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["externalId"])); got != "tg-doc-1" {
		t.Fatalf("artifactManifest.externalId=%q want tg-doc-1 entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["attachmentId"])); got != "attach-1" {
		t.Fatalf("artifactManifest.attachmentId=%q want attach-1 entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["outputRole"])); got != "generated" {
		t.Fatalf("artifactManifest.outputRole=%q want generated entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["downloadUrl"])); got != "/downloads/tok-1/release-notes.txt" {
		t.Fatalf("artifactManifest.downloadUrl=%q want /downloads/tok-1/release-notes.txt entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["transport"])); got != "telegram" {
		t.Fatalf("artifactManifest.transport=%q want telegram entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["deliveryMethod"])); got != "sendDocument" {
		t.Fatalf("artifactManifest.deliveryMethod=%q want sendDocument entry=%+v", got, manifestEntry)
	}
	if got := strings.TrimSpace(anyToString(manifestEntry["previewText"])); got != "Attachment: release-notes.txt" {
		t.Fatalf("artifactManifest.previewText=%q want Attachment: release-notes.txt entry=%+v", got, manifestEntry)
	}
	mediaOutputs, _ := evidence["mediaOutputs"].([]interface{})
	if len(mediaOutputs) != 1 {
		t.Fatalf("mediaOutputs len=%d want 1 evidence=%+v", len(mediaOutputs), evidence)
	}
	mediaEntry, _ := mediaOutputs[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(mediaEntry["deliveryKind"])); got != "document" {
		t.Fatalf("mediaOutputs.deliveryKind=%q want document entry=%+v", got, mediaEntry)
	}
	if got := strings.TrimSpace(anyToString(mediaEntry["deliveryRef"])); got != "/downloads/tok-1/release-notes.txt" {
		t.Fatalf("mediaOutputs.deliveryRef=%q want /downloads/tok-1/release-notes.txt entry=%+v", got, mediaEntry)
	}
	if got := strings.TrimSpace(anyToString(mediaEntry["renderMode"])); got != "rich_media" {
		t.Fatalf("mediaOutputs.renderMode=%q want rich_media entry=%+v", got, mediaEntry)
	}
	if got := strings.TrimSpace(anyToString(mediaEntry["transport"])); got != "telegram" {
		t.Fatalf("mediaOutputs.transport=%q want telegram entry=%+v", got, mediaEntry)
	}
	if got := strings.TrimSpace(anyToString(mediaEntry["deliveryMethod"])); got != "sendDocument" {
		t.Fatalf("mediaOutputs.deliveryMethod=%q want sendDocument entry=%+v", got, mediaEntry)
	}
	if got := strings.TrimSpace(anyToString(mediaEntry["previewText"])); got != "Attachment: release-notes.txt" {
		t.Fatalf("mediaOutputs.previewText=%q want Attachment: release-notes.txt entry=%+v", got, mediaEntry)
	}
	audit, _ := evidence["audit"].([]interface{})
	if len(audit) != 2 {
		t.Fatalf("audit len=%d want 2 evidence=%+v", len(audit), evidence)
	}

	exportRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/audit/export?executionId="+seed.ID, "")
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected audit export status 200, got %d body=%s", exportRec.Code, exportRec.Body.String())
	}
	exportPayload := decodeJSONMap(t, exportRec)
	events, _ := exportPayload["events"].([]interface{})
	if len(events) != 3 {
		t.Fatalf("events len=%d want 3 payload=%+v", len(events), exportPayload)
	}

	auditData, err := os.ReadFile(auditLog)
	if err != nil {
		t.Fatalf("read audit log failed: %v", err)
	}
	auditText := string(auditData)
	if !strings.Contains(auditText, "orchestrator_execution_evidence_export") {
		t.Fatalf("expected evidence export audit event, got %s", auditText)
	}
	if !strings.Contains(auditText, "gateway_audit_export") {
		t.Fatalf("expected audit export event, got %s", auditText)
	}
}

func TestHandleOrchestratorExecutionEvidenceZipAndNegativeCases(t *testing.T) {
	tmp := t.TempDir()
	artifactRoot, err := os.MkdirTemp(".", "evidence-artifacts-zip-*")
	if err != nil {
		t.Fatalf("mkdir artifact root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(artifactRoot) })
	artifactPath := filepath.Join(artifactRoot, "summary.json")
	if err := os.WriteFile(artifactPath, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", filepath.Join(tmp, "gateway-audit.jsonl"))
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
		ArtifactRoot:              artifactRoot,
	}, nil)
	hostID := createRemoteHostForTests(t, mux)

	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:            "exec-evidence-zip",
		Goal:          "bundle execution evidence",
		ApprovalScope: "infrastructure_only",
		Status:        OrchestratorExecutionStatusCancelled,
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  hostID,
			AgentID: "picoclaw",
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:      "task-zip",
			Input:   "summarize rollback",
			AgentID: "picoclaw",
			HostID:  hostID,
		}},
		Outcome: OrchestratorExecutionOutcome{
			RenderMode: "text_fallback",
			Artifacts: []OrchestratorArtifact{{
				ID:             "artifact-zip",
				AttachmentID:   "attach-zip",
				TaskID:         "task-zip",
				Name:           "summary.json",
				Kind:           "json",
				OutputRole:     "fallback",
				MediaType:      "application/json",
				ContentType:    "application/json",
				SizeBytes:      11,
				Path:           artifactPath,
				Source:         "telegram",
				ExternalID:     "tg-file-zip",
				DownloadURL:    "/downloads/tok-zip/summary.json",
				Transport:      "telegram",
				DeliveryMethod: "sendDocument",
				PreviewText:    "Attachment: summary.json",
			}},
		},
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution failed: %v", err)
	}
	emitRemoteAuditEvent("req-cancel", "orchestrator_execution_cancel", seed.ID, "ok", map[string]interface{}{
		"executionId": seed.ID,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/evidence?format=zip", nil)
	req.Header.Set("Authorization", "Bearer test-gateway-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected evidence zip status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/zip") {
		t.Fatalf("expected zip content type, got %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, seed.ID+"-evidence.zip") {
		t.Fatalf("expected evidence attachment filename, got %q", got)
	}
	reader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("open zip response: %v", err)
	}
	entries := map[string]string{}
	for _, file := range reader.File {
		rc, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, openErr)
		}
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, readErr)
		}
		entries[file.Name] = string(data)
	}
	required := []string{
		"bundle.json",
		"execution.json",
		"plan.json",
		"policy.json",
		"governance.json",
		"authorization.json",
		"worker-leases.json",
		"results.json",
		"result-summary.json",
		"provider-attribution.json",
		"artifact-manifest.json",
		"media-outputs.json",
		"audit.json",
		"artifacts/summary.json",
	}
	for _, name := range required {
		if _, ok := entries[name]; !ok {
			t.Fatalf("missing zip entry %q entries=%v", name, keysOfStringMap(entries))
		}
	}

	var manifest []map[string]interface{}
	if err := json.Unmarshal([]byte(entries["artifact-manifest.json"]), &manifest); err != nil {
		t.Fatalf("decode artifact manifest: %v", err)
	}
	if len(manifest) != 1 || strings.TrimSpace(anyToString(manifest[0]["id"])) != "artifact-zip" {
		t.Fatalf("unexpected artifact manifest: %+v", manifest)
	}
	if got := strings.TrimSpace(anyToString(manifest[0]["attachmentId"])); got != "attach-zip" {
		t.Fatalf("artifact-manifest attachmentId=%q want attach-zip manifest=%+v", got, manifest)
	}
	if got := strings.TrimSpace(anyToString(manifest[0]["outputRole"])); got != "fallback" {
		t.Fatalf("artifact-manifest outputRole=%q want fallback manifest=%+v", got, manifest)
	}
	if got := strings.TrimSpace(anyToString(manifest[0]["downloadUrl"])); got != "/downloads/tok-zip/summary.json" {
		t.Fatalf("artifact-manifest downloadUrl=%q want /downloads/tok-zip/summary.json manifest=%+v", got, manifest)
	}
	if got := strings.TrimSpace(anyToString(manifest[0]["transport"])); got != "telegram" {
		t.Fatalf("artifact-manifest transport=%q want telegram manifest=%+v", got, manifest)
	}
	if got := strings.TrimSpace(anyToString(manifest[0]["deliveryMethod"])); got != "sendDocument" {
		t.Fatalf("artifact-manifest deliveryMethod=%q want sendDocument manifest=%+v", got, manifest)
	}
	if got := strings.TrimSpace(anyToString(manifest[0]["previewText"])); got != "Attachment: summary.json" {
		t.Fatalf("artifact-manifest previewText=%q want Attachment: summary.json manifest=%+v", got, manifest)
	}
	var mediaOutputs []map[string]interface{}
	if err := json.Unmarshal([]byte(entries["media-outputs.json"]), &mediaOutputs); err != nil {
		t.Fatalf("decode media outputs: %v", err)
	}
	if len(mediaOutputs) != 1 || strings.TrimSpace(anyToString(mediaOutputs[0]["artifactId"])) != "artifact-zip" {
		t.Fatalf("unexpected media outputs: %+v", mediaOutputs)
	}
	if got := strings.TrimSpace(anyToString(mediaOutputs[0]["deliveryKind"])); got != "document" {
		t.Fatalf("media-outputs deliveryKind=%q want document outputs=%+v", got, mediaOutputs)
	}
	if got := strings.TrimSpace(anyToString(mediaOutputs[0]["transport"])); got != "telegram" {
		t.Fatalf("media-outputs transport=%q want telegram outputs=%+v", got, mediaOutputs)
	}
	if got := strings.TrimSpace(anyToString(mediaOutputs[0]["deliveryMethod"])); got != "sendDocument" {
		t.Fatalf("media-outputs deliveryMethod=%q want sendDocument outputs=%+v", got, mediaOutputs)
	}
	if got := strings.TrimSpace(anyToString(mediaOutputs[0]["previewText"])); got != "Attachment: summary.json" {
		t.Fatalf("media-outputs previewText=%q want Attachment: summary.json outputs=%+v", got, mediaOutputs)
	}

	methodRec := runJSONRequest(t, mux, http.MethodPost, "/api/v1/orchestrator/executions/"+seed.ID+"/evidence", `{}`)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected evidence method status 405, got %d body=%s", methodRec.Code, methodRec.Body.String())
	}
	badFormatRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/evidence?format=tar", "")
	if badFormatRec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad evidence format status 400, got %d body=%s", badFormatRec.Code, badFormatRec.Body.String())
	}
	missingExecutionRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/audit/export", "")
	if missingExecutionRec.Code != http.StatusBadRequest {
		t.Fatalf("expected audit export validation status 400, got %d body=%s", missingExecutionRec.Code, missingExecutionRec.Body.String())
	}
}

func TestHandleOrchestratorExecutionEvidenceIncludesWorkSnapshots(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		ID:           "proj_alpha",
		Name:         "Alpha",
		SourceType:   work.SourceTypeLocal,
		SourceRef:    t.TempDir(),
		WorkflowPath: "WORKFLOW.md",
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}
	item, err := upsertWorkItem(work.WorkItem{
		ID:          "work_bug",
		ProjectID:   project.ID,
		Title:       "Fix worker drift",
		Description: "Investigate stale worker leases in the control plane.",
		Acceptance:  []string{"Document the source of drift", "Propose remediation steps"},
		Priority:    work.WorkPriorityUrgent,
		Source:      work.WorkSourceLocal,
		SourceRef:   "local:manual",
		State:       work.WorkItemStateAwaitingReview,
		LatestRunID: "run_123",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}
	roots, err := work.ResolveRoots()
	if err != nil {
		t.Fatalf("ResolveRoots error: %v", err)
	}
	workspacePath := filepath.Join(roots.Projects, project.ID, "worktrees", "run_123")
	if err := os.MkdirAll(workspacePath, 0o700); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	run, err := work.NormalizeRun(work.Run{
		ID:                 "run_123",
		ProjectID:          project.ID,
		WorkItemID:         item.ID,
		ExecutionID:        "exec-work-evidence",
		WorkspaceID:        "ws_123",
		WorkspacePath:      workspacePath,
		Backend:            work.RunBackendManaged,
		Phase:              work.RunPhasePublishing,
		LeaseOwner:         "carrier:test",
		VerificationStatus: work.VerificationStatusPassed,
		PublishStatus:      work.PublishStatusPublished,
		WorkflowDigest:     "sha256:workflow-alpha",
	})
	if err != nil {
		t.Fatalf("NormalizeRun error: %v", err)
	}
	if err := saveWorkRun(run); err != nil {
		t.Fatalf("saveWorkRun error: %v", err)
	}
	record := workGitHubPublishRecord{
		ID:         "publish_123",
		ProjectID:  project.ID,
		WorkItemID: item.ID,
		RunID:      run.ID,
		Repository: "acme/alpha",
		SourceRef:  "github:acme/alpha/issues/12",
		Targets: []workGitHubPublishTargetStatus{{
			Target: "comment",
			Status: string(work.PublishStatusPublished),
		}},
		CreatedAt: nowTimestamp(),
	}
	if err := saveGitHubPublishRecord(record); err != nil {
		t.Fatalf("saveGitHubPublishRecord error: %v", err)
	}

	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}, nil)
	execution, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:     "exec-work-evidence",
		Mode:   OrchestratorExecutionModeWork,
		Goal:   "Fix worker drift",
		Status: OrchestratorExecutionStatusCompleted,
		Work: OrchestratorExecutionWorkContext{
			ProjectID:      project.ID,
			WorkItemID:     item.ID,
			RunID:          run.ID,
			WorkspaceID:    run.WorkspaceID,
			WorkflowDigest: run.WorkflowDigest,
			Phase:          string(run.Phase),
		},
		CreatedAt: nowTimestamp(),
		UpdatedAt: nowTimestamp(),
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution error: %v", err)
	}

	rec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+execution.ID+"/evidence?format=json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected evidence json status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload := decodeJSONMap(t, rec)
	evidence, _ := payload["evidence"].(map[string]interface{})
	if evidence == nil {
		t.Fatalf("missing evidence payload: %+v", payload)
	}
	workItemSnapshot, _ := evidence["workItemSnapshot"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(workItemSnapshot["id"])); got != item.ID {
		t.Fatalf("workItemSnapshot.id=%q want %q evidence=%+v", got, item.ID, evidence)
	}
	runSnapshot, _ := evidence["runSnapshot"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(runSnapshot["id"])); got != run.ID {
		t.Fatalf("runSnapshot.id=%q want %q evidence=%+v", got, run.ID, evidence)
	}
	workspaceManifest, _ := evidence["workspaceManifest"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(workspaceManifest["workspacePath"])); got != workspacePath {
		t.Fatalf("workspaceManifest.workspacePath=%q want %q manifest=%+v", got, workspacePath, workspaceManifest)
	}
	if got := strings.TrimSpace(anyToString(workspaceManifest["backend"])); got != string(run.Backend) {
		t.Fatalf("workspaceManifest.backend=%q want %q manifest=%+v", got, run.Backend, workspaceManifest)
	}
	publishRecords, _ := evidence["publishRecords"].([]interface{})
	if len(publishRecords) != 1 {
		t.Fatalf("publishRecords len=%d want 1 evidence=%+v", len(publishRecords), evidence)
	}

	zipRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+execution.ID+"/evidence?format=zip", "")
	if zipRec.Code != http.StatusOK {
		t.Fatalf("expected evidence zip status 200, got %d body=%s", zipRec.Code, zipRec.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(zipRec.Body.Bytes()), int64(zipRec.Body.Len()))
	if err != nil {
		t.Fatalf("open evidence zip: %v", err)
	}
	names := make(map[string]struct{}, len(archive.File))
	for _, file := range archive.File {
		names[file.Name] = struct{}{}
	}
	archiveNames := make([]string, 0, len(names))
	for name := range names {
		archiveNames = append(archiveNames, name)
	}
	for _, required := range []string{
		"work-item-snapshot.json",
		"run-snapshot.json",
		"workspace-manifest.json",
		"publish-records.json",
	} {
		if _, ok := names[required]; !ok {
			t.Fatalf("missing %s in zip: %v", required, archiveNames)
		}
	}
}

func TestHandleGatewayAuditExportSupportsMetadataFilters(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_GATEWAY_AUDIT_LOG", filepath.Join(tmp, "gateway-audit.jsonl"))

	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
		RemoteChatEnabled:         true,
		ProviderBindingEnabled:    true,
	}, nil)

	for _, execution := range []OrchestratorExecution{
		{
			ID:            "exec-audit-platform",
			Goal:          "release note prep",
			Team:          "platform",
			Project:       "carrier",
			TemplateID:    "pr-triage",
			TriggerSource: "github",
			TriggerID:     "trigger-gh-1",
			ApprovalScope: "infrastructure_only",
			Status:        OrchestratorExecutionStatusCompleted,
			CreatedAt:     nowTimestamp(),
			UpdatedAt:     nowTimestamp(),
		},
		{
			ID:            "exec-audit-sre",
			Goal:          "incident diagnosis",
			Team:          "sre",
			Project:       "checkout",
			TemplateID:    "incident-diagnosis",
			TriggerSource: "schedule",
			TriggerID:     "trigger-nightly",
			ApprovalScope: "infrastructure_only",
			Status:        OrchestratorExecutionStatusFailed,
			CreatedAt:     nowTimestamp(),
			UpdatedAt:     nowTimestamp(),
		},
	} {
		if _, err := upsertOrchestratorExecution(execution); err != nil {
			t.Fatalf("upsertOrchestratorExecution(%s) failed: %v", execution.ID, err)
		}
	}

	emitRemoteAuditEvent("req-1", "orchestrator_execution_create", "exec-audit-platform", "ok", map[string]interface{}{
		"executionId": "exec-audit-platform",
	})
	emitRemoteAuditEvent("req-2", "orchestrator_execution_create", "exec-audit-sre", "ok", map[string]interface{}{
		"executionId": "exec-audit-sre",
	})

	platformRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/audit/export?team=platform&templateId=pr-triage", "")
	if platformRec.Code != http.StatusOK {
		t.Fatalf("expected filtered audit export status 200, got %d body=%s", platformRec.Code, platformRec.Body.String())
	}
	platformPayload := decodeJSONMap(t, platformRec)
	platformEvents, _ := platformPayload["events"].([]interface{})
	if len(platformEvents) != 1 {
		t.Fatalf("filtered platform events len=%d want 1 payload=%+v", len(platformEvents), platformPayload)
	}
	platformEvent, _ := platformEvents[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(platformEvent["target"])); got != "exec-audit-platform" {
		t.Fatalf("filtered platform target=%q want exec-audit-platform event=%+v", got, platformEvent)
	}

	triggerRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/audit/export?trigger=schedule:trigger-nightly", "")
	if triggerRec.Code != http.StatusOK {
		t.Fatalf("expected trigger audit export status 200, got %d body=%s", triggerRec.Code, triggerRec.Body.String())
	}
	triggerPayload := decodeJSONMap(t, triggerRec)
	triggerEvents, _ := triggerPayload["events"].([]interface{})
	if len(triggerEvents) != 1 {
		t.Fatalf("filtered trigger events len=%d want 1 payload=%+v", len(triggerEvents), triggerPayload)
	}
	triggerEvent, _ := triggerEvents[0].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(triggerEvent["target"])); got != "exec-audit-sre" {
		t.Fatalf("filtered trigger target=%q want exec-audit-sre event=%+v", got, triggerEvent)
	}
}

func keysOfStringMap(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
