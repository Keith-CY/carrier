package gateway

import (
	"net/http"
	"strings"
	"testing"

	"carrier/baseagent"
)

func TestNormalizeOrchestratorExecutionForStoreBuildsRuntimeContextManifestAndGuardrails(t *testing.T) {
	execution := normalizeOrchestratorExecutionForStore(OrchestratorExecution{
		ID:                "exec-meta-store",
		Mode:              OrchestratorExecutionModeWork,
		Goal:              "Investigate failed publish",
		RequestedProvider: "openrouter",
		Work: OrchestratorExecutionWorkContext{
			ProjectID:     "proj-1",
			WorkItemID:    "work-1",
			RunID:         "run-1",
			WorkspaceID:   "ws-1",
			WorkspacePath: "/tmp/carrier/ws-1",
			Backend:       "managed_isolated",
		},
		SharedInstructions: []baseagent.SharedInstruction{{
			ID:      "work-mode",
			Title:   "Work Execution",
			Content: "Use the managed workspace as the execution source of truth.",
		}},
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision:      orchestratorPolicyDecisionAsk,
			Reason:        "prod publish requires review",
			MatchedRuleID: "policy-prod",
		},
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  "host-1",
			AgentID: "zeroclaw",
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:    "task-1",
			Input: "Inspect publish logs",
		}},
	})

	if len(execution.RuntimeContextManifest.Entries) == 0 {
		t.Fatalf("expected runtime context manifest entries, got %+v", execution.RuntimeContextManifest)
	}
	keys := make([]string, 0, len(execution.RuntimeContextManifest.Entries))
	for _, entry := range execution.RuntimeContextManifest.Entries {
		keys = append(keys, entry.Key)
	}
	joined := strings.Join(keys, ",")
	for _, expected := range []string{"execution.id", "policy.decision", "work.workspace_path"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected runtime context manifest key %q, got %v", expected, keys)
		}
	}
	if len(execution.Guardrails.Events) == 0 {
		t.Fatalf("expected guardrail events, got %+v", execution.Guardrails)
	}
	if execution.Guardrails.Events[0].Scope != baseagent.GuardrailScopeExecutionLaunch || execution.Guardrails.Events[0].Decision != baseagent.GuardrailDecisionAsk {
		t.Fatalf("unexpected execution launch guardrail event: %+v", execution.Guardrails.Events[0])
	}
	if execution.Guardrails.Summary.AskCount != 1 {
		t.Fatalf("expected ask guardrail summary, got %+v", execution.Guardrails.Summary)
	}
}

func TestBuildOrchestratorExecutionMetadataSnapshotIncludesGraphAndContracts(t *testing.T) {
	execution := normalizeOrchestratorExecutionForStore(OrchestratorExecution{
		ID:      "exec-meta-graph",
		Goal:    "Ship release notes",
		Mode:    OrchestratorExecutionModeTask,
		Status:  OrchestratorExecutionStatusCompleted,
		Project: "Carrier",
		SharedInstructions: []baseagent.SharedInstruction{{
			ID:      "release",
			Content: "Prefer release-safe changes.",
		}},
		RequiredMemory: []string{"shared:release"},
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  "host-1",
			AgentID: "zeroclaw",
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:      "task-1",
			Input:   "Draft release notes",
			AgentID: "zeroclaw",
			HostID:  "host-1",
		}},
		Results: []OrchestratorTaskResult{{
			TaskID:   "task-1",
			Status:   OrchestratorTaskStatusCompleted,
			WorkerID: "lease-1",
			HostID:   "host-1",
			AgentID:  "zeroclaw",
		}},
		Outcome: OrchestratorExecutionOutcome{
			Artifacts: []OrchestratorArtifact{{
				ID:     "artifact-1",
				TaskID: "task-1",
				Name:   "release-notes.md",
			}},
		},
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision: orchestratorPolicyDecisionAllow,
		},
	})

	metadata := buildOrchestratorExecutionMetadataSnapshot(execution)
	if metadata.ExecutionID != execution.ID {
		t.Fatalf("metadata.executionId=%q want %q", metadata.ExecutionID, execution.ID)
	}
	if len(metadata.SharedInstructions) != 1 {
		t.Fatalf("expected shared instructions in metadata snapshot, got %+v", metadata)
	}
	if len(metadata.Nodes) == 0 || len(metadata.Edges) == 0 {
		t.Fatalf("expected execution graph nodes/edges, got %+v", metadata)
	}
	var sawTaskNode, sawArtifactEdge bool
	for _, node := range metadata.Nodes {
		if node.Kind == "task" && node.ID == "task:task-1" {
			sawTaskNode = true
		}
	}
	for _, edge := range metadata.Edges {
		if edge.Kind == "produces_artifact" && edge.ToID == "artifact:artifact-1" {
			sawArtifactEdge = true
		}
	}
	if !sawTaskNode || !sawArtifactEdge {
		t.Fatalf("unexpected execution graph: %+v %+v", metadata.Nodes, metadata.Edges)
	}
}

func TestHandleOrchestratorExecutionMetadataAndEvidenceIncludeMetadataSnapshot(t *testing.T) {
	mux := buildRemoteFeatureMuxWithConfigAndDaemonHandlers(t, &GatewayConfig{
		APIToken:                  "test-gateway-token",
		MaxCommandBodyBytes:       64 * 1024,
		RemoteControlPlaneEnabled: true,
	}, nil)

	seed, err := upsertOrchestratorExecution(OrchestratorExecution{
		ID:     "exec-metadata-endpoint",
		Goal:   "Inspect metadata export",
		Status: OrchestratorExecutionStatusCompleted,
		SharedInstructions: []baseagent.SharedInstruction{{
			ID:      "meta",
			Content: "Keep metadata deterministic.",
		}},
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  "local",
			AgentID: "zeroclaw",
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:    "task-1",
			Input: "Inspect metadata export",
		}},
		Policy: OrchestratorExecutionPolicySnapshot{
			Decision: orchestratorPolicyDecisionAsk,
			Reason:   "metadata export should remain auditable",
		},
	})
	if err != nil {
		t.Fatalf("upsertOrchestratorExecution failed: %v", err)
	}

	metaRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/metadata", "")
	if metaRec.Code != http.StatusOK {
		t.Fatalf("metadata status=%d body=%s", metaRec.Code, metaRec.Body.String())
	}
	metaPayload := decodeJSONMap(t, metaRec)
	metadata, _ := metaPayload["metadata"].(map[string]interface{})
	if metadata == nil {
		t.Fatalf("missing metadata payload: %+v", metaPayload)
	}
	if got := strings.TrimSpace(anyToString(metadata["executionId"])); got != seed.ID {
		t.Fatalf("metadata.executionId=%q want %q payload=%+v", got, seed.ID, metaPayload)
	}
	guardrails, _ := metadata["guardrails"].(map[string]interface{})
	summary, _ := guardrails["summary"].(map[string]interface{})
	if got := int(anyToFloat(summary["askCount"])); got != 1 {
		t.Fatalf("guardrails.summary.askCount=%d want 1 metadata=%+v", got, metadata)
	}

	evidenceRec := runJSONRequest(t, mux, http.MethodGet, "/api/v1/orchestrator/executions/"+seed.ID+"/evidence?format=json", "")
	if evidenceRec.Code != http.StatusOK {
		t.Fatalf("evidence status=%d body=%s", evidenceRec.Code, evidenceRec.Body.String())
	}
	evidencePayload := decodeJSONMap(t, evidenceRec)
	evidence, _ := evidencePayload["evidence"].(map[string]interface{})
	metadataSnapshot, _ := evidence["metadataSnapshot"].(map[string]interface{})
	if metadataSnapshot == nil {
		t.Fatalf("missing metadataSnapshot in evidence: %+v", evidence)
	}
	if got := strings.TrimSpace(anyToString(metadataSnapshot["executionId"])); got != seed.ID {
		t.Fatalf("metadataSnapshot.executionId=%q want %q snapshot=%+v", got, seed.ID, metadataSnapshot)
	}
}
